package domain

import "errors"

var ErrDuplicateUser = errors.New("duplicate user")
var ErrNotFound = errors.New("resource not found")
var ErrValidation = errors.New("validation failed")
var ErrConflict = errors.New("resource conflict")
var ErrUnauthorized = errors.New("unauthorized")
