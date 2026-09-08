package requestprofile

import (
	"github.com/zhanghaiyang/adflow/internal/decision/adapter/memory"
	"github.com/zhanghaiyang/adflow/internal/decision/domain"
	"testing"
)

func TestRequestProfilesAreClonedAndDoNotLeak(t *testing.T) {
	source := memory.NewRuntime()
	store := New(source)
	input := domain.NewProfile("temp", []string{"gaming_interest"}, map[string]string{"score": "88"})
	ctx := WithProfile(t.Context(), input)
	input.Fields["score"] = "1"
	loaded, err := store.FindProfile(ctx, "temp")
	if err != nil || loaded.Fields["score"] != "88" {
		t.Fatal(loaded, err)
	}
	loaded.Fields["score"] = "2"
	again, _ := store.FindProfile(ctx, "temp")
	if again.Fields["score"] != "88" {
		t.Fatal("shared mutable fields")
	}
	if _, err := store.FindProfile(t.Context(), "temp"); err != domain.ErrProfileNotFound {
		t.Fatal("request data leaked", err)
	}
}
