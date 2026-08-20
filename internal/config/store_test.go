package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func tempStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	return New(filepath.Join(dir, "clusters.json"))
}

func TestStoreAddListGet(t *testing.T) {
	s := tempStore(t)
	if err := s.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}

	c := &ClusterConfig{
		ID:      "dev-1",
		Name:    "Dev Cluster",
		Address: "http://127.0.0.1:4646",
		Region:  "global",
	}
	if err := s.Add(c); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s.Add(c); !errors.Is(err, ErrClusterExists) {
		t.Fatalf("Add dup: want ErrClusterExists, got %v", err)
	}

	got, err := s.Get("dev-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Address != "http://127.0.0.1:4646" {
		t.Fatalf("Get: address = %q", got.Address)
	}

	// 修改原对象不应影响 store 内部数据（拷贝）
	c.Name = "Mutated"
	got2, _ := s.Get("dev-1")
	if got2.Name == "Mutated" {
		t.Fatal("store leaks internal state")
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List: want 1, got %d", len(list))
	}
}

func TestStorePersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clusters.json")

	s1 := New(path)
	if err := s1.Add(&ClusterConfig{ID: "a", Name: "A", Address: "http://x:4646"}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	s2 := New(path)
	if err := s2.Load(); err != nil {
		t.Fatalf("Load s2: %v", err)
	}
	got, err := s2.Get("a")
	if err != nil {
		t.Fatalf("Get from s2: %v", err)
	}
	if got.Name != "A" {
		t.Fatalf("persist: name = %q", got.Name)
	}
}

func TestStoreUpdateDelete(t *testing.T) {
	s := tempStore(t)
	if err := s.Add(&ClusterConfig{ID: "a", Name: "A", Address: "http://x:4646"}); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := s.Update(&ClusterConfig{ID: "a", Name: "A2", Address: "http://x:4646"}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := s.Get("a")
	if got.Name != "A2" {
		t.Fatalf("Update: name = %q", got.Name)
	}
	if err := s.Delete("a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get("a"); !errors.Is(err, ErrClusterNotFound) {
		t.Fatalf("after delete: want ErrClusterNotFound, got %v", err)
	}
}

func TestStoreNoFileMeansEmpty(t *testing.T) {
	s := tempStore(t)
	if err := s.Load(); err != nil {
		t.Fatalf("Load on missing file: %v", err)
	}
	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("want 0 clusters, got %d", len(list))
	}
}

func TestStoreInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clusters.json")
	if err := os.WriteFile(path, []byte("{invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := New(path)
	if err := s.Load(); err == nil {
		t.Fatal("want error on invalid JSON")
	}
}

func TestStoreFilePerms(t *testing.T) {
	s := tempStore(t)
	if err := s.Add(&ClusterConfig{ID: "a", Name: "A", Address: "http://x:4646"}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config file perms = %v, want 0600", info.Mode().Perm())
	}
}

func TestDefaultDir(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir, err := DefaultDir()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(filepath.Base(dir)) != "np" {
		t.Fatalf("unexpected default dir: %s", dir)
	}
}

// TestDefaultDirMigratesLegacy 验证旧版 ~/.config/nomad-manager 目录被迁移到 np。
func TestDefaultDirMigratesLegacy(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	legacy := filepath.Join(base, "nomad-manager")
	if err := os.MkdirAll(legacy, 0o750); err != nil {
		t.Fatal(err)
	}
	payload := "[]"
	if err := os.WriteFile(filepath.Join(legacy, "clusters.json"), []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}

	dir, err := DefaultDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "np")
	if dir != want {
		t.Fatalf("dir = %s, want %s", dir, want)
	}
	// 旧目录内容必须搬到新目录，旧目录本身被移除。
	data, err := os.ReadFile(filepath.Join(want, "clusters.json"))
	if err != nil {
		t.Fatalf("migrated content missing: %v", err)
	}
	if string(data) != payload {
		t.Fatalf("migrated content = %q, want %q", data, payload)
	}
	if _, err := os.Stat(legacy); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("legacy dir still exists: %v", err)
	}
}

// TestDefaultDirSkipsMigrationWhenNewExists 新目录已存在时不得搬动旧目录。
func TestDefaultDirSkipsMigrationWhenNewExists(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	legacy := filepath.Join(base, "nomad-manager")
	if err := os.MkdirAll(legacy, 0o750); err != nil {
		t.Fatal(err)
	}
	newDir := filepath.Join(base, "np")
	if err := os.MkdirAll(newDir, 0o750); err != nil {
		t.Fatal(err)
	}

	dir, err := DefaultDir()
	if err != nil {
		t.Fatal(err)
	}
	if dir != newDir {
		t.Fatalf("dir = %s, want %s", dir, newDir)
	}
	// 旧目录保持原样（不覆盖、不搬移）。
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("legacy dir should be untouched: %v", err)
	}
}
