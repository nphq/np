package uiapi

import (
	"strings"
	"testing"
)

func TestValidateClusterID(t *testing.T) {
	valid := []string{"dev", "dev-1", "PROD_2", "a.b.c", strings.Repeat("x", 64)}
	for _, v := range valid {
		if err := ValidateClusterID(v); err != nil {
			t.Errorf("want valid %q, got %v", v, err)
		}
	}
	invalid := []string{"", "has space", "中文", "a/b", strings.Repeat("x", 65)}
	for _, v := range invalid {
		if err := ValidateClusterID(v); err == nil {
			t.Errorf("want invalid %q", v)
		}
	}
}

func TestValidateAddress(t *testing.T) {
	got, err := ValidateAddress("127.0.0.1:4646")
	if err != nil {
		t.Fatalf("no scheme: %v", err)
	}
	if got != "http://127.0.0.1:4646" {
		t.Fatalf("normalized = %q", got)
	}

	got, err = ValidateAddress("https://nomad.example.com")
	if err != nil || got != "https://nomad.example.com" {
		t.Fatalf("keep https: %q %v", got, err)
	}

	if _, err := ValidateAddress(""); err == nil {
		t.Fatal("want error on empty")
	}
	if _, err := ValidateAddress("http://:4646"); err == nil {
		t.Fatal("want error on missing host")
	}
	if _, err := ValidateAddress("http://x:99999"); err == nil {
		t.Fatal("want error on bad port")
	}
	if _, err := ValidateAddress("http://x:0"); err == nil {
		t.Fatal("want error on port 0")
	}
	if _, err := ValidateAddress("http://x:-1"); err == nil {
		t.Fatal("want error on negative port")
	}
}

func TestValidateNames(t *testing.T) {
	if err := ValidateNamespace(""); err != nil {
		t.Fatal("empty namespace ok")
	}
	if err := ValidateNamespace("prod"); err != nil {
		t.Fatal("valid namespace rejected")
	}
	if err := ValidateNamespace("a b"); err == nil {
		t.Fatal("space namespace should fail")
	}
	if err := ValidateRegion("us-east-1"); err != nil {
		t.Fatal("valid region rejected")
	}
	if err := ValidateRegion("a/b"); err == nil {
		t.Fatal("slash region should fail")
	}
}

func TestWrap(t *testing.T) {
	if Wrap(nil) != nil {
		t.Fatal("Wrap(nil) should be nil")
	}
	ie := NewError(CodeInvalidInput, "bad input")
	if Wrap(ie) != ie {
		t.Fatal("Wrap should pass through *Error")
	}
	we := Wrap(errStringError{})
	if we.Code != CodeInternal {
		t.Fatalf("default code = %s", we.Code)
	}
}

type errStringError struct{}

func (errStringError) Error() string { return "boom" }
