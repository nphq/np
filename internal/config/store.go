package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// ClusterConfig 是集群的非敏感配置。token 绝不存于此，只存 Keychain。
type ClusterConfig struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	Region    string `json:"region"`
	Namespace string `json:"namespace"`
	TLS       bool   `json:"tls"`
	// InsecureSkipVerify 仅用于自签证书的开发集群，默认 false
	InsecureSkipVerify bool `json:"insecureSkipVerify,omitempty"`
	// Pinned 表示收藏/置顶；排序规则见 uiapi.ListClusters（Pinned 在前，
	// 组内按 SortOrder → Name → ID）。此结构仅持久化，不负责排序。
	Pinned    bool `json:"pinned,omitempty"`
	SortOrder int  `json:"sortOrder,omitempty"`
}

var (
	ErrClusterExists   = errors.New("cluster already exists")
	ErrClusterNotFound = errors.New("cluster not found")
)

// Store 管理多集群配置的持久化（JSON，原子写）。
type Store struct {
	mu       sync.RWMutex
	path     string
	clusters map[string]*ClusterConfig // key = ID
	loaded   bool
}

// dirName 是当前配置目录名（品牌短名）；legacyDirName 是品牌统一前的旧目录名。
const (
	dirName       = "np"
	legacyDirName = "nomad-manager"
)

// DefaultDir 返回用户配置目录：~/.config/np
// （品牌统一前为 ~/.config/nomad-manager，旧目录存在时自动迁移一次）。
func DefaultDir() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	dir := filepath.Join(base, dirName)
	// 一次性迁移旧版目录；失败仅告警不阻断启动（新目录照常创建）。
	if err := migrateLegacyDir(base, dir); err != nil {
		fmt.Printf("WARN: migrate legacy config dir %s -> %s: %v\n", filepath.Join(base, legacyDirName), dir, err)
	}
	// 目录来自 XDG/HOME，非请求路径；0750 限制同用户组外访问。
	if err := os.MkdirAll(dir, 0o750); err != nil { //nolint:gosec // G703: trusted config home, not user path input
		return "", err
	}
	return dir, nil
}

// migrateLegacyDir 把旧版配置目录（~/.config/nomad-manager）迁移到新目录。
// 仅当旧目录存在且新目录不存在时执行；新目录已存在则跳过，绝不覆盖用户数据。
// base/newDir 来自 XDG/HOME 与固定常量，非用户输入路径（与 DefaultDir 同源）。
func migrateLegacyDir(base, newDir string) error {
	legacy := filepath.Join(base, legacyDirName)
	if _, err := os.Stat(legacy); err != nil { //nolint:gosec // G703: trusted config home, not user path input
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if _, err := os.Stat(newDir); err == nil { //nolint:gosec // G703: trusted config home, not user path input
		return nil // 新目录已有内容，保留旧目录不动
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(legacy, newDir) //nolint:gosec // G703: trusted config home, not user path input
}

// DefaultPath 返回默认的集群配置文件路径
func DefaultPath() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "clusters.json"), nil
}

// New 创建一个 Store（需调用 Load 后使用）。
func New(path string) *Store {
	return &Store{path: path}
}

// MustDefaultPath 返回默认路径，失败时 panic（仅限启动期使用）。
func MustDefaultPath() string {
	p, err := DefaultPath()
	if err != nil {
		panic(err)
	}
	return p
}

// Path 返回配置文件路径。
func (s *Store) Path() string { return s.path }

// Load 从磁盘读取；文件不存在视为空配置（幂等）。
func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *Store) loadLocked() error {
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		s.clusters = map[string]*ClusterConfig{}
		s.loaded = true
		return nil
	}
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	var list []*ClusterConfig
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	m := make(map[string]*ClusterConfig, len(list))
	for _, c := range list {
		m[c.ID] = c
	}
	s.clusters = m
	s.loaded = true
	return nil
}

// saveLocked 原子写盘（tmp + rename），需持锁。
func (s *Store) saveLocked() error {
	list := make([]*ClusterConfig, 0, len(s.clusters))
	for _, c := range s.clusters {
		list = append(list, c)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].ID < list[j].ID })
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// ensureLoaded 在写操作前保证已加载。
func (s *Store) ensureLoaded() error {
	if !s.loaded {
		return s.loadLocked()
	}
	return nil
}

// List 返回全部集群（ID 排序，拷贝防篡改）。
func (s *Store) List() ([]*ClusterConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLoaded(); err != nil {
		return nil, err
	}
	out := make([]*ClusterConfig, 0, len(s.clusters))
	for _, c := range s.clusters {
		cp := *c
		out = append(out, &cp)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Get 按 ID 取集群。
func (s *Store) Get(id string) (*ClusterConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLoaded(); err != nil {
		return nil, err
	}
	c, ok := s.clusters[id]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrClusterNotFound, id)
	}
	cp := *c
	return &cp, nil
}

// Add 新增集群；ID 冲突返回 ErrClusterExists。
func (s *Store) Add(c *ClusterConfig) error {
	if c == nil || c.ID == "" {
		return errors.New("invalid cluster config")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	if _, ok := s.clusters[c.ID]; ok {
		return fmt.Errorf("%w: %s", ErrClusterExists, c.ID)
	}
	cp := *c
	s.clusters[c.ID] = &cp
	return s.saveLocked()
}

// Update 更新已有集群（ID 不变）。
func (s *Store) Update(c *ClusterConfig) error {
	if c == nil || c.ID == "" {
		return errors.New("invalid cluster config")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	if _, ok := s.clusters[c.ID]; !ok {
		return fmt.Errorf("%w: %s", ErrClusterNotFound, c.ID)
	}
	cp := *c
	s.clusters[c.ID] = &cp
	return s.saveLocked()
}

// Delete 删除集群。
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	if _, ok := s.clusters[id]; !ok {
		return fmt.Errorf("%w: %s", ErrClusterNotFound, id)
	}
	delete(s.clusters, id)
	return s.saveLocked()
}
