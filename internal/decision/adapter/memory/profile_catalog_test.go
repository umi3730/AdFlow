package memory

import (
	"github.com/zhanghaiyang/adflow/internal/decision/domain"
	"testing"
)

func TestProfileCatalogFiltersPaginatesAndCopies(t *testing.T) {
	runtime := NewRuntime()
	for _, id := range []string{"user-2", "user-1", "other"} {
		_ = runtime.PutProfile(t.Context(), domain.NewProfile(id, []string{"anime"}, map[string]string{"device": "android"}))
	}
	page, err := runtime.ListProfiles(t.Context(), domain.ProfileFilter{Query: "user", Tag: "anime", Device: "android", Limit: 1, Offset: 1})
	if err != nil || page.Total != 2 || len(page.Items) != 1 || page.Items[0].UserID != "user-2" {
		t.Fatalf("page=%+v err=%v", page, err)
	}
	page.Items[0].Fields["device"] = "changed"
	profile, _ := runtime.FindProfile(t.Context(), "user-2")
	if profile.Fields["device"] != "android" {
		t.Fatal("returned profile aliases source")
	}
}
