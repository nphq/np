package secure

import (
	"errors"
	"testing"
)

func TestMemoryKeyringRoundTrip(t *testing.T) {
	m := NewMemory()
	if _, err := m.GetToken("c1"); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("GetToken missing = %v; want ErrTokenNotFound", err)
	}
	if err := m.SaveToken("c1", "t1"); err != nil {
		t.Fatal(err)
	}
	if got, err := m.GetToken("c1"); err != nil || got != "t1" {
		t.Fatalf("GetToken = %q,%v; want t1,nil", got, err)
	}
	if err := m.DeleteToken("c1"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.GetToken("c1"); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("after delete GetToken = %v; want ErrTokenNotFound", err)
	}
	if err := m.DeleteToken("missing"); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("DeleteToken missing = %v; want ErrTokenNotFound", err)
	}
}
