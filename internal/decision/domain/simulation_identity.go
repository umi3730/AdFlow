package domain

import (
	"crypto/sha256"
	"fmt"
)

// Shared with read-only inspection so an interrupted temporary-profile request
// can be located from its original run/user/request tuple without replaying it.
func SimulationIdentity(runID, userID, requestID string) (string, string) {
	return fmt.Sprintf("tmp:%x", sha256.Sum256([]byte(runID+"\x00"+userID))), fmt.Sprintf("tmp:%x", sha256.Sum256([]byte(runID+"\x00"+userID+"\x00"+requestID)))
}
