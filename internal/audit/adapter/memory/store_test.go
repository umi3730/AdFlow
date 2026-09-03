package memory

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/zhanghaiyang/adflow/internal/audit/domain"
)

func TestConcurrentAppendAndNewestFirstList(t *testing.T) {
	store := NewStore()
	var wait sync.WaitGroup
	for index := 0; index < 100; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_ = store.Append(t.Context(), domain.Entry{ID: fmt.Sprintf("audit-%03d", index), CreatedAt: time.Now()})
		}(index)
	}
	wait.Wait()
	entries, err := store.List(t.Context(), domain.Filter{Limit: 100})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 100 {
		t.Fatalf("entries=%d", len(entries))
	}
}
