package domain

import "errors"

var (
	ErrInvalidRequest  = errors.New("decision request is invalid")
	ErrProfileNotFound = errors.New("user profile not found")
)
