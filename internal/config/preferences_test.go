package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testPrefs(t *testing.T) *PrefsStore {
	t.Helper()
	p := NewPrefs(filepath.Join(t.TempDir(), "preferences.json"))
	if err := p.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return p
}

func mustGetActive(t *testing.T, p *PrefsStore) string {
	t.Helper()
	got, err := p.GetActive()
	if err != nil {
		t.Fatalf("GetActive: %v", err)
	}
	return got
}

// TestPrefsSetGetClearActive 验证 active 的写入/读取/清空闭环。
func TestPrefsSetGetClearActive(t *testing.T) {
	p := testPrefs(t)

	if got := mustGetActive(t, p); got != "" {
		t.Fatalf("initial active = %q, want empty", got)
	}
	if err := p.SetActive("prod-east"); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	if got := mustGetActive(t, p); got != "prod-east" {
		t.Fatalf("active = %q, want prod-east", got)
	}
	if err := p.ClearActive(); err != nil {
		t.Fatalf("ClearActive: %v", err)
	}
	if got := mustGetActive(t, p); got != "" {
		t.Fatalf("active after clear = %q, want empty", got)
	}
}

// TestPrefsRoundTrip 验证落盘后可被新实例读回（重启恢复的核心）。
func TestPrefsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	p1 := NewPrefs(path)
	if err := p1.Load(); err != nil {
		t.Fatal(err)
	}
	if err := p1.SetActive("dev"); err != nil {
		t.Fatal(err)
	}

	p2 := NewPrefs(path)
	if err := p2.Load(); err != nil {
		t.Fatal(err)
	}
	if got := mustGetActive(t, p2); got != "dev" {
		t.Fatalf("reloaded active = %q, want dev", got)
	}
}

// TestPrefsFileMode 验证偏好文件权限 0600（含 active 的会话信息不应宽松可读）。
func TestPrefsFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	p := NewPrefs(path)
	if err := p.Load(); err != nil {
		t.Fatal(err)
	}
	if err := p.SetActive("x"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
	// 原子写不留 .tmp 残留
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("leftover .tmp file")
	}
}

// TestPrefsMissingFileIsEmpty 验证文件不存在时 Load 幂等为空、且不报错。
func TestPrefsMissingFileIsEmpty(t *testing.T) {
	p := NewPrefs(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err := p.Load(); err != nil {
		t.Fatalf("Load on missing file should be idempotent-empty: %v", err)
	}
	if got := mustGetActive(t, p); got != "" {
		t.Fatalf("active = %q, want empty", got)
	}
}

// TestPrefsCorruptFile 验证坏文件 Load 报错（启动时不应静默吞掉）。
func TestPrefsCorruptFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := NewPrefs(path)
	if err := p.Load(); err == nil {
		t.Fatal("want error for corrupt prefs file")
	}
}

// TestPrefsGetActiveSurfacesLoadError 懒加载遇到坏文件时必须返回 error（不得静默空串）。
func TestPrefsGetActiveSurfacesLoadError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := NewPrefs(path) // 未 Load
	got, err := p.GetActive()
	if err == nil {
		t.Fatal("want GetActive error for corrupt prefs")
	}
	if got != "" {
		t.Fatalf("got = %q, want empty on error", got)
	}
}

// TestPrefsIgnoresUnknownFields 验证未来版本新增字段不会破坏旧文件读取。
func TestPrefsIgnoresUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences.json")
	raw := `{"activeClusterID":"a","futureField":"b"}`
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	p := NewPrefs(path)
	if err := p.Load(); err != nil {
		t.Fatal(err)
	}
	if got := mustGetActive(t, p); got != "a" {
		t.Fatalf("active = %q, want a", got)
	}
	// 保存时不应带上未知字段
	if err := p.SetActive("b"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "futureField") {
		t.Fatal("unknown field leaked into saved prefs")
	}
}
