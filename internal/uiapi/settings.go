package uiapi

import (
	"github.com/nphq/np/internal/config"
)

// SettingsInput 是 UpdateSettings 的前端入参（与 config.AppSettings 同构，
// 单独定义避免前端直接依赖 config 包路径变更）。
type SettingsInput struct {
	ConfirmDestructive bool   `json:"confirmDestructive"`
	AutoRestoreActive  bool   `json:"autoRestoreActive"`
	HealthIntervalSec  int    `json:"healthIntervalSec"`
	MetricsIntervalSec int    `json:"metricsIntervalSec"`
	DefaultRegion      string `json:"defaultRegion"`
	DefaultNamespace   string `json:"defaultNamespace"`
}

// ConfigPaths 是数据目录诊断信息（设置页展示 + 一键复制）。
type ConfigPaths struct {
	ConfigDir   string `json:"configDir"`
	Clusters    string `json:"clusters"`
	Preferences string `json:"preferences"`
}

// SettingsService 承载通用设置 IPC：读/写/重置 + 路径诊断。
// 写操作会同步应用到 ClusterService（健康轮询）与 LoadsService（指标轮询）。
type SettingsService struct {
	prefs    *config.PrefsStore
	clusters *ClusterService
	loads    *LoadsService
}

// NewSettingsService 创建设置服务；clusters/loads 可为 nil（仅读写文件时）。
func NewSettingsService(prefs *config.PrefsStore, clusters *ClusterService, loads *LoadsService) *SettingsService {
	return &SettingsService{prefs: prefs, clusters: clusters, loads: loads}
}

var (
	allowedHealth  = map[int]bool{10: true, 15: true, 30: true, 60: true, 120: true}
	allowedMetrics = map[int]bool{5: true, 10: true, 15: true, 30: true, 60: true}
)

// GetSettings 返回归一化设置。
func (s *SettingsService) GetSettings() (config.AppSettings, *Error) {
	if s.prefs == nil {
		return config.DefaultSettings(), nil
	}
	st, err := s.prefs.GetSettings()
	if err != nil {
		return config.DefaultSettings(), Wrap(err)
	}
	return st, nil
}

// UpdateSettings 校验并落盘，同时应用轮询间隔到后台服务。
func (s *SettingsService) UpdateSettings(in SettingsInput) *Error {
	if !allowedHealth[in.HealthIntervalSec] {
		return NewError(CodeInvalidInput, "invalid health interval: %d (allowed: 10/15/30/60/120)", in.HealthIntervalSec)
	}
	if !allowedMetrics[in.MetricsIntervalSec] {
		return NewError(CodeInvalidInput, "invalid metrics interval: %d (allowed: 5/10/15/30/60)", in.MetricsIntervalSec)
	}
	if err := ValidateRegion(in.DefaultRegion); err != nil {
		return NewError(CodeInvalidInput, "%v", err)
	}
	if err := ValidateNamespace(in.DefaultNamespace); err != nil {
		return NewError(CodeInvalidInput, "%v", err)
	}
	st := config.AppSettings{
		ConfirmDestructive: in.ConfirmDestructive,
		AutoRestoreActive:  in.AutoRestoreActive,
		HealthIntervalSec:  in.HealthIntervalSec,
		MetricsIntervalSec: in.MetricsIntervalSec,
		DefaultRegion:      in.DefaultRegion,
		DefaultNamespace:   in.DefaultNamespace,
	}
	if s.prefs != nil {
		if err := s.prefs.UpdateSettings(st); err != nil {
			return Wrap(err)
		}
	}
	s.applyIntervals(st)
	return nil
}

// ResetSettings 恢复出厂并应用。
func (s *SettingsService) ResetSettings() (config.AppSettings, *Error) {
	d := config.DefaultSettings()
	if s.prefs != nil {
		if err := s.prefs.ResetSettings(); err != nil {
			return d, Wrap(err)
		}
	}
	s.applyIntervals(d)
	return d, nil
}

// applyIntervals 把轮询间隔推送到后台服务（nil-safe，便于单测）。
func (s *SettingsService) applyIntervals(st config.AppSettings) {
	if s.clusters != nil {
		s.clusters.SetHealthInterval(st.HealthIntervalSec)
	}
	if s.loads != nil {
		s.loads.SetMetricsInterval(st.MetricsIntervalSec)
	}
}

// GetConfigPaths 返回配置文件路径（诊断/备份指引用，不含敏感 token）。
func (s *SettingsService) GetConfigPaths() (ConfigPaths, *Error) {
	dir, err := config.DefaultDir()
	if err != nil {
		return ConfigPaths{}, Wrap(err)
	}
	clusters, err := config.DefaultPath()
	if err != nil {
		return ConfigPaths{}, Wrap(err)
	}
	prefs, err := config.PrefsPath()
	if err != nil {
		return ConfigPaths{}, Wrap(err)
	}
	return ConfigPaths{ConfigDir: dir, Clusters: clusters, Preferences: prefs}, nil
}
