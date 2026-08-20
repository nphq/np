//go:build e2e

// Package e2e 在 build tag `e2e` 下运行；通过 Docker 拉起 hashicorp/nomad
// server（server-only 配置，避免 dev agent 在 Docker 内的 cgroup 限制），
// 对 internal/cluster + internal/uiapi 做端到端验证。
// 默认不跑：`go test ./...` 跳过本包；`go test -tags=e2e ./internal/e2e/...` 触发。
// 镜像可用环境变量 NOMAD_IMAGE 覆盖（默认 hashicorp/nomad:2.0.5）。
package e2e

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

const defaultImage = "hashicorp/nomad:2.0.5"

func nomadImage() string {
	if v := os.Getenv("NOMAD_IMAGE"); v != "" {
		return v
	}
	return defaultImage
}

const serverConfig = `# E2E server-only 配置；client 关掉以避开 Docker cgroup 限制
data_dir = "/var/lib/nomad"
datacenter = "dc1"
name = "nomad-e2e"

server {
  enabled = true
  bootstrap_expect = 1
}

client {
  enabled = false
}
`

// NomadDev 是一个跑在 Docker 里的 nomad server 实例。
// 测试包内全局共享（TestMain 启动一次），加快测试。
type NomadDev struct {
	ContainerID string
	Address     string // http://localhost:<host-port>
	image       string
}

var (
	sharedOnce sync.Once
	shared     *NomadDev
	sharedErr  error
)

// StartSharedNomadDev 返回 TestMain 启动的全局共享 NomadDev。
// 容器随整个测试进程退出由 TestMain 释放（不绑单个测试的 t.Cleanup，
// 否则首个测试结束就会把容器删掉）。
func StartSharedNomadDev(t *testing.T) *NomadDev {
	t.Helper()
	sharedOnce.Do(func() {
		// 用 background ctx 启动；生命周期由 TestMain 管
		shared, sharedErr = startNomadDevLocked(context.Background())
	})
	if sharedErr != nil {
		t.Fatalf("start shared nomad: %v", sharedErr)
	}
	return shared
}

func getFreePorts(n int) ([]int, error) {
	var ports []int
	var listeners []*net.TCPListener
	defer func() {
		for _, l := range listeners {
			_ = l.Close()
		}
	}()

	for i := 0; i < n; i++ {
		addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
		if err != nil {
			return nil, err
		}
		l, err := net.ListenTCP("tcp", addr)
		if err != nil {
			return nil, err
		}
		listeners = append(listeners, l)
		ports = append(ports, l.Addr().(*net.TCPAddr).Port)
	}
	return ports, nil
}

func startLocalNomadDev(ctx context.Context) (*NomadDev, error) {
	nomadPath, err := exec.LookPath("nomad")
	if err != nil {
		return nil, fmt.Errorf("nomad binary not in PATH: %w", err)
	}

	ports, err := getFreePorts(3)
	if err != nil {
		return nil, fmt.Errorf("failed to get free ports: %w", err)
	}
	httpPort := ports[0]
	rpcPort := ports[1]
	serfPort := ports[2]

	dataDir, err := os.MkdirTemp("", "nomad-local-data-")
	if err != nil {
		return nil, err
	}

	cfgDir, err := os.MkdirTemp("", "nomad-local-cfg-")
	if err != nil {
		return nil, err
	}

	localConfig := fmt.Sprintf(`# Local E2E server-only 配置
data_dir = %q
datacenter = "dc1"
name = "nomad-local-e2e"
bind_addr = "127.0.0.1"

advertise {
  http = "127.0.0.1"
  rpc  = "127.0.0.1"
  serf = "127.0.0.1"
}

ports {
  http = %d
  rpc = %d
  serf = %d
}

server {
  enabled = true
  bootstrap_expect = 1
}

client {
  enabled = false
}
`, dataDir, httpPort, rpcPort, serfPort)

	cfgPath := filepath.Join(cfgDir, "server.hcl")
	if err := os.WriteFile(cfgPath, []byte(localConfig), 0o644); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, nomadPath, "agent", "-config="+cfgPath)
	logFile, err := os.Create(filepath.Join(cfgDir, "nomad.log"))
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start nomad agent: %w", err)
	}

	n := &NomadDev{
		ContainerID: fmt.Sprintf("local:%d:%s:%s", cmd.Process.Pid, cfgDir, dataDir),
		Address:     fmt.Sprintf("http://127.0.0.1:%d", httpPort),
		image:       "local-binary",
	}

	if !n.waitForLeader(ctx, 40*time.Second) {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		logBytes, _ := os.ReadFile(filepath.Join(cfgDir, "nomad.log"))
		return nil, fmt.Errorf("local nomad server did not elect leader in 40s; logs:\n%s", string(logBytes))
	}
	return n, nil
}

