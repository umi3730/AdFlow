package domain

import "errors"

var (
	ErrInvalidRequest       = errors.New("decision request is invalid")
	ErrProfileNotFound      = errors.New("user profile not found")
	ErrRateLimited          = errors.New("decision request rate limited")
	ErrOverloaded           = errors.New("decision service overloaded")
	ErrDecisionTimeout      = errors.New("decision request timed out")
	ErrAdmissionUnavailable = errors.New("decision admission control unavailable")
)
