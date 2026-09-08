package domain

import (
	"testing"
	"time"
)

func TestRequestFingerprintCanonicalizesIdentityButNotBusinessDifferences(t *testing.T) {
	a := Request{RequestID: " req ", UserID: " user ", SlotID: " slot "}
	b := Request{RequestID: "req", UserID: "user", SlotID: "slot", Now: time.Now()}
	if RequestFingerprint(a) != RequestFingerprint(b) {
		t.Fatal("transport whitespace/internal time changed identity")
	}
	c := b
	c.UserID = "other"
	if RequestFingerprint(c) == RequestFingerprint(b) {
		t.Fatal("different user collided")
	}
	p := NewProfile("user", []string{"a", "b", "a"}, map[string]string{"age": "20", "device": "android"})
	q := NewProfile("user", []string{"b", "a"}, map[string]string{"device": "android", "age": "20"})
	if ProfileDigest(p) != ProfileDigest(q) {
		t.Fatal("equivalent inline profile changed identity")
	}
	q.Fields["age"] = "21"
	if ProfileDigest(p) == ProfileDigest(q) {
		t.Fatal("changed inline profile not bound")
	}
}
