package uiapi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// clearEnv 清掉全部 NOMAD_* 环境变量，避免本机开发者环境串扰测试。
func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{"NOMAD_ADDR", "NOMAD_TOKEN", "NOMAD_REGION", "NOMAD_NAMESPACE", "NOMAD_SKIP_VERIFY", "NOMAD_CACERT"} {
		t.Setenv(k, "")
	}
}

// TestDiscoverClusters_NoEnv 无 NOMAD_ADDR 时返回空列表、不报错。
func TestDiscoverClusters_NoEnv(t *testing.T) {
	clearEnv(t)
	svc, _, _ := testService(t)
	list, e := svc.DiscoverClusters()
	if e != nil {
		t.Fatalf("no env should not error: %v", e)
	}
	if len(list) != 0 {
		t.Fatalf("want empty list, got %+v", list)
	}
}

// TestDiscoverClusters_EnvCombos 表驱动：各种 env 组合 → Discover 结果。
func TestDiscoverClusters_EnvCombos(t *testing.T) {
	clearEnv(t)
	svc, _, _ := testService(t)

	cases := []struct {
		name string
		env  map[string]string
		want DiscoveredCluster
	}{
		{
			name: "addr only",
			env:  map[string]string{"NOMAD_ADDR": "127.0.0.1:4646"},
			want: DiscoveredCluster{
				Source: SourceEnv, SuggestedID: "local",
				Name: "From environment", Address: "127.0.0.1:4646",
			},
		},
		{
			name: "full with token",
			env: map[string]string{
				"NOMAD_ADDR": "http://prod:4646", "NOMAD_TOKEN": "secret",
				"NOMAD_REGION": "us-east", "NOMAD_NAMESPACE": "team-a",
			},
			want: DiscoveredCluster{
				Source: SourceEnv, SuggestedID: "local",
				Name: "From environment", Address: "http://prod:4646",
				Region: "us-east", Namespace: "team-a", HasToken: true,
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			list, e := svc.DiscoverClusters()
			if e != nil {
				t.Fatal(e)
			}
			if len(list) != 1 {
				t.Fatalf("want 1 discovery, got %+v", list)
			}
			d := list[0]
			if d.Source != c.want.Source || d.Address != c.want.Address ||
				d.Region != c.want.Region || d.Namespace != c.want.Namespace ||
				d.HasToken != c.want.HasToken || d.TLS != c.want.TLS ||
				d.InsecureSkipVerify != c.want.InsecureSkipVerify {
				t.Fatalf("discovery = %+v, want %+v", d, c.want)
			}
			// token 明文绝不能出现在 Discover 响应里
			for _, f := range []string{d.Address, d.Name, d.SuggestedID, d.Region, d.Namespace} {
				if strings.Contains(f, "secret") {
					t.Fatalf("token leaked into discovery field %q", f)
				}
			}
		})
	}
}

// TestDiscoverClusters_SkipVerify 表驱动 NOMAD_SKIP_VERIFY 取值。
func TestDiscoverClusters_SkipVerify(t *testing.T) {
	clearEnv(t)
	svc, _, _ := testService(t)

	for _, v := range []string{"true", "1", "yes", "TRUE", "Yes"} {
		t.Setenv("NOMAD_ADDR", "https://x:4646")
		t.Setenv("NOMAD_SKIP_VERIFY", v)
		list, _ := svc.DiscoverClusters()
		if len(list) != 1 || !list[0].TLS || !list[0].InsecureSkipVerify {
			t.Fatalf("SKIP_VERIFY=%q: got %+v, want TLS+skip", v, list)
		}
	}
	t.Setenv("NOMAD_SKIP_VERIFY", "0")
	list, _ := svc.DiscoverClusters()
	if len(list) != 1 || list[0].TLS || list[0].InsecureSkipVerify {
		t.Fatalf("SKIP_VERIFY=0: got %+v, want no TLS", list)
	}
	t.Setenv("NOMAD_SKIP_VERIFY", "")
	list, _ = svc.DiscoverClusters()
	if list[0].TLS {
		t.Fatalf("SKIP_VERIFY empty: got %+v, want no TLS", list)
	}
}

