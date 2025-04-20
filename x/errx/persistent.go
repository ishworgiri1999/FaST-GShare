package errx

import (
	"errors"
	"fmt"
)

// PersistentError wraps an error to indicate it is non-retryable.
type PersistentError struct {
	Err error
}

// Error implements the error interface for PersistentError.
func (e *PersistentError) Error() string {
	return fmt.Sprintf("persistent error: %v", e.Err)
}

// Unwrap provides compatibility for errors.Is and errors.As.
func (e *PersistentError) Unwrap() error {
	return e.Err
}

// WrapPersistent wraps an error as persistent.
func WrapPersistent(err error) error {
	if err == nil {
		return nil
	}
	return &PersistentError{Err: err}
}

// NewPersistent creates a new persistent error.
func NewPersistent(msg string) error {
	return &PersistentError{Err: errors.New(msg)}
}

// IsPersistentError checks if the error is a persistent error.
func IsPersistentError(err error) bool {
	var persistentErr *PersistentError
	return errors.As(err, &persistentErr)
}