func startNomadDevLocked(ctx context.Context) (*NomadDev, error) {
	// First check if docker is available and running. If not, fallback to local nomad binary.
	var useDocker = true
	if _, err := exec.LookPath("docker"); err != nil {
		useDocker = false
	} else if err := exec.CommandContext(ctx, "docker", "info").Run(); err != nil {
		useDocker = false
	}

	if !useDocker {
		return startLocalNomadDev(ctx)
	}

	image := nomadImage()
	pull := exec.CommandContext(ctx, "docker", "pull", image)
	if out, err := pull.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("docker pull %s: %w\n%s", image, err, out)
	}

	// 写 server-only 配置到临时目录，bind-mount 进容器
	cfgDir, err := os.MkdirTemp("", "nomad-e2e-cfg-")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "server.hcl"), []byte(serverConfig), 0o644); err != nil {
		return nil, err
	}

	name := fmt.Sprintf("nomad-e2e-%d", time.Now().UnixNano())
	// 不加 --rm：失败时便于 docker logs 取证；TestMain 在 m.Run() 后 stopContainer。
	run := exec.CommandContext(ctx, "docker", "run", "-d",
		"--name", name,
		"-p", "4646",
		"-v", cfgDir+":/etc/nomad.d:ro",
		image, "agent", "-config=/etc/nomad.d", "-log-level=WARN",
	)
	out, err := run.Output()
	if err != nil {
		return nil, fmt.Errorf("docker run: %w: %s", err, out)
	}
	id := strings.TrimSpace(string(out))

	portOut, err := exec.CommandContext(ctx, "docker", "port", id, "4646").Output()
	if err != nil {
		stopContainer(id)
		return nil, fmt.Errorf("docker port: %w", err)
	}
	hostPort := parseHostPort(string(portOut))
	if hostPort == "" {
		stopContainer(id)
		return nil, fmt.Errorf("cannot parse host port from %q", string(portOut))
	}

	n := &NomadDev{
		ContainerID: id,
		Address:     "http://localhost:" + hostPort,
		image:       image,
	}

	if !n.waitForLeader(ctx, 40*time.Second) {
		return nil, fmt.Errorf("nomad server did not elect leader in 40s; logs:\n%s", n.logs(ctx))
	}
	return n, nil
}

func parseHostPort(s string) string {
	// docker port 输出形如 "0.0.0.0:49153\n:::49153"
	for _, line := range strings.Split(s, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 2)
		if len(parts) == 2 && parts[1] != "" {
			return parts[1]
		}
	}
	return ""
}

// waitForLeader 轮询 /v1/status/leader 直到返回非空 leader 或超时。
func (n *NomadDev) waitForLeader(ctx context.Context, max time.Duration) bool {
	deadline := time.Now().Add(max)
	url := n.Address + "/v1/status/leader"
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			// 形如 "\"192.168.215.2:4647\""，未就绪时返回空字符串 "\"\""
			trimmed := strings.Trim(strings.TrimSpace(string(body)), "\"")
			if trimmed != "" && resp.StatusCode == 200 {
				return true
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}

func (n *NomadDev) logs(ctx context.Context) string {
	if strings.HasPrefix(n.ContainerID, "local:") {
		parts := strings.Split(n.ContainerID, ":")
		if len(parts) >= 3 && parts[2] != "" {
			logBytes, err := os.ReadFile(filepath.Join(parts[2], "nomad.log"))
			if err == nil {
				return string(logBytes)
			}
			return fmt.Sprintf("(read local log failed: %v)", err)
		}
		return "(no log path for local)"
	}

	out, err := exec.CommandContext(ctx, "docker", "logs", n.ContainerID).CombinedOutput()
	if err != nil {
		return fmt.Sprintf("(docker logs failed: %v)", err)
	}
	return string(out)
}

func stopContainer(id string) {
	if strings.HasPrefix(id, "local:") {
		parts := strings.Split(id, ":")
		if len(parts) >= 2 {
			pidStr := parts[1]
			if pid, err := strconv.Atoi(pidStr); err == nil {
				if proc, err := os.FindProcess(pid); err == nil {
					_ = proc.Kill()
					_, _ = proc.Wait()
				}
			}
		}
		if len(parts) >= 3 && parts[2] != "" {
			_ = os.RemoveAll(parts[2])
		}
		if len(parts) >= 4 && parts[3] != "" {
			_ = os.RemoveAll(parts[3])
		}
		return
	}

	_ = exec.Command("docker", "stop", "-t", "1", id).Run()
	_ = exec.Command("docker", "rm", "-f", id).Run()
}