// TestDiscoverSuggestedID 已存在 local 时建议 local-2，依次递增。
func TestDiscoverSuggestedID(t *testing.T) {
	clearEnv(t)
	svc, _, _ := testService(t)
	t.Setenv("NOMAD_ADDR", "http://x:4646")

	list, _ := svc.DiscoverClusters()
	if list[0].SuggestedID != "local" {
		t.Fatalf("suggested = %q, want local", list[0].SuggestedID)
	}
	_ = svc.AddCluster(ClusterInput{ID: "local", Name: "L", Address: "http://x:4646"})
	_ = svc.AddCluster(ClusterInput{ID: "local-2", Name: "L2", Address: "http://y:4646"})
	list, _ = svc.DiscoverClusters()
	if list[0].SuggestedID != "local-3" {
		t.Fatalf("suggested = %q, want local-3", list[0].SuggestedID)
	}
}

// TestImportFromEnv_NoEnv 无 NOMAD_ADDR 时导入报错且无副作用。
func TestImportFromEnv_NoEnv(t *testing.T) {
	clearEnv(t)
	svc, store, _ := testService(t)
	if _, e := svc.ImportFromEnv(""); e == nil || e.Code != CodeNotFound {
		t.Fatalf("want not_found, got %+v", e)
	}
	if cfgs, _ := store.List(); len(cfgs) != 0 {
		t.Fatal("ImportFromEnv with no env must not create clusters")
	}
}

// TestImportFromEnv_CreatesAndActivates 一键导入：创建、激活、token 进 Keychain。
func TestImportFromEnv_CreatesAndActivates(t *testing.T) {
	clearEnv(t)
	svc, store, kr := testService(t)
	t.Setenv("NOMAD_ADDR", "http://127.0.0.1:4646")
	t.Setenv("NOMAD_TOKEN", "env-secret")
	t.Setenv("NOMAD_REGION", "global")

	info, e := svc.ImportFromEnv("")
	if e != nil {
		t.Fatalf("ImportFromEnv: %v", e)
	}
	if info == nil || info.ID != "local" || info.Name != "From environment" {
		t.Fatalf("info = %+v", info)
	}
	if svc.ActiveCluster() != "local" {
		t.Fatalf("active = %q, want local", svc.ActiveCluster())
	}
	// Address 补全 http://
	if info.Address != "http://127.0.0.1:4646" {
		t.Fatalf("address = %q", info.Address)
	}
	// token 只进 Keychain
	tok, err := kr.GetToken("local")
	if err != nil || tok != "env-secret" {
		t.Fatalf("keychain token = %q, %v", tok, err)
	}
	raw, _ := os.ReadFile(store.Path())
	if strings.Contains(string(raw), "env-secret") {
		t.Fatal("token leaked into clusters.json")
	}
	// 配置落盘且 id 正确
	if _, err := store.Get("local"); err != nil {
		t.Fatalf("config not saved: %v", err)
	}
}

// TestImportFromEnv_DedupeByAddress 同 Address 已存在 → 不重复创建，激活已有；
// 若 env 带 token，刷新 Keychain（轮换后 re-import 应对齐）。
func TestImportFromEnv_DedupeByAddress(t *testing.T) {
	clearEnv(t)
	svc, store, kr := testService(t)
	_ = svc.AddCluster(ClusterInput{ID: "existing", Name: "E", Address: "http://127.0.0.1:4646", Token: "old-token"})
	t.Setenv("NOMAD_ADDR", "127.0.0.1:4646") // 无 scheme，应匹配规范化地址
	t.Setenv("NOMAD_TOKEN", "new-env-token")

	info, e := svc.ImportFromEnv("Import Me")
	if e != nil {
		t.Fatalf("ImportFromEnv: %v", e)
	}
	if info.ID != "existing" {
		t.Fatalf("should activate existing, got %+v", info)
	}
	if svc.ActiveCluster() != "existing" {
		t.Fatalf("active = %q, want existing", svc.ActiveCluster())
	}
	cfgs, _ := store.List()
	if len(cfgs) != 1 {
		t.Fatalf("must not create duplicate: %+v", cfgs)
	}
	tok, err := kr.GetToken("existing")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if tok != "new-env-token" {
		t.Fatalf("token = %q, want refreshed new-env-token", tok)
	}
}

