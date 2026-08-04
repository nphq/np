package secure

import (
	"errors"
	"testing"
)

func TestMemoryKeyringCRUD(t *testing.T) {
	k := NewMemory()
	if _, err := k.GetToken("c1"); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("want ErrTokenNotFound, got %v", err)
	}
	if err := k.SaveToken("c1", "secret-1"); err != nil {
		t.Fatal(err)
	}
	got, err := k.GetToken("c1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "secret-1" {
		t.Fatalf("token = %q", got)
	}
	if err := k.DeleteToken("c1"); err != nil {
		t.Fatal(err)
	}
	if _, err := k.GetToken("c1"); !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("after delete: want ErrTokenNotFound, got %v", err)
	}
}

func TestMemoryKeyringIsolation(t *testing.T) {
	k := NewMemory()
	_ = k.SaveToken("c1", "t1")
	_ = k.SaveToken("c2", "t2")
	got, err := k.GetToken("c1")
	if err != nil || got != "t1" {
		t.Fatalf("c1 token not isolated: %q %v", got, err)
	}
	got, err = k.GetToken("c2")
	if err != nil || got != "t2" {
		t.Fatalf("c2 token not isolated: %q %v", got, err)
	}
}

func TestOSKeyringValidation(t *testing.T) {
	k := New()
	if err := k.SaveToken("", "t"); err == nil {
		t.Fatal("want error on empty clusterID")
	}
	if err := k.SaveToken("c1", ""); err == nil {
		t.Fatal("want error on empty token")
	}
	if _, err := k.GetToken(""); err == nil {
		t.Fatal("want error on empty clusterID")
	}
}
