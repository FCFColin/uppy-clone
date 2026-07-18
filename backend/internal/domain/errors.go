package domain

import "errors"

// ErrDuplicateUser is returned when a user already exists in a game or resource.
var ErrDuplicateUser = errors.New("duplicate user")

// ErrNotFound is returned when a requested resource does not exist.
var ErrNotFound = errors.New("resource not found")

// ErrValidation is returned when input fails validation checks.
var ErrValidation = errors.New("validation failed")
