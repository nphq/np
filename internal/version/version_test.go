package version

import "testing"

func TestBuildDefaultsDev(t *testing.T) {
	if Version == "" {
		t.Fatal("Version must default to non-empty (dev)")
	}
	if got := Build(); got != Version {
		t.Fatalf("Build() = %q; want %q", got, Version)
	}
}
