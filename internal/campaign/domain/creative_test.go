package domain

import (
	"errors"
	"testing"
)

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
