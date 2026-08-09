package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/nphq/np/internal/config"
	"github.com/nphq/np/internal/nomad"
	"github.com/nphq/np/internal/secure"
	"github.com/nphq/np/internal/uiapi"
)

// newAppWithDeps 构造一个 App，cfg/keyring 可注入，便于 e2e。
// 复用 NewApp 的服务编排（clusters→loads→nodes 链），但替换默认的 OS keyring。
func newAppWithDeps(t *testing.T) (*App, *config.Store, *secure.MemoryKeyring) {
	t.Helper()
	store := config.New(filepath.Join(t.TempDir(), "clusters.json"))
	kr := secure.NewMemory()
	prefs := config.NewPrefs(filepath.Join(t.TempDir(), "preferences.json"))
	if err := prefs.Load(); err != nil {
		t.Fatalf("prefs Load: %v", err)
	}
	clusters := uiapi.NewClusterService(store, prefs, kr)
	loads := uiapi.NewLoadsService(clusters.Pool())
	clusters.OnActiveChanged = loads.Activate
	return &App{
		clusters: clusters,
		jobs:     uiapi.NewJobsService(clusters.Pool()),
		nodes:    uiapi.NewNodesService(clusters.Pool(), loads),
		loads:    loads,
	}, store, kr
}

// callBinding 通过 v3 binding dispatcher 调用 bound method，模拟前端调用路径。
// 这是真正 end-to-end 的：参数经 JSON 往返、走 v3 BoundMethod.Call 的全部错误处理逻辑。
// methodID 必须和 wails3 generate bindings 生成的稳定 ID 一致（前端 $Call.ByID 用同一个）。
func callBinding(t *testing.T, app *App, methodID uint32, args ...any) (any, error) {
	t.Helper()
	// application.NewBindings 需要 globalApplication 已初始化（其内部 a.debug 用到 Logger）
	_ = application.New(application.Options{})
	bindings := application.NewBindings(nil, nil)
	if err := bindings.Add(application.NewService(app)); err != nil {
		t.Fatalf("bindings.Add: %v", err)
	}
	rawArgs := make([]json.RawMessage, 0, len(args))
	for _, a := range args {
		b, _ := json.Marshal(a)
		rawArgs = append(rawArgs, b)
	}
	// 双路径都试一遍：ByID（前端实际路径）+ ByName（fallback，用 v3 test 文件里的格式）
	m := bindings.GetByID(methodID)
	if m == nil {
		// main 包的 PkgPath 是 "main"；用 FQN fallback 帮助诊断 ID 不匹配的情形
		m = bindings.Get(&application.CallOptions{
			MethodName: "main.App." + nameForID(methodID),
		})
	}
	if m == nil {
		t.Fatalf("bound method not found by ID: %d (also not via FQN fallback)", methodID)
	}
	return m.Call(context.Background(), rawArgs)
}

// nameForID 把已知的稳定 method ID 反查到方法名，仅用于诊断 FQN fallback。
func nameForID(id uint32) string {
	switch id {
	case bindingRemoveCluster:
		return "RemoveCluster"
	case bindingSetActiveCluster:
		return "SetActiveCluster"
	case bindingListClusters:
		return "ListClusters"
	case bindingDiscoverClusters:
		return "DiscoverClusters"
	case bindingImportFromEnv:
		return "ImportFromEnv"
	case bindingPinCluster:
		return "PinCluster"
	}
	return ""
}

// v3 生成的稳定 method ID（来自 frontend/bindings/nomad-manager/app.ts）。
// 改名/重排方法时 wails3 generate bindings 会重算，测试要同步更新。
const (
	bindingRemoveCluster    uint32 = 624040237
	bindingSetActiveCluster uint32 = 4053701917
	bindingListClusters     uint32 = 2689603438
	bindingDiscoverClusters uint32 = 1339163193
	bindingImportFromEnv    uint32 = 3269253331
	bindingPinCluster       uint32 = 1873643878
)

// TestE2ERemoveCluster_ActiveCluster 验证：
//   - RemoveCluster 经 v3 binding dispatcher 调用后返回 (nil, nil)（前端 if(err) 判定为成功）
//   - 集群从配置和 keychain 中被实际删除
//   - active 集群被删时自动清空 activeID
//
// 回归：v2→v3 迁移后这里若断，说明 (any, error) 在 v3 dispatcher 下走样。
func TestE2ERemoveCluster_ActiveCluster(t *testing.T) {
	app, store, kr := newAppWithDeps(t)

	// 加一个集群，token 入 keychain
	if err := app.clusters.AddCluster(uiapi.ClusterInput{
		ID: "dev-1", Name: "Dev", Address: "http://x:4646", Token: "secret",
	}); err != nil {
		t.Fatalf("AddCluster: %v", err)
	}

	// 设为 active（UI 删除按钮只对 active 集群显示）
	if err := app.clusters.SetActiveCluster("dev-1"); err != nil {
		t.Fatalf("SetActiveCluster: %v", err)
	}
	if got := app.clusters.ActiveCluster(); got != "dev-1" {
		t.Fatalf("active = %q, want dev-1", got)
	}

	// 经 v3 binding dispatcher 调用 RemoveCluster（模拟前端 await RemoveCluster(id)）
	result, err := callBinding(t, app, bindingRemoveCluster, "dev-1")
	if err != nil {
		t.Fatalf("binding Call returned error: %v", err)
	}
	// v3 dispatcher 把 nil data 序列化为 nil；前端 if (err) 必须判定为 false（成功路径）
	if result != nil {
		t.Fatalf("expected nil result (success), got %v", result)
	}

	// 配置/keychain 都清
	if _, err := store.Get("dev-1"); err == nil {
		t.Fatal("config not removed")
	}
	if _, err := kr.GetToken("dev-1"); err == nil {
		t.Fatal("token not removed")
	}
	// active 自动清空
	if got := app.clusters.ActiveCluster(); got != "" {
		t.Fatalf("active = %q, want empty after removing active cluster", got)
	}
}