// TestImportClusterJSON 数组/单对象导入；坏 JSON 报错；token 字段被忽略。
func TestImportClusterJSON(t *testing.T) {
	clearEnv(t)
	svc, store, kr := testService(t)

	// 单对象
	raw := `{"id":"j1","name":"J1","address":"http://a:4646","token":"should-be-ignored"}`
	infos, e := svc.ImportClusterJSON(raw)
	if e != nil {
		t.Fatalf("ImportClusterJSON single: %v", e)
	}
	if len(infos) != 1 || infos[0].ID != "j1" {
		t.Fatalf("infos = %+v", infos)
	}
	if svc.ActiveCluster() != "j1" {
		t.Fatalf("active = %q, want j1", svc.ActiveCluster())
	}
	// token 字段绝不进 Keychain（文件导入没有可信 token 通道）
	if _, err := kr.GetToken("j1"); err == nil {
		t.Fatal("file import must not read token field")
	}

	// 数组（含一个无效条目：缺 address → 跳过）
	raw2 := `[{"id":"a1","name":"A","address":"http://b:4646"},{"id":"bad","name":"B"}]`
	infos, e = svc.ImportClusterJSON(raw2)
	if e != nil {
		t.Fatalf("ImportClusterJSON array: %v", e)
	}
	if len(infos) != 1 || infos[0].ID != "a1" {
		t.Fatalf("infos = %+v", infos)
	}
	if svc.ActiveCluster() != "a1" {
		t.Fatalf("active = %q, want a1", svc.ActiveCluster())
	}
	if _, err := store.Get("bad"); err == nil {
		t.Fatal("invalid entry must be skipped")
	}

	// 坏 JSON
	if _, e := svc.ImportClusterJSON("{not-json"); e == nil || e.Code != CodeInvalidInput {
		t.Fatalf("want invalid_input for bad JSON, got %+v", e)
	}
}

// TestImportClusterJSON_IDConflictSkipped 同 ID、不同 Address → 跳过冲突条目，
// 不得改写成 prod-2 后偷偷创建（§5.4）。
func TestImportClusterJSON_IDConflictSkipped(t *testing.T) {
	clearEnv(t)
	svc, store, _ := testService(t)
	_ = svc.AddCluster(ClusterInput{ID: "prod", Name: "Old", Address: "http://old:4646"})

	// 仅冲突条目 → 无可导入
	if _, e := svc.ImportClusterJSON(`{"id":"prod","name":"New","address":"http://new:4646"}`); e == nil || e.Code != CodeInvalidInput {
		t.Fatalf("want invalid_input when only conflict, got %+v", e)
	}
	if _, err := store.Get("prod-2"); err == nil {
		t.Fatal("must not create prod-2 for ID conflict")
	}
	cfg, err := store.Get("prod")
	if err != nil || cfg.Address != "http://old:4646" {
		t.Fatalf("existing prod must be unchanged: %+v %v", cfg, err)
	}

	// 冲突 + 合法 → 只导入合法，不改写冲突 ID
	infos, e := svc.ImportClusterJSON(`[
		{"id":"prod","name":"New","address":"http://new:4646"},
		{"id":"ok","name":"OK","address":"http://ok:4646"}
	]`)
	if e != nil {
		t.Fatalf("ImportClusterJSON: %v", e)
	}
	if len(infos) != 1 || infos[0].ID != "ok" {
		t.Fatalf("infos = %+v, want only ok", infos)
	}
	if _, err := store.Get("prod-2"); err == nil {
		t.Fatal("must not create prod-2 alongside ok")
	}
	cfgs, _ := store.List()
	if len(cfgs) != 2 {
		t.Fatalf("want 2 clusters (prod+ok), got %+v", cfgs)
	}
}

