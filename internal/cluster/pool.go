package cluster

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/nomad/api"

	"github.com/nphq/np/internal/config"
	"github.com/nphq/np/internal/secure"
)

// ClientFactory 创建 api.Client 的函数类型（便于测试注入）。
type ClientFactory func(cfg *config.ClusterConfig, token string) (*api.Client, error)

// DefaultClientFactory 用官方 SDK 创建客户端。
func DefaultClientFactory(cfg *config.ClusterConfig, token string) (*api.Client, error) {
	addr := cfg.Address
	// 与 nomad CLI 约定一致：允许省略 scheme，默认 http://
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}
	acfg := api.DefaultConfig()
	acfg.Address = addr
	acfg.SecretID = token
	acfg.HttpClient = buildHTTPClient(cfg)
	if cfg.Region != "" {
		acfg.Region = cfg.Region
	}
	if cfg.Namespace != "" {
		acfg.Namespace = cfg.Namespace
	}
	client, err := api.NewClient(acfg)
	if err != nil {
		return nil, fmt.Errorf("create nomad client: %w", err)
	}
	return client, nil
}

// buildHTTPClient 构造统一的 raw HTTP client（10s 超时；自签证书时跳过校验）。
// 与 DefaultClientFactory 共享，避免 SDK 客户端与 Probe 路径行为不一致。
func buildHTTPClient(cfg *config.ClusterConfig) *http.Client {
	return NewProbeTarget(cfg.Address, "", cfg.TLS, cfg.InsecureSkipVerify).HTTPClient
}

// NewProbeTarget 由 addr/token/useTLS/insecure 直接构造 ProbeTarget，不走 Pool/Store。
// 用于"添加集群前测连接"等尚未落盘的场景，与已落盘集群走同一份 TLS/超时配置。
func NewProbeTarget(addr, token string, useTLS, insecureSkipVerify bool) ProbeTarget {
	hc := &http.Client{Timeout: 10 * time.Second}
	if useTLS && insecureSkipVerify {
		hc.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // 用户显式开启
		}
	}
	return ProbeTarget{Addr: addr, Token: token, HTTPClient: hc}
}

// Pool 管理每集群一个 *api.Client，按 clusterID 路由。
type Pool struct {
	mu      sync.RWMutex
	buildMu sync.Mutex
	clients map[string]*api.Client
	factory ClientFactory
	cfg     *config.Store
	keyring secure.Keyring
}

// NewPool 创建 Client Pool。
func NewPool(cfg *config.Store, kr secure.Keyring) *Pool {
	return &Pool{
		clients: map[string]*api.Client{},
		factory: DefaultClientFactory,
		cfg:     cfg,
		keyring: kr,
	}
}

// Get 返回集群的 api.Client（懒创建）。创建失败返回 nil client 与错误。
func (p *Pool) Get(clusterID string) (*api.Client, error) {
	p.mu.RLock()
	c, ok := p.clients[clusterID]
	p.mu.RUnlock()
	if ok {
		return c, nil
	}
	return p.build(clusterID)
}

func (p *Pool) build(clusterID string) (*api.Client, error) {
	p.buildMu.Lock()
	defer p.buildMu.Unlock()
	// 双检：等待期间的并发 build 已完成
	p.mu.RLock()
	if c, ok := p.clients[clusterID]; ok {
		p.mu.RUnlock()
		return c, nil
	}
	p.mu.RUnlock()

	cfg, err := p.cfg.Get(clusterID)
	if err != nil {
		return nil, err
	}
	token, err := p.keyring.GetToken(clusterID)
	if err != nil && !errors.Is(err, secure.ErrTokenNotFound) {
		return nil, fmt.Errorf("read token: %w", err)
	}
	client, err := p.factory(cfg, token)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if existing, ok := p.clients[clusterID]; ok {
		return existing, nil
	}
	p.clients[clusterID] = client
	return client, nil
}

// Close 释放全部客户端（应用退出时调用）。
// 注意：*api.Client 没有公开的 Close，这里只清缓存；底层 HTTP transport
// 由 SDK 共享，进程退出时回收。
func (p *Pool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for id := range p.clients {
		delete(p.clients, id)
	}
}

// Namespace 返回集群配置的默认 namespace（空 = 不设置，服务端回退 default）。
// SDK 的 acfg.Namespace 不会自动传导到请求（只在 Query/WriteOptions.Namespace
// 非空时注入参数），所以查询/写入前需显式读取并填充（review：集群配置的
// namespace 之前对 ListJobs/StopJob 等完全不生效）。
func (p *Pool) Namespace(clusterID string) (string, error) {
	cfg, err := p.cfg.Get(clusterID)
	if err != nil {
		return "", err
	}
	return cfg.Namespace, nil
}

// GetNS 返回集群 client 与配置的默认 namespace，供调用方构造带 namespace 的
// Query/WriteOptions（job 域操作必须带，否则打到 default namespace）。
func (p *Pool) GetNS(clusterID string) (*api.Client, string, error) {
	client, err := p.Get(clusterID)
	if err != nil {
		return nil, "", err
	}
	ns, err := p.Namespace(clusterID)
	if err != nil {
		return nil, "", err
	}
	return client, ns, nil
}

// Invalidate 清除指定集群的缓存 client。下次 Get 会用最新配置/token 重建。
// 用于 RemoveCluster、Update（未来）等场景，避免返回带旧 token 的 client。
// 健康探测逻辑见 health.go（Probe + HealthMonitor）。
func (p *Pool) Invalidate(clusterID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.clients, clusterID)
}

// ProbeTarget 返回某集群的 raw HTTP 材料（addr + token + httpClient），
// 供 Probe 走 ctx-aware 的直连 HTTP，绕开 SDK Status().Leader() 不收 ctx
// 的限制。读取配置/token 与 Get 同源，但每次重建 http.Client（调用频率低）。
func (p *Pool) ProbeTarget(clusterID string) (ProbeTarget, error) {
	cfg, err := p.cfg.Get(clusterID)
	if err != nil {
		return ProbeTarget{}, err
	}
	token, err := p.keyring.GetToken(clusterID)
	if err != nil && !errors.Is(err, secure.ErrTokenNotFound) {
		return ProbeTarget{}, fmt.Errorf("read token: %w", err)
	}
	return ProbeTarget{
		Addr:       cfg.Address,
		Token:      token,
		HTTPClient: buildHTTPClient(cfg),
	}, nil
}
