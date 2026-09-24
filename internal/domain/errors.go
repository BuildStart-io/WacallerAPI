package domain

import "errors"

var (
	// ErrNotFound is returned when a requested domain resource does not exist.
	ErrNotFound = errors.New("resource not found")

	// ErrUnauthorized is returned when authentication fails or credentials are missing.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden is returned when an authenticated user/key lacks permissions.
	ErrForbidden = errors.New("forbidden")

	// ErrConflict is returned when a resource already exists (e.g. duplicate slug or email).
	ErrConflict = errors.New("resource conflict")

	// ErrInvalidInput is returned when validation fails on input parameters.
	ErrInvalidInput = errors.New("invalid input parameters")

	// ErrRateLimited is returned when rate limits are exceeded.
	ErrRateLimited = errors.New("rate limit exceeded")

	// ErrInternal is returned for unexpected server/database errors.
	ErrInternal = errors.New("internal server error")
)