// TestPinCluster 收藏置顶：ListClusters 排序 Pinned 在前；编辑不清收藏。
func TestPinCluster(t *testing.T) {
	clearEnv(t)
	svc, _, _ := testService(t)
	_ = svc.AddCluster(ClusterInput{ID: "z", Name: "Zeta", Address: "http://z:4646"})
	_ = svc.AddCluster(ClusterInput{ID: "a", Name: "Alpha", Address: "http://a:4646"})

	list, _ := svc.ListClusters()
	if list.Clusters[0].ID != "a" {
		t.Fatalf("id-sorted = %+v", list.Clusters)
	}

	// 置顶 zeta
	if e := svc.PinCluster("z", true); e != nil {
		t.Fatalf("PinCluster: %v", e)
	}
	list, _ = svc.ListClusters()
	if len(list.Clusters) != 2 || list.Clusters[0].ID != "z" || !list.Clusters[0].Pinned {
		t.Fatalf("pinned first = %+v", list.Clusters)
	}
	if list.Clusters[1].Pinned {
		t.Fatal("only z should be pinned")
	}

	// 取消置顶 → 恢复字典序
	if e := svc.PinCluster("z", false); e != nil {
		t.Fatal(e)
	}
	list, _ = svc.ListClusters()
	if list.Clusters[0].ID != "a" {
		t.Fatalf("unpinned order = %+v", list.Clusters)
	}

	// 编辑集群不丢 Pinned
	_ = svc.PinCluster("a", true)
	if e := svc.UpdateCluster(ClusterInput{ID: "a", Name: "Alpha2", Address: "http://a:4646"}); e != nil {
		t.Fatal(e)
	}
	list, _ = svc.ListClusters()
	if !list.Clusters[0].Pinned || list.Clusters[0].Name != "Alpha2" {
		t.Fatalf("pin lost on update = %+v", list.Clusters)
	}
}

// TestPinClusterSurvivesRestart 置顶在重载 Store 后仍保留（重启持久化）。
func TestPinClusterSurvivesRestart(t *testing.T) {
	clearEnv(t)
	cfgPath := filepath.Join(t.TempDir(), "clusters.json")
	prefsPath := filepath.Join(t.TempDir(), "preferences.json")
	svc, _, _ := testServiceAt(t, cfgPath, prefsPath)
	_ = svc.AddCluster(ClusterInput{ID: "c1", Name: "C", Address: "http://x:4646"})
	_ = svc.PinCluster("c1", true)

	// 新服务读同一文件
	svc2, _, _ := testServiceAt(t, cfgPath, prefsPath)
	list, e := svc2.ListClusters()
	if e != nil {
		t.Fatal(e)
	}
	if len(list.Clusters) != 1 || !list.Clusters[0].Pinned {
		t.Fatalf("pin lost across restart: %+v", list.Clusters)
	}
}

// TestListClustersActiveID ListClusters 响应带 activeID（前端单一数据源）。
func TestListClustersActiveID(t *testing.T) {
	clearEnv(t)
	svc, _, _ := testService(t)
	_ = svc.AddCluster(ClusterInput{ID: "c1", Name: "C1", Address: "http://x:4646"})
	_ = svc.AddCluster(ClusterInput{ID: "c2", Name: "C2", Address: "http://y:4646"})

	list, _ := svc.ListClusters()
	if list.ActiveID != "" {
		t.Fatalf("activeID before set = %q", list.ActiveID)
	}
	_ = svc.SetActiveCluster("c2")
	list, _ = svc.ListClusters()
	if list.ActiveID != "c2" {
		t.Fatalf("activeID = %q, want c2", list.ActiveID)
	}
}

// TestSetActivePersistsPrefs SetActiveCluster 成功后 prefs 落盘。
func TestSetActivePersistsPrefs(t *testing.T) {
	clearEnv(t)
	svc, _, _ := testService(t)
	_ = svc.AddCluster(ClusterInput{ID: "c1", Name: "C", Address: "http://x:4646"})
	if e := svc.SetActiveCluster("c1"); e != nil {
		t.Fatal(e)
	}
	if got, err := svc.prefs.GetActive(); err != nil || got != "c1" {
		t.Fatalf("prefs active = %q err=%v, want c1", got, err)
	}
}

