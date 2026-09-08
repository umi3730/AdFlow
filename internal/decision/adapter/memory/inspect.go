package memory

import (
	"context"
	"github.com/zhanghaiyang/adflow/internal/decision/domain"
	"time"
)

func (r *Runtime) PeekDecision(ctx context.Context, id string) (domain.Result, bool, error) {
	if err := ctx.Err(); err != nil {
		return domain.Result{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	stored, ok := r.decisions[id]
	if !ok || (!stored.expiresAt.IsZero() && !time.Now().Before(stored.expiresAt)) {
		return domain.Result{}, false, nil
	}
	return stored.result, true, nil
}
