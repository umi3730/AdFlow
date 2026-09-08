package domain

import (
	"errors"
	"testing"
)

func TestCreativeCanBeEnabledAgainButDeletedCannot(t *testing.T) {
	creative, err := NewCreative("id", "campaign", "test creative", "", "https://example.com/a.png", "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if creative.Enable() == nil {
		t.Fatal("already enabled transition accepted")
	}
	if err := creative.Disable(); err != nil {
		t.Fatal(err)
	}
	if err := creative.Enable(); err != nil {
		t.Fatal(err)
	}
	if creative.Status() != CreativeActive || creative.Revision() != 3 {
		t.Fatal("incorrect enable state/revision")
	}
	_ = creative.Disable()
	_ = creative.Delete()
	if creative.Enable() == nil || creative.Status() != CreativeDeleted {
		t.Fatal("deleted creative resurrected")
	}
}

func TestCreativeRejectsUnsafeURL(t *testing.T) {
	_, err := NewCreative("crt-1", "cmp-1", "Banner", "", "javascript:alert(1)", "https://example.com")
	if !errors.Is(err, ErrInvalidCreative) {
		t.Fatalf("NewCreative() error = %v", err)
	}
}

func TestCreativeCanBeDisabledOnce(t *testing.T) {
	creative, err := NewCreative("crt-1", "cmp-1", "Banner", "", "https://example.com/image.png", "https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if err := creative.Disable(); err != nil {
		t.Fatal(err)
	}
	if err := creative.Disable(); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Disable() error = %v", err)
	}
}
