package domain

import (
	"fmt"
	"github.com/zhanghaiyang/adflow/internal/profile/schema"
)

// Kept separate from rehydration: invalid old rules can still be opened and fixed.
func (r TargetingRule) ValidateForPublication() error {
	for _, group := range [][]Condition{r.All, r.Any, r.None} {
		for _, condition := range group {
			if condition.Tag != "" {
				if condition.Field != "" || condition.Op != "" || condition.Value != "" {
					return ErrInvalidTargeting
				}
				continue
			}
			if err := schema.ValidateCondition(condition.Field, condition.Op, condition.Value); err != nil {
				return fmt.Errorf("%w: %s", ErrInvalidTargeting, err)
			}
		}
	}
	return nil
}
