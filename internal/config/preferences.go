package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Preferences 是应用级会话偏好（活跃集群 + 通用设置），与集群列表分离存储。
// 活跃 ID 是会话偏好而非集群属性：删除集群时清理逻辑简单，也不怕误写进
// 备份/分享用的 clusters.json。token 绝不存于此。
type Preferences struct {
	ActiveClusterID string `json:"activeClusterID,omitempty"`
	LastActiveAt    int64  `json:"lastActiveAt,omitempty"` // unix seconds，仅用于诊断
	// Settings 为空表示从未保存过，读取时回落 DefaultSettings。
	Settings *AppSettings `json:"settings,omitempty"`
}

// AppSettings 是跨重启持久化的通用行为设置（设置页 General/集群默认值）。
// 外观（主题/字体/字号）与语言存前端 localStorage（即时生效、无需 IPC），
// 此处只存需要后端生效或跨端一致的行为项。
type AppSettings struct {
	// ConfirmDestructive 写操作（删集群/stop job 等）前是否二次确认，默认 true。
	ConfirmDestructive bool `json:"confirmDestructive"`
	// AutoRestoreActive 启动时是否恢复上次活跃集群，默认 true。
	AutoRestoreActive bool `json:"autoRestoreActive"`
	// HealthIntervalSec 健康轮询秒数，默认 30（允许 10/15/30/60/120）。
	HealthIntervalSec int `json:"healthIntervalSec"`
	// MetricsIntervalSec 负载轮询秒数，默认 15（允许 5/10/15/30/60）。
	MetricsIntervalSec int `json:"metricsIntervalSec"`
	// DefaultRegion/DefaultNamespace 新建集群表单的预填默认值（可空）。
	DefaultRegion    string `json:"defaultRegion,omitempty"`
	DefaultNamespace string `json:"defaultNamespace,omitempty"`
}

// DefaultSettings 返回出厂默认值（Settings 为 nil 或字段越界时的回落）。
func DefaultSettings() AppSettings {
	return AppSettings{
		ConfirmDestructive: true,
		AutoRestoreActive:  true,
		HealthIntervalSec:  30,
		MetricsIntervalSec: 15,
	}
}

// Normalize 用默认值补齐越界/零值（向前兼容旧文件）。
func (s AppSettings) Normalize() AppSettings {
	d := DefaultSettings()
	if s.HealthIntervalSec <= 0 {
		s.HealthIntervalSec = d.HealthIntervalSec
	}
	if s.MetricsIntervalSec <= 0 {
		s.MetricsIntervalSec = d.MetricsIntervalSec
	}
	return s
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
// 快路径 RLock；冷启动升级写锁做懒加载。
func (s *PrefsStore) GetActive() (string, error) {
	s.mu.RLock()
	if s.loaded {
		id := s.prefs.ActiveClusterID
		s.mu.RUnlock()
		return id, nil
	}
	s.mu.RUnlock()
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

// GetSettings 返回归一化后的设置（从未保存回落默认值，不写盘）。
func (s *PrefsStore) GetSettings() (AppSettings, error) {
	s.mu.RLock()
	if s.loaded {
		st := s.prefs.Settings
		s.mu.RUnlock()
		if st == nil {
			return DefaultSettings(), nil
		}
		return st.Normalize(), nil
	}
	s.mu.RUnlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLoaded(); err != nil {
		return DefaultSettings(), err
	}
	if s.prefs.Settings == nil {
		return DefaultSettings(), nil
	}
	return s.prefs.Settings.Normalize(), nil
}

// UpdateSettings 全量覆盖设置并落盘（调用方已校验）。
func (s *PrefsStore) UpdateSettings(st AppSettings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLoaded(); err != nil {
		return err
	}
	cp := st.Normalize()
	s.prefs.Settings = &cp
	return s.saveLocked()
}

// ResetSettings 恢复出厂默认值并落盘。
func (s *PrefsStore) ResetSettings() error {
	return s.UpdateSettings(DefaultSettings())
}
