//go:build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nphq/np/internal/cluster"
	"github.com/nphq/np/internal/config"
	"github.com/nphq/np/internal/secure"
	"github.com/nphq/np/internal/uiapi"
)

// newService 搭一个用临时配置文件 + 内存 keyring 的 ClusterService，
// 不会调 Start（避免 EventsEmit 需要 wails ctx）。
func newService(t *testing.T) *uiapi.ClusterService {
	t.Helper()
	store := config.New(filepath.Join(t.TempDir(), "clusters.json"))
	kr := secure.NewMemory()
	return uiapi.NewClusterService(store, kr)
}

// withCluster 启动共享 Nomad 并创建 ClusterService，支持 -short 跳过与失败时自动 dump 日志
func withCluster(t *testing.T) (*NomadDev, *uiapi.ClusterService) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}
	n := StartSharedNomadDev(t)
	svc := newService(t)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("--- Nomad Agent Logs (on failure) ---\n%s", n.logs(context.Background()))
		}
	})
	return n, svc
}

// connectCluster 调用 withCluster 并自动在 service 中添加默认的 'dev' 集群
func connectCluster(t *testing.T) (*NomadDev, *uiapi.ClusterService) {
	t.Helper()
	n, svc := withCluster(t)
	if err := svc.AddCluster(uiapi.ClusterInput{
		ID: "dev", Name: "Dev", Address: n.Address,
	}); err != nil {
		t.Fatalf("connectCluster: AddCluster: %v", err)
	}
	return n, svc
}