// TestRestoreActive 重启模拟：prefs 有 active → 新服务 RestoreActive 恢复；
// 集群已删 → 清空偏好。
func TestRestoreActive(t *testing.T) {
	clearEnv(t)
	cfgPath := filepath.Join(t.TempDir(), "clusters.json")
	prefsPath := filepath.Join(t.TempDir(), "preferences.json")
	svc, _, _ := testServiceAt(t, cfgPath, prefsPath)
	_ = svc.AddCluster(ClusterInput{ID: "c1", Name: "C", Address: "http://x:4646"})
	_ = svc.SetActiveCluster("c1")

	// 模拟重启：同一 prefs/存储路径重建服务
	svc2, _, _ := testServiceAt(t, cfgPath, prefsPath)
	var restored string
	svc2.OnActiveChanged = func(id string) { restored = id }
	svc2.RestoreActive()
	if svc2.ActiveCluster() != "c1" || restored != "c1" {
		t.Fatalf("restore: active=%q restored=%q", svc2.ActiveCluster(), restored)
	}

	// 集群已被删 → RestoreActive 清空偏好
	if e := svc2.RemoveCluster("c1"); e != nil {
		t.Fatal(e)
	}
	svc3, _, _ := testServiceAt(t, cfgPath, prefsPath)
	svc3.OnActiveChanged = func(id string) { restored = id }
	svc3.RestoreActive()
	if svc3.ActiveCluster() != "" {
		t.Fatalf("active after restore of missing cluster = %q", svc3.ActiveCluster())
	}
	if got, err := svc3.prefs.GetActive(); err != nil || got != "" {
		t.Fatalf("prefs should be cleared, got %q err=%v", got, err)
	}
}

// TestRemoveActiveFallsBackToPinned 删除活跃集群 → 回退到下一个 Pinned（§5.2）。
func TestRemoveActiveFallsBackToPinned(t *testing.T) {
	clearEnv(t)
	svc, _, _ := testService(t)
	_ = svc.AddCluster(ClusterInput{ID: "a", Name: "A", Address: "http://a:4646"})
	_ = svc.AddCluster(ClusterInput{ID: "b", Name: "B", Address: "http://b:4646"})
	_ = svc.PinCluster("b", true)
	_ = svc.SetActiveCluster("a")

	if e := svc.RemoveCluster("a"); e != nil {
		t.Fatal(e)
	}
	if got := svc.ActiveCluster(); got != "b" {
		t.Fatalf("fallback active = %q, want pinned b", got)
	}
}

// TestRemoveActiveFallsBackToFirst 无 Pinned 时回退到列表第一项。
func TestRemoveActiveFallsBackToFirst(t *testing.T) {
	clearEnv(t)
	svc, _, _ := testService(t)
	_ = svc.AddCluster(ClusterInput{ID: "b", Name: "B", Address: "http://b:4646"})
	_ = svc.AddCluster(ClusterInput{ID: "a", Name: "A", Address: "http://a:4646"})
	_ = svc.SetActiveCluster("b")

	if e := svc.RemoveCluster("b"); e != nil {
		t.Fatal(e)
	}
	if got := svc.ActiveCluster(); got != "a" {
		t.Fatalf("fallback active = %q, want first (a)", got)
	}
}

// TestRemoveLastClusterClearsActive 删除最后一个集群 → 清空 active，无空指针。
func TestRemoveLastClusterClearsActive(t *testing.T) {
	clearEnv(t)
	svc, _, _ := testService(t)
	_ = svc.AddCluster(ClusterInput{ID: "only", Name: "O", Address: "http://x:4646"})
	_ = svc.SetActiveCluster("only")

	if e := svc.RemoveCluster("only"); e != nil {
		t.Fatal(e)
	}
	if got := svc.ActiveCluster(); got != "" {
		t.Fatalf("active = %q, want empty", got)
	}
	if got, err := svc.prefs.GetActive(); err != nil || got != "" {
		t.Fatalf("prefs active = %q err=%v, want empty", got, err)
	}
}
