package domain

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

var (
	ErrInvalidSimulationIdentity = errors.New("invalid simulation identity")
	ErrSimulationUserOutOfRange  = errors.New("simulation user number must be between 1 and 100")
	simulationRunPattern         = regexp.MustCompile(`^[\w-]{1,80}$`)
	simulationUserPattern        = regexp.MustCompile(`^user-\d{4}$`)
)

// ValidateSimulationIdentity keeps submission and trace lookup on the same
// run/user contract. Request IDs retain each endpoint's existing validation.
func ValidateSimulationIdentity(runID, userID string) error {
	if !simulationRunPattern.MatchString(runID) || !simulationUserPattern.MatchString(userID) {
		return ErrInvalidSimulationIdentity
	}
	number, _ := strconv.Atoi(userID[5:])
	if number < 1 || number > 100 {
		return ErrSimulationUserOutOfRange
	}
	return nil
}

// Shared with read-only inspection so an interrupted temporary-profile request
// can be located from its original run/user/request tuple without replaying it.
func SimulationIdentity(runID, userID, requestID string) (string, string) {
	return fmt.Sprintf("tmp:%x", sha256.Sum256([]byte(runID+"\x00"+userID))), fmt.Sprintf("tmp:%x", sha256.Sum256([]byte(runID+"\x00"+userID+"\x00"+requestID)))
}
