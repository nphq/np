package uiapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nphq/np/internal/config"
	"github.com/nphq/np/internal/secure"
)

func readFile(t *testing.T, path string) ([]byte, error) {
	t.Helper()
	return os.ReadFile(path)
}

func stringContains(s, sub string) bool {
	return strings.Contains(s, sub)
}

func testService(t *testing.T) (*ClusterService, *config.Store, *secure.MemoryKeyring) {
	t.Helper()
	store := config.New(filepath.Join(t.TempDir(), "clusters.json"))
	kr := secure.NewMemory()
	return NewClusterService(store, kr), store, kr
}

func nomadTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/status/leader":
			_, _ = w.Write([]byte(`"127.0.0.1:4647"`))
		case "/v1/agent/self":
			out := map[string]any{
				"config": map[string]any{"version": "1.9.4"},
			}
			b, _ := json.Marshal(out)
			_, _ = w.Write(b)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAddClusterValidation(t *testing.T) {
	svc, _, _ := testService(t)
	cases := []ClusterInput{
		{ID: "", Name: "n", Address: "http://x:4646"},
		{ID: "ok", Name: "n", Address: "bad address"},
		{ID: "ok", Name: "n", Address: "http://x:99999"},
		{ID: "ok", Name: "n", Address: "http://x:4646", Namespace: "a b"},
	}
	for i, c := range cases {
		if err := svc.AddCluster(c); err == nil {
			t.Errorf("case %d: want validation error", i)
		}
	}
}

