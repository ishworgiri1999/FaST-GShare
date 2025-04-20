package errx

import (
	"fmt"
)

type FieldError struct {
	// FIXME: probably create enum
	Code    int
	Message string
	Rule    string
}

func (e FieldError) Error() string {
	if e.Code != 0 {
		return fmt.Sprintf("%d: %s", e.Code, e.Message)
	}

	return e.Message
}

func NewFieldError(message string) *FieldError {
	return &FieldError{
		Message: message,
	}
}

func NewFieldErrorWithCode(code int, message string) *FieldError {
	return &FieldError{
		Code:    code,
		Message: message,
	}
}

func NewFieldErrorWithRule(rule, message string) *FieldError {
	return &FieldError{
		Message: message,
		Rule:    rule,
	}
}