// TestE2ERemoveCluster_NotFound 验证错误路径：
//   - 删除不存在的集群返回 *uiapi.Error（落在 data 槽，前端 if (err) 判定为失败）
//   - v3 dispatcher 不会把 data 槽的 *uiapi.Error 当成 Go error 拒绝 promise
func TestE2ERemoveCluster_NotFound(t *testing.T) {
	app, _, _ := newAppWithDeps(t)

	result, err := callBinding(t, app, bindingRemoveCluster, "no-such-cluster")
	if err != nil {
		t.Fatalf("dispatcher should not reject for in-data error: %v", err)
	}
	// result 应该是 *uiapi.Error（前端 if (err) truthy → toastErr）
	e, ok := result.(*uiapi.Error)
	if !ok {
		t.Fatalf("expected *uiapi.Error in data slot, got %T: %v", result, result)
	}
	if e.Code != uiapi.CodeNotFound {
		t.Fatalf("code = %v, want CodeNotFound", e.Code)
	}
}

// TestE2ERemoveCluster_InvalidInput 验证 v3 dispatcher 把校验错误也走 data 槽。
func TestE2ERemoveCluster_InvalidInput(t *testing.T) {
	app, _, _ := newAppWithDeps(t)

	result, err := callBinding(t, app, bindingRemoveCluster, "")
	if err != nil {
		t.Fatalf("dispatcher should not reject for validation error: %v", err)
	}
	e, ok := result.(*uiapi.Error)
	if !ok {
		t.Fatalf("expected *uiapi.Error for invalid input, got %T", result)
	}
	if e.Code != uiapi.CodeInvalidInput {
		t.Fatalf("code = %v, want CodeInvalidInput", e.Code)
	}
}

// TestE2EImportFromEnvBinding 经 v3 binding dispatcher 走 ImportFromEnv 全链路：
// env → 建集群 + 自动激活；ListClusters 响应带 activeID（前端单一数据源）。
func TestE2EImportFromEnvBinding(t *testing.T) {
	t.Setenv("NOMAD_ADDR", "http://127.0.0.1:4646")
	t.Setenv("NOMAD_TOKEN", "binding-secret")
	app, _, kr := newAppWithDeps(t)

	result, err := callBinding(t, app, bindingImportFromEnv, "Imported")
	if err != nil {
		t.Fatalf("ImportFromEnv binding: %v", err)
	}
	info, ok := result.(*nomad.ClusterInfo)
	if !ok {
		t.Fatalf("expected *nomad.ClusterInfo, got %T", result)
	}
	if info.ID != "local" || info.Name != "Imported" {
		t.Fatalf("info = %+v", info)
	}
	if app.clusters.ActiveCluster() != "local" {
		t.Fatalf("active = %q, want local", app.clusters.ActiveCluster())
	}
	// token 只进 keychain
	if tok, err := kr.GetToken("local"); err != nil || tok != "binding-secret" {
		t.Fatalf("keychain token = %q, %v", tok, err)
	}
	// ListClusters 响应带 activeID（前端单一数据源）
	if list, e := app.clusters.ListClusters(); e != nil || list.ActiveID != "local" || len(list.Clusters) != 1 {
		t.Fatalf("ListClusters = %+v, %v", list, e)
	}
}

// TestE2EPinClusterBinding 经 dispatcher 置顶：ListClusters 顺序与 Pinned 字段生效。
func TestE2EPinClusterBinding(t *testing.T) {
	app, _, _ := newAppWithDeps(t)
	_ = app.clusters.AddCluster(uiapi.ClusterInput{ID: "z", Name: "Z", Address: "http://z:4646"})
	_ = app.clusters.AddCluster(uiapi.ClusterInput{ID: "a", Name: "A", Address: "http://a:4646"})

	if _, err := callBinding(t, app, bindingPinCluster, "z", true); err != nil {
		t.Fatalf("PinCluster binding: %v", err)
	}
	list, e := app.clusters.ListClusters()
	if e != nil || list.Clusters[0].ID != "z" || !list.Clusters[0].Pinned {
		t.Fatalf("list = %+v, %v", list, e)
	}
}

// TestE2EDiscoverClustersBinding Discover 经 dispatcher 返回候选（无 env 为空、不报错）。
func TestE2EDiscoverClustersBinding(t *testing.T) {
	t.Setenv("NOMAD_ADDR", "http://127.0.0.1:4646")
	app, _, _ := newAppWithDeps(t)

	result, err := callBinding(t, app, bindingDiscoverClusters)
	if err != nil {
		t.Fatalf("DiscoverClusters binding: %v", err)
	}
	list, ok := result.([]uiapi.DiscoveredCluster)
	if !ok {
		t.Fatalf("expected []uiapi.DiscoveredCluster, got %T", result)
	}
	if len(list) != 1 || list[0].SuggestedID != "local" {
		t.Fatalf("list = %+v", list)
	}
}
