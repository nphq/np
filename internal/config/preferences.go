package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Preferences 是应用级会话偏好（活跃集群等），与集群列表分离存储。
// 活跃 ID 是会话偏好而非集群属性：删除集群时清理逻辑简单，也不怕误写进
// 备份/分享用的 clusters.json。token 绝不存于此。
type Preferences struct {
	ActiveClusterID string `json:"activeClusterID,omitempty"`
	LastActiveAt    int64  `json:"lastActiveAt,omitempty"` // unix seconds，仅用于诊断
}

// PrefsPath 返回默认偏好文件路径：<DefaultDir>/preferences.json。
func PrefsPath() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "preferences.json"), nil
}

// MustPrefsPath 返回默认偏好路径，失败时 panic（仅限启动期使用）。
func MustPrefsPath() string {
	p, err := PrefsPath()
	if err != nil {
		panic(err)
	}
	return p
}

// PrefsStore 管理 preferences.json（0600，原子写）。
type PrefsStore struct {
	mu     sync.RWMutex
	path   string
	prefs  Preferences
	loaded bool
}

// NewPrefs 创建一个 PrefsStore（需调用 Load 后使用）。
func NewPrefs(path string) *PrefsStore {
	return &PrefsStore{path: path}
}

// Path 返回偏好文件路径。
func (s *PrefsStore) Path() string { return s.path }

// Load 从磁盘读取；文件不存在视为空偏好（幂等）。
func (s *PrefsStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *PrefsStore) loadLocked() error {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		s.prefs = Preferences{}
		s.loaded = true
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &s.prefs); err != nil {
		return err
	}
	s.loaded = true
	return nil
}

// saveLocked 原子写盘（tmp + rename），权限 0600，需持锁。
func (s *PrefsStore) saveLocked() error {
	data, err := json.MarshalIndent(s.prefs, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o750); err != nil { //nolint:gosec // 同 store.go
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *PrefsStore) ensureLoaded() error {
	if !s.loaded {
		return s.loadLocked()
	}
	return nil
}

// GetActive 返回当前活跃集群 ID（可能为空）。
// 懒加载会写 loaded/prefs，必须持写锁；加载失败时返回 error（不静默吞掉）。
func (s *PrefsStore) GetActive() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLoaded(); err != nil {
		return "", err
	}
	return s.prefs.ActiveClusterID, nil
}

// SetActive 记录活跃集群 ID 并落盘。
func (s *PrefsStore) SetActive(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	s.prefs.ActiveClusterID = id
	s.prefs.LastActiveAt = time.Now().Unix()
	return s.saveLocked()
}

// ClearActive 清空活跃集群 ID 并落盘。
func (s *PrefsStore) ClearActive() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	s.prefs.ActiveClusterID = ""
	return s.saveLocked()
}
