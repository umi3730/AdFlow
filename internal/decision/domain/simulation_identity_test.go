package domain

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateSimulationIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, run, user string
		want            error
	}{
		{"first", "run_1-test", "user-0001", nil},
		{"last", strings.Repeat("a", 80), "user-0100", nil},
		{"missing run", "", "user-0001", ErrInvalidSimulationIdentity},
		{"long run", strings.Repeat("a", 81), "user-0001", ErrInvalidSimulationIdentity},
		{"invalid run", "run/1", "user-0001", ErrInvalidSimulationIdentity},
		{"unicode run", "运行", "user-0001", ErrInvalidSimulationIdentity},
		{"short user", "run", "user-1", ErrInvalidSimulationIdentity},
		{"missing user", "run", "", ErrInvalidSimulationIdentity},
		{"invalid user", "run", "user-00x1", ErrInvalidSimulationIdentity},
		{"zero", "run", "user-0000", ErrSimulationUserOutOfRange},
		{"over limit", "run", "user-0101", ErrSimulationUserOutOfRange},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateSimulationIdentity(tc.run, tc.user); !errors.Is(err, tc.want) {
				t.Fatalf("error=%v, want %v", err, tc.want)
			}
		})
	}
}