func TestAddRemoveClusterLifecycle(t *testing.T) {
	svc, store, kr := testService(t)
	srv := nomadTestServer(t)

	in := ClusterInput{
		ID:      "dev-1",
		Name:    "Dev",
		Address: srv.URL,
		Token:   "secret-token",
	}
	if err := svc.AddCluster(in); err != nil {
		t.Fatalf("AddCluster: %v", err)
	}

	// 配置落盘且不含 token
	cfg, err := store.Get("dev-1")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Address != srv.URL {
		t.Fatalf("address = %q", cfg.Address)
	}
	data, _ := json.Marshal(cfg)
	if string(data) == "" {
		t.Fatal("unmarshalable")
	}
	// token 只在 Keychain，配置文件中绝不能出现
	raw, err := readFile(t, store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if stringContains(string(raw), "secret-token") {
		t.Fatal("token leaked into config file!")
	}

	tok, err := kr.GetToken("dev-1")
	if err != nil || tok != "secret-token" {
		t.Fatalf("keychain token = %q, %v", tok, err)
	}

	// 健康检查
	health, e := svc.TestConnection("dev-1")
	if e != nil {
		t.Fatalf("TestConnection: %v", e)
	}
	if health.Status != "ok" || health.Leader == "" || health.Version != "1.9.4" {
		t.Fatalf("health = %+v", health)
	}

	// 列表
	list, e := svc.ListClusters()
	if e != nil {
		t.Fatal(e)
	}
	if len(list) != 1 || list[0].ID != "dev-1" {
		t.Fatalf("list = %+v", list)
	}

	// 删除：配置 + keychain 都清
	if err := svc.RemoveCluster("dev-1"); err != nil {
		t.Fatalf("RemoveCluster: %v", err)
	}
	if _, err := store.Get("dev-1"); err == nil {
		t.Fatal("config not removed")
	}
	if _, err := kr.GetToken("dev-1"); err == nil {
		t.Fatal("token not removed")
	}
}

func TestAddClusterDuplicate(t *testing.T) {
	svc, _, _ := testService(t)
	in := ClusterInput{ID: "dup", Name: "D", Address: "http://x:4646"}
	if err := svc.AddCluster(in); err != nil {
		t.Fatal(err)
	}
	err := svc.AddCluster(in)
	if err == nil || err.Code != CodeDuplicate {
		t.Fatalf("want CodeDuplicate, got %+v", err)
	}
}

// ListClusters 必须如实反映 Keychain 是否有 token、以及 InsecureSkipVerify 设置。
// 回归保险：编辑时 UI 不能误判 token 状态（"已存 token" 徽章不出现）或静默丢失 skip-verify。
func TestListClustersReflectsTokenAndTLSFlags(t *testing.T) {
	svc, _, kr := testService(t)

	// c1：带 token + insecureSkipVerify
	if err := svc.AddCluster(ClusterInput{
		ID: "c1", Name: "C1", Address: "http://x:4646",
		TLS: true, InsecureSkipVerify: true, Token: "t1",
	}); err != nil {
		t.Fatal(err)
	}
	// c2：无 token、不跳过校验
	if err := svc.AddCluster(ClusterInput{
		ID: "c2", Name: "C2", Address: "http://x:4646",
	}); err != nil {
		t.Fatal(err)
	}

	list, e := svc.ListClusters()
	if e != nil {
		t.Fatal(e)
	}
	byID := map[string]int{}
	for i, c := range list {
		byID[c.ID] = i
	}

	c1 := list[byID["c1"]]
	if !c1.HasToken {
		t.Errorf("c1 HasToken = false; want true (token saved)")
	}
	if !c1.InsecureSkipVerify {
		t.Errorf("c1 InsecureSkipVerify = false; want true (silent reset regression)")
	}

	c2 := list[byID["c2"]]
	if c2.HasToken {
		t.Errorf("c2 HasToken = true; want false (no token saved)")
	}
	if c2.InsecureSkipVerify {
		t.Errorf("c2 InsecureSkipVerify = true; want false")
	}

	// 删掉 c1 的 token 后，HasToken 必须翻回 false
	if err := kr.DeleteToken("c1"); err != nil {
		t.Fatal(err)
	}
	list2, _ := svc.ListClusters()
	c1b := list2[byID["c1"]]
	if c1b.HasToken {
		t.Errorf("c1 HasToken = true after DeleteToken; want false")
	}
}

func TestTestConnectionDown(t *testing.T) {
	svc, _, _ := testService(t)
	// 不可达地址
	if err := svc.AddCluster(ClusterInput{ID: "dead", Name: "D", Address: "http://127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	h, e := svc.TestConnection("dead")
	if e != nil {
		t.Fatalf("down cluster should return health not error: %v", e)
	}
	if h.Status != "down" || h.Error == "" {
		t.Fatalf("health = %+v", h)
	}
}

func TestSetActiveCluster(t *testing.T) {
	svc, _, _ := testService(t)
	if err := svc.AddCluster(ClusterInput{ID: "c1", Name: "C", Address: "http://x:4646"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetActiveCluster("c1"); err != nil {
		t.Fatal(err)
	}
	if svc.ActiveCluster() != "c1" {
		t.Fatalf("active = %q", svc.ActiveCluster())
	}
	if err := svc.SetActiveCluster("nope"); err == nil {
		t.Fatal("want error for unknown cluster")
	}
}

// TestRemoveActiveClusterClearsActive 验证删 active 集群后 active 被清空。
// 防 Bug 1：旧实现 setActive("", id) 参数顺序写反，删 active 反而被设成已删 ID。
func TestRemoveActiveClusterClearsActive(t *testing.T) {
	svc, _, _ := testService(t)
	if err := svc.AddCluster(ClusterInput{ID: "c1", Name: "C", Address: "http://x:4646"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetActiveCluster("c1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveCluster("c1"); err != nil {
		t.Fatalf("RemoveCluster: %v", err)
	}
	if svc.ActiveCluster() != "" {
		t.Fatalf("after remove active: active = %q, want empty", svc.ActiveCluster())
	}
}

// TestRemoveNonActiveClusterKeepsActive 验证删别的集群不影响 active。
func TestRemoveNonActiveClusterKeepsActive(t *testing.T) {
	svc, _, _ := testService(t)
	_ = svc.AddCluster(ClusterInput{ID: "c1", Name: "C1", Address: "http://x:4646"})
	_ = svc.AddCluster(ClusterInput{ID: "c2", Name: "C2", Address: "http://y:4646"})
	if err := svc.SetActiveCluster("c1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.RemoveCluster("c2"); err != nil {
		t.Fatal(err)
	}
	if svc.ActiveCluster() != "c1" {
		t.Fatalf("active = %q, want c1", svc.ActiveCluster())
	}
}

// TestTestConnectionInput 验证添加前 Test：未落盘也能探测；失败不写配置。
func TestTestConnectionInput(t *testing.T) {
	svc, store, _ := testService(t)
	srv := nomadTestServer(t)

	// 成功路径：未 Add，直接 Test
	h, e := svc.TestConnectionInput(ClusterInput{
		ID: "ghost", Name: "Ghost", Address: srv.URL,
	})
	if e != nil {
		t.Fatalf("TestConnectionInput: %v", e)
	}
	if h.Status != "ok" || h.Leader == "" {
		t.Fatalf("health = %+v", h)
	}

	// 关键：TestConnectionInput 不落盘
	if cfgs, _ := store.List(); len(cfgs) != 0 {
		t.Fatalf("TestConnectionInput leaked config: %+v", cfgs)
	}

	// 校验失败：bad address
	_, e = svc.TestConnectionInput(ClusterInput{ID: "x", Name: "X", Address: "http://x:99999"})
	if e == nil || e.Code != CodeInvalidInput {
		t.Fatalf("want invalid_input, got %+v", e)
	}

	// 不可达：返回 down 不当 error
	h, e = svc.TestConnectionInput(ClusterInput{
		ID: "dead", Name: "D", Address: "http://127.0.0.1:1",
	})
	if e != nil {
		t.Fatalf("want down-status not error: %v", e)
	}
	if h.Status != "down" || h.Error == "" {
		t.Fatalf("health = %+v", h)
	}
}

// TestUpdateClusterEmptyTokenKeepsOld 验证编辑时空 token 保留 Keychain 旧 token。
// 这是 Update vs Remove+Add 的核心差异：token 轮换时不会因忘填而失联。
func TestUpdateClusterEmptyTokenKeepsOld(t *testing.T) {
	svc, store, kr := testService(t)
	srv := nomadTestServer(t)

	orig := ClusterInput{
		ID: "c1", Name: "Old", Address: srv.URL, Token: "old-token",
	}
	if err := svc.AddCluster(orig); err != nil {
		t.Fatal(err)
	}

	// 编辑：只改 Name，Token 留空
	if err := svc.UpdateCluster(ClusterInput{
		ID: "c1", Name: "New", Address: srv.URL,
	}); err != nil {
		t.Fatalf("UpdateCluster: %v", err)
	}

	cfg, _ := store.Get("c1")
	if cfg.Name != "New" {
		t.Fatalf("name not updated: %+v", cfg)
	}

	// 关键：token 仍是 old-token
	tok, err := kr.GetToken("c1")
	if err != nil || tok != "old-token" {
		t.Fatalf("token lost after Update: tok=%q err=%v", tok, err)
	}
}

// TestUpdateClusterReplacesToken 验证编辑时给新 token 则覆盖。
func TestUpdateClusterReplacesToken(t *testing.T) {
	svc, _, kr := testService(t)
	srv := nomadTestServer(t)

	_ = svc.AddCluster(ClusterInput{ID: "c1", Name: "C", Address: srv.URL, Token: "old"})
	if err := svc.UpdateCluster(ClusterInput{
		ID: "c1", Name: "C", Address: srv.URL, Token: "new",
	}); err != nil {
		t.Fatal(err)
	}
	tok, _ := kr.GetToken("c1")
	if tok != "new" {
		t.Fatalf("token = %q, want new", tok)
	}
}

// TestUpdateClusterUnknownFails 验证编辑不存在的集群报错。
func TestUpdateClusterUnknownFails(t *testing.T) {
	svc, _, _ := testService(t)
	err := svc.UpdateCluster(ClusterInput{ID: "ghost", Name: "G", Address: "http://x:4646"})
	if err == nil || err.Code != CodeNotFound {
		t.Fatalf("want not_found, got %+v", err)
	}
}

// TestUpdateClusterInvalidatesPool 验证编辑后 Pool 用新地址重建 client。
func TestUpdateClusterInvalidatesPool(t *testing.T) {
	svc, _, _ := testService(t)
	srv1 := nomadTestServer(t)
	srv2 := nomadTestServer(t)

	_ = svc.AddCluster(ClusterInput{ID: "c1", Name: "C", Address: srv1.URL})
	// 触发 Pool 构造 client
	if _, err := svc.Pool().Get("c1"); err != nil {
		t.Fatal(err)
	}
	// 改地址
	if err := svc.UpdateCluster(ClusterInput{ID: "c1", Name: "C", Address: srv2.URL}); err != nil {
		t.Fatal(err)
	}
	// ProbeTarget 应反映新地址
	target, err := svc.Pool().ProbeTarget("c1")
	if err != nil {
		t.Fatal(err)
	}
	if target.Addr != srv2.URL {
		t.Fatalf("pool still serves old addr: %q", target.Addr)
	}
}