// 1. TestE2E_PoolGetProbe 验证 Pool → ProbeTarget → Probe 走通真 Nomad。
func TestE2E_PoolGetProbe(t *testing.T) {
	n, _ := withCluster(t)

	store := config.New(filepath.Join(t.TempDir(), "clusters.json"))
	_ = store.Add(&config.ClusterConfig{ID: "dev", Name: "Dev", Address: n.Address})
	pool := cluster.NewPool(store, secure.NewMemory())

	target, err := pool.ProbeTarget("dev")
	if err != nil {
		t.Fatalf("pool.ProbeTarget: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	u := cluster.Probe(ctx, target)
	if u.Status != "ok" {
		t.Fatalf("Probe status=%s err=%s", u.Status, u.Error)
	}
	if u.Leader == "" {
		t.Fatal("leader empty")
	}
	if u.Version == "" {
		t.Fatal("version empty (agent/self 未取到？)")
	}
	t.Logf("nomad %s leader=%s", u.Version, u.Leader)
}

// 2. TestE2E_AddClusterEndToEnd 全链路验证（合并了原来的 TestConnection 与 ListClustersReflectsHealth）：
// 添加前为空 → AddCluster → ListClusters 出现(unknown) → TestConnection 连通 → ListClusters 命中缓存变 ok。
func TestE2E_AddClusterEndToEnd(t *testing.T) {
	n, svc := withCluster(t)

	// 1. 添加前列表为空
	before, e := svc.ListClusters()
	if e != nil {
		t.Fatalf("ListClusters before add: %v", e)
	}
	if len(before) != 0 {
		t.Fatalf("expected empty list before add, got %d items", len(before))
	}

	// 2. 添加集群
	input := uiapi.ClusterInput{
		ID:      "e2e-full",
		Name:    "E2E Full Test",
		Address: n.Address,
	}
	if err := svc.AddCluster(input); err != nil {
		t.Fatalf("AddCluster: %v", err)
	}

	// 3. 添加后列表中可见，但未探测过，应为 unknown
	after, e := svc.ListClusters()
	if e != nil {
		t.Fatalf("ListClusters after add: %v", e)
	}
	if len(after) != 1 {
		t.Fatalf("expected 1 cluster, got %d: %+v", len(after), after)
	}
	c := after[0]
	if c.ID != "e2e-full" || c.Name != "E2E Full Test" || c.Address != n.Address {
		t.Fatalf("cluster fields mismatch: %+v", c)
	}
	if c.Health != "unknown" {
		t.Fatalf("health before probe should be unknown, got %s", c.Health)
	}

	// 4. 连通性探测
	health, e := svc.TestConnection("e2e-full")
	if e != nil {
		t.Fatalf("TestConnection: %v", e)
	}
	if health.Status != "ok" {
		t.Fatalf("TestConnection status=%s err=%s", health.Status, health.Error)
	}
	if health.Leader == "" || health.Version == "" {
		t.Fatal("leader/version empty")
	}

	// 5. 健康状态已更新，LastChecked 正常
	list, e := svc.ListClusters()
	if e != nil {
		t.Fatalf("ListClusters after probe: %v", e)
	}
	if list[0].Health != "ok" {
		t.Fatalf("health after probe should be ok, got %s", list[0].Health)
	}
	if list[0].LastChecked == 0 {
		t.Fatal("LastChecked not updated")
	}
}

// 3. TestE2E_HealthMonitorRun 验证后台 monitor 周期探测真 Nomad，在 deadline 内 emit 一个 ok 状态。
func TestE2E_HealthMonitorRun(t *testing.T) {
	n, _ := withCluster(t)

	store := config.New(filepath.Join(t.TempDir(), "clusters.json"))
	_ = store.Add(&config.ClusterConfig{ID: "dev", Address: n.Address})

	var got atomic.Int32
	emit := func(u cluster.HealthUpdate) {
		if u.ClusterID == "dev" && u.Status == "ok" {
			got.Add(1)
		}
	}
	m := cluster.NewHealthMonitor(cluster.NewPool(store, secure.NewMemory()), store, 200*time.Millisecond, emit)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go m.Run(ctx)

	deadline := time.After(5 * time.Second)
	for got.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("monitor did not emit ok within 5s")
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}
	m.Stop()

	if u, ok := m.Latest("dev"); !ok || u.Status != "ok" {
		t.Fatalf("Latest = %+v ok=%v", u, ok)
	}
}

// 4. TestE2E_RemoveClusterInvalidatesPool 验证 Remove + 用同 ID 重加 → 新 client。
func TestE2E_RemoveClusterInvalidatesPool(t *testing.T) {
	n, svc := connectCluster(t)

	// 触发一次 pool 缓存
	if _, e := svc.TestConnection("dev"); e != nil {
		t.Fatalf("TestConnection: %v", e)
	}

	// 删 → 重加（不同 name 以示区别）
	if e := svc.RemoveCluster("dev"); e != nil {
		t.Fatalf("RemoveCluster: %v", e)
	}
	if err := svc.AddCluster(uiapi.ClusterInput{
		ID: "dev", Name: "Dev2", Address: n.Address,
	}); err != nil {
		t.Fatalf("re-AddCluster: %v", err)
	}

	// 再探：必须走新 client（pool 已 invalidate），状态 ok
	h, e := svc.TestConnection("dev")
	if e != nil {
		t.Fatalf("TestConnection after re-add: %v", e)
	}
	if h.Status != "ok" {
		t.Fatalf("status=%+v", h)
	}

	// 验证配置确实更新为 Dev2
	list, _ := svc.ListClusters()
	if len(list) != 1 || list[0].Name != "Dev2" {
		t.Fatalf("list = %+v", list)
	}
}

// 5. TestE2E_DownClusterReflectsDown 验证不可达集群的健康状态确实变红。
// 使用动态分配的、已关闭的本地端口，避免使用 port 9 (Discard) 导致的超时挂起。
func TestE2E_DownClusterReflectsDown(t *testing.T) {
	_, svc := withCluster(t)

	// 获取一个保证已关闭的 ephemeral port
	ports, err := getFreePorts(1)
	var port int
	if err == nil && len(ports) > 0 {
		port = ports[0]
	} else {
		port = 59999 // fallback
	}
	addr := fmt.Sprintf("http://127.0.0.1:%d", port)

	if err := svc.AddCluster(uiapi.ClusterInput{
		ID: "dead", Name: "Dead", Address: addr,
	}); err != nil {
		t.Fatalf("AddCluster: %v", err)
	}
	h, e := svc.TestConnection("dead")
	if e != nil {
		t.Fatalf("TestConnection returned error (expected health update instead of error): %v", e)
	}
	if h.Status != "down" || h.Error == "" {
		t.Fatalf("want down + error msg, got %+v", h)
	}
}

// 6. TestE2E_RegisterClusterUserFlow 把"通过 UI 注册一个新集群"的完整旅程串成一个 case：
// 空列表 → AddCluster → 列表可见且 health=unknown → SetActiveCluster →
// TestConnection → health=ok → RemoveCluster → 列表清空、active 归零。
// 覆盖前端单条"注册集群"流程在真实 Nomad 上的全部状态切换。
func TestE2E_RegisterClusterUserFlow(t *testing.T) {
	n, svc := withCluster(t)

	const id = "ux-flow"
	// withCluster 不预加任何集群，基准应为 0（用 nBefore 兜底以适配未来变体）
	before, e := svc.ListClusters()
	if e != nil {
		t.Fatalf("ListClusters before: %v", e)
	}
	nBefore := len(before)

	// 1. AddCluster
	if e := svc.AddCluster(uiapi.ClusterInput{
		ID: id, Name: "UX Flow", Address: n.Address,
	}); e != nil {
		t.Fatalf("AddCluster: %v", e)
	}

	// 2. 列表中可见，health=unknown（未探测）
	list, e := svc.ListClusters()
	if e != nil {
		t.Fatalf("ListClusters after add: %v", e)
	}
	if len(list) != nBefore+1 {
		t.Fatalf("expected %d clusters, got %d", nBefore+1, len(list))
	}
	var found bool
	for _, c := range list {
		if c.ID == id {
			found = true
			if c.Health != "unknown" {
				t.Fatalf("health before probe = %s, want unknown", c.Health)
			}
		}
	}
	if !found {
		t.Fatalf("cluster %s not in list: %+v", id, list)
	}

	// 3. SetActiveCluster
	if e := svc.SetActiveCluster(id); e != nil {
		t.Fatalf("SetActiveCluster: %v", e)
	}
	if svc.ActiveCluster() != id {
		t.Fatalf("active = %q, want %q", svc.ActiveCluster(), id)
	}

	// 4. TestConnection → ok
	h, e := svc.TestConnection(id)
	if e != nil {
		t.Fatalf("TestConnection: %v", e)
	}
	if h.Status != "ok" {
		t.Fatalf("TestConnection status=%s err=%s", h.Status, h.Error)
	}

	// 5. ListClusters 反映 ok
	list2, e := svc.ListClusters()
	if e != nil {
		t.Fatalf("ListClusters after probe: %v", e)
	}
	for _, c := range list2 {
		if c.ID == id && c.Health != "ok" {
			t.Fatalf("health after probe = %s, want ok", c.Health)
		}
	}

	// 6. RemoveCluster → 列表回到 nBefore、active 归零（删的正是 active）
	if e := svc.RemoveCluster(id); e != nil {
		t.Fatalf("RemoveCluster: %v", e)
	}
	if svc.ActiveCluster() != "" {
		t.Fatalf("active should be cleared (was %q), got %q", id, svc.ActiveCluster())
	}
	final, e := svc.ListClusters()
	if e != nil {
		t.Fatalf("ListClusters after remove: %v", e)
	}
	if len(final) != nBefore {
		t.Fatalf("expected %d clusters after remove, got %d", nBefore, len(final))
	}
	for _, c := range final {
		if c.ID == id {
			t.Fatalf("cluster %s still present after remove", id)
		}
	}
}

// 7. TestE2E_MultiClusterPoolIsolation 验证多集群并存：
// 每个集群在池里拿到独立 client、active 切换不串味、删一个不影响另一个。
func TestE2E_MultiClusterPoolIsolation(t *testing.T) {
	n, svc := connectCluster(t) // 已含 'dev'

	if e := svc.AddCluster(uiapi.ClusterInput{
		ID: "second", Name: "Second", Address: n.Address,
	}); e != nil {
		t.Fatalf("AddCluster second: %v", e)
	}

	// 两边都能探到 ok（说明 pool 给出了两个独立可用 client）
	h1, e := svc.TestConnection("dev")
	if e != nil {
		t.Fatalf("TestConnection dev: %v", e)
	}
	h2, e := svc.TestConnection("second")
	if e != nil {
		t.Fatalf("TestConnection second: %v", e)
	}
	if h1.Status != "ok" || h2.Status != "ok" {
		t.Fatalf("both should be ok: dev=%s second=%s", h1.Status, h2.Status)
	}

	// 切换 active：dev → second
	if e := svc.SetActiveCluster("dev"); e != nil {
		t.Fatalf("SetActiveCluster dev: %v", e)
	}
	if svc.ActiveCluster() != "dev" {
		t.Fatalf("active = %q, want dev", svc.ActiveCluster())
	}
	if e := svc.SetActiveCluster("second"); e != nil {
		t.Fatalf("SetActiveCluster second: %v", e)
	}
	if svc.ActiveCluster() != "second" {
		t.Fatalf("active = %q, want second", svc.ActiveCluster())
	}

	// 删 second：active 应被清空（CAS 命中），但 dev 不受影响
	if e := svc.RemoveCluster("second"); e != nil {
		t.Fatalf("RemoveCluster second: %v", e)
	}
	if svc.ActiveCluster() != "" {
		t.Fatalf("active should be cleared (was 'second'), got %q", svc.ActiveCluster())
	}
	h3, e := svc.TestConnection("dev")
	if e != nil {
		t.Fatalf("TestConnection dev after remove: %v", e)
	}
	if h3.Status != "ok" {
		t.Fatalf("dev should still be ok after removing second: %s", h3.Status)
	}
}

// 8. TestE2E_TokenGoesToKeyringOnly 守护 ADR-1：ClusterInput.Token 必须只进 Keychain，
// 绝不落盘到 clusters.json。token 泄漏到配置文件是重大安全事故。
func TestE2E_TokenGoesToKeyringOnly(t *testing.T) {
	n, _ := withCluster(t)

	cfgPath := filepath.Join(t.TempDir(), "clusters.json")
	store := config.New(cfgPath)
	kr := secure.NewMemory()
	svc := uiapi.NewClusterService(store, kr)

	const secret = "super-secret-token-xyz"
	if e := svc.AddCluster(uiapi.ClusterInput{
		ID: "with-token", Name: "With Token",
		Address: n.Address, Token: secret,
	}); e != nil {
		t.Fatalf("AddCluster: %v", e)
	}

	// Keyring 有 token
	got, err := kr.GetToken("with-token")
	if err != nil {
		t.Fatalf("keyring GetToken: %v", err)
	}
	if got != secret {
		t.Fatalf("keyring returned %q, want %q", got, secret)
	}

	// 配置文件不含 token 明文，也不含 "token" 字段名
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if strings.Contains(string(data), secret) {
		t.Fatalf("TOKEN LEAKED INTO CONFIG FILE:\n%s", string(data))
	}
	if strings.Contains(string(data), `"token"`) {
		t.Fatalf("'token' field present in config file:\n%s", string(data))
	}

	// 删集群 → keyring 也清掉（避免悬空 token）
	if e := svc.RemoveCluster("with-token"); e != nil {
		t.Fatalf("RemoveCluster: %v", e)
	}
	if _, err := kr.GetToken("with-token"); !errors.Is(err, secure.ErrTokenNotFound) {
		t.Fatalf("expected ErrTokenNotFound after remove, got %v", err)
	}
}

// 9. TestE2E_AddClusterRejectsInvalidInput 在 e2e 边界验证校验：各种坏输入都返回
// CodeInvalidInput，重复 ID 返回 CodeDuplicate。
func TestE2E_AddClusterRejectsInvalidInput(t *testing.T) {
	n, svc := connectCluster(t) // 已含 'dev'，用于 duplicate 用例
	goodAddr := n.Address

	cases := []struct {
		name string
		in   uiapi.ClusterInput
	}{
		{"empty ID", uiapi.ClusterInput{ID: "", Name: "X", Address: goodAddr}},
		{"bad ID chars", uiapi.ClusterInput{ID: "bad id!", Name: "X", Address: goodAddr}},
		{"empty address", uiapi.ClusterInput{ID: "valid1", Name: "X", Address: ""}},
		{"bad port", uiapi.ClusterInput{ID: "valid2", Name: "X", Address: "http://localhost:99999"}},
		{"name too long", uiapi.ClusterInput{ID: "valid3", Name: strings.Repeat("x", 65), Address: goodAddr}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			e := svc.AddCluster(c.in)
			if e == nil {
				t.Fatalf("expected error, got nil")
			}
			if e.Code != uiapi.CodeInvalidInput {
				t.Fatalf("got code %q, want %q (msg: %s)", e.Code, uiapi.CodeInvalidInput, e.Message)
			}
		})
	}

	// 重复 ID（'dev' 已由 withCluster 加过）应给 CodeDuplicate
	if e := svc.AddCluster(uiapi.ClusterInput{
		ID: "dev", Name: "Dup", Address: goodAddr,
	}); e == nil || e.Code != uiapi.CodeDuplicate {
		t.Fatalf("duplicate add: want %q, got %+v", uiapi.CodeDuplicate, e)
	}
}
