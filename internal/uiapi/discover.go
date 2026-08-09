package uiapi

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/nphq/np/internal/config"
	"github.com/nphq/np/internal/nomad"
)

// 来源常量。
const (
	SourceEnv  = "env"
	SourceFile = "file"
)

// DiscoveredCluster 是「从环境/文件发现」的候选，供 UI 预填。
// 不落盘；token 明文绝不进响应（HasToken 只表示是否有值）。
type DiscoveredCluster struct {
	Source             string `json:"source"` // "env" | "file"
	SuggestedID        string `json:"suggestedID"`
	Name               string `json:"name"`
	Address            string `json:"address"`
	Region             string `json:"region"`
	Namespace          string `json:"namespace"`
	TLS                bool   `json:"tls"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
	HasToken           bool   `json:"hasToken"`
}

// DiscoverClusters 探测本机可用连接候选：先环境变量，其次常见配置文件。
// 纯读、无副作用；无发现时返回空列表而非错误。
func (s *ClusterService) DiscoverClusters() ([]DiscoveredCluster, *Error) {
	var out []DiscoveredCluster
	if d, ok := s.discoverFromEnv(); ok {
		out = append(out, d)
	}
	return out, nil
}

// discoverFromEnv 从 NOMAD_* 环境变量读取候选（与 Nomad CLI 习惯对齐）。
func (s *ClusterService) discoverFromEnv() (DiscoveredCluster, bool) {
	addr := strings.TrimSpace(os.Getenv("NOMAD_ADDR"))
	if addr == "" {
		return DiscoveredCluster{}, false
	}
	d := DiscoveredCluster{
		Source:      SourceEnv,
		SuggestedID: s.nextSuggestedID("local"),
		Name:        "From environment",
		Address:     addr,
		Region:      strings.TrimSpace(os.Getenv("NOMAD_REGION")),
		Namespace:   strings.TrimSpace(os.Getenv("NOMAD_NAMESPACE")),
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("NOMAD_SKIP_VERIFY"))) {
	case "true", "1", "yes":
		d.TLS = true
		d.InsecureSkipVerify = true
	}
	if os.Getenv("NOMAD_TOKEN") != "" {
		d.HasToken = true
	}
	return d, true
}

// nextSuggestedID 生成不冲突的 ID：local → local-2 → local-3…
func (s *ClusterService) nextSuggestedID(base string) string {
	if _, err := s.cfg.Get(base); err != nil {
		return base
	}
	for i := 2; ; i++ {
		cand := fmt.Sprintf("%s-%d", base, i)
		if _, err := s.cfg.Get(cand); err != nil {
			return cand
		}
	}
}

// ImportFromEnv 一键导入：服务端读取 NOMAD_* 环境变量（token 不经前端往返），
// 组装 ClusterInput → AddCluster → SetActive。name 为显示名（空则用默认）。
// 若同 Address 已存在，则激活已有集群而非重复创建（§5.4）。
func (s *ClusterService) ImportFromEnv(name string) (*nomad.ClusterInfo, *Error) {
	d, ok := s.discoverFromEnv()
	if !ok {
		return nil, NewError(CodeNotFound, "NOMAD_ADDR is not set")
	}
	if strings.TrimSpace(name) == "" {
		name = "From environment"
	}
	info, e := s.importDiscovered(d, name, os.Getenv("NOMAD_TOKEN"))
	if e != nil {
		return nil, e
	}
	return info, nil
}

// importDiscovered 是 Discover → Import 的共享实现。
// token 仅在此处传入（ImportFromEnv 直接读 env；不会出现在 Discover 响应里）。
func (s *ClusterService) importDiscovered(d DiscoveredCluster, name, token string) (*nomad.ClusterInfo, *Error) {
	addr, err := ValidateAddress(d.Address)
	if err != nil {
		return nil, NewError(CodeInvalidInput, "%v", err)
	}
	// 同 Address 去重（§5.4）：激活已有集群，不重复创建；
	// 若本次带了 token，刷新 Keychain（env 轮换后 re-import 应对齐）。
	if existing := s.findByAddress(addr); existing != nil {
		if token != "" {
			if err := s.keyring.SaveToken(existing.ID, token); err != nil {
				return nil, NewError(CodeInternal, "token save failed: %v", err)
			}
			s.pool.Invalidate(existing.ID)
		}
		if e := s.SetActiveCluster(existing.ID); e != nil {
			return nil, e
		}
		info := s.clusterInfo(existing)
		return &info, nil
	}
	// JSON 导入（SourceFile）必须保留声明的 ID：冲突时由 AddCluster 返回
	// CodeDuplicate，ImportClusterJSON 再跳过（§5.4）。env 发现仍可自增。
	id := d.SuggestedID
	if d.Source != SourceFile {
		id = s.nextSuggestedID(d.SuggestedID)
	}
	in := ClusterInput{
		ID:                 id,
		Name:               name,
		Address:            addr,
		Region:             d.Region,
		Namespace:          d.Namespace,
		TLS:                d.TLS,
		InsecureSkipVerify: d.InsecureSkipVerify,
		Token:              token,
	}
	if e := s.AddCluster(in); e != nil {
		return nil, e
	}
	if e := s.SetActiveCluster(in.ID); e != nil {
		return nil, e
	}
	saved, err := s.cfg.Get(in.ID)
	if err != nil {
		return nil, Wrap(err)
	}
	info := s.clusterInfo(saved)
	return &info, nil
}

// findByAddress 按规范化后的 Address 查找已存集群（用于去重）。
func (s *ClusterService) findByAddress(addr string) *config.ClusterConfig {
	cfgs, err := s.cfg.List()
	if err != nil {
		return nil
	}
	for _, c := range cfgs {
		if c.Address == addr {
			return c
		}
	}
	return nil
}

// ImportClusterJSON 导入与 ClusterConfig 同形的 JSON（数组或单对象，无 token 字段）。
// 同 Address 去重（激活已有）；ID 冲突的条目跳过。返回成功导入（或已激活）的集群。
func (s *ClusterService) ImportClusterJSON(raw string) ([]nomad.ClusterInfo, *Error) {
	var single config.ClusterConfig
	var list []config.ClusterConfig
	if err := json.Unmarshal([]byte(raw), &single); err == nil && single.Address != "" {
		list = []config.ClusterConfig{single}
	} else if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, NewError(CodeInvalidInput, "invalid cluster JSON: %v", err)
	}
	if len(list) == 0 {
		return nil, NewError(CodeInvalidInput, "no clusters found in JSON")
	}

	var imported []nomad.ClusterInfo
	for _, c := range list {
		if c.ID == "" || c.Address == "" {
			continue // 缺关键字段的条目静默跳过
		}
		addr, err := ValidateAddress(c.Address)
		if err != nil {
			continue
		}
		d := DiscoveredCluster{
			Source:             SourceFile,
			SuggestedID:        c.ID,
			Name:               c.Name,
			Address:            addr,
			Region:             c.Region,
			Namespace:          c.Namespace,
			TLS:                c.TLS,
			InsecureSkipVerify: c.InsecureSkipVerify,
		}
		info, e := s.importDiscovered(d, c.Name, "")
		if e != nil {
			if e.Code == CodeDuplicate {
				continue // 同 ID 已存在但地址不同：跳过，不覆盖
			}
			continue
		}
		imported = append(imported, *info)
	}
	if len(imported) == 0 {
		return nil, NewError(CodeInvalidInput, "no valid clusters in JSON")
	}
	return imported, nil
}
