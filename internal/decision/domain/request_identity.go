package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

func RequestFingerprint(request Request) string {
	payload, _ := json.Marshal([]string{strings.TrimSpace(request.RequestID), strings.TrimSpace(request.UserID), strings.TrimSpace(request.SlotID), request.ProfileDigest})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func MatchesRequest(result Result, request Request) bool {
	if result.RequestID != request.RequestID || result.UserID != request.UserID || result.SlotID != request.SlotID {
		return false
	}
	if result.RequestFingerprint == "" {
		return request.ProfileDigest == ""
	}
	return result.RequestFingerprint == RequestFingerprint(request)
}

func ProfileDigest(profile Profile) string {
	payload, _ := json.Marshal(struct {
		Tags   map[string]struct{}
		Fields map[string]string
	}{profile.Tags, profile.Fields})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}
