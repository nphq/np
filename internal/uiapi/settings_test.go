package uiapi

import (
	"path/filepath"
	"testing"

	"github.com/nphq/np/internal/cluster"
	"github.com/nphq/np/internal/config"
	"github.com/nphq/np/internal/secure"
)

func testSettingsService(t *testing.T) *SettingsService {
	t.Helper()
	prefs := config.NewPrefs(filepath.Join(t.TempDir(), "preferences.json"))
	if err := prefs.Load(); err != nil {
		t.Fatal(err)
	}
	store := config.New(filepath.Join(t.TempDir(), "clusters.json"))
	pool := cluster.NewPool(store, secure.NewMemory())
	cl := NewClusterService(store, prefs, secure.NewMemory())
	_ = pool
	loads := NewLoadsService(cl.Pool())
	return NewSettingsService(prefs, cl, loads)
}

func TestSettingsDefaults(t *testing.T) {
	svc := testSettingsService(t)
	st, e := svc.GetSettings()
	if e != nil {
		t.Fatal(e)
	}
	if !st.ConfirmDestructive || !st.AutoRestoreActive {
		t.Fatalf("defaults = %+v; want confirm+restore true", st)
	}
	if st.HealthIntervalSec != 30 || st.MetricsIntervalSec != 15 {
		t.Fatalf("defaults = %+v; want 30/15", st)
	}
}

func TestSettingsUpdateValidation(t *testing.T) {
	svc := testSettingsService(t)
	if e := svc.UpdateSettings(SettingsInput{HealthIntervalSec: 999, MetricsIntervalSec: 15}); e == nil {
		t.Fatal("want error for bad health interval")
	}
	if e := svc.UpdateSettings(SettingsInput{HealthIntervalSec: 30, MetricsIntervalSec: 7}); e == nil {
		t.Fatal("want error for bad metrics interval")
	}
	if e := svc.UpdateSettings(SettingsInput{
		HealthIntervalSec: 60, MetricsIntervalSec: 30,
		DefaultRegion: "global", DefaultNamespace: "default",
	}); e != nil {
		t.Fatalf("UpdateSettings: %v", e)
	}
	st, _ := svc.GetSettings()
	if st.HealthIntervalSec != 60 || st.DefaultRegion != "global" {
		t.Fatalf("persisted = %+v", st)
	}
	if _, e := svc.ResetSettings(); e != nil {
		t.Fatalf("ResetSettings: %v", e)
	}
	st, _ = svc.GetSettings()
	if st.HealthIntervalSec != 30 {
		t.Fatalf("after reset = %+v; want 30", st)
	}
	if _, e := svc.GetConfigPaths(); e != nil {
		t.Fatalf("GetConfigPaths: %v", e)
	}
}
