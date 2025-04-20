package errx

import (
	"errors"
	"net/http"
)

type ErrorCode int

const (
	CodeUnknown ErrorCode = iota
	CodeInvalidArgument
	CodeDeadlineExceeded
	CodeNotFound
	CodeConflict
	CodeUnauthenticated
	CodePermissionDenied
	CodeValidation
	CodeResourceExhausted
	CodeFailedPrecondition
	CodeAborted
	CodeUnimplemented
	CodeInternal
	CodeUnavailable
)

var messages = map[ErrorCode]string{
	CodeUnknown: "unknown",
}

type ErrorFields map[string]*FieldError

type Error struct {
	// code is the custom error code
	code ErrorCode

	// msg is the custom error message
	msg string

	// fields is a map of field errors
	fields ErrorFields

	// err is the base error
	err error
}

// Unwrap returns the base error
func (e *Error) Unwrap() error {
	return e.err
}

// Is checks if the given error is an Error
func (e *Error) Is(target error) bool {
	if target == nil {
		return false
	}

	if e2, ok := target.(*Error); ok {
		return e.code == e2.code
	}

	return false
}

// Code returns the error code
func (e *Error) Code() ErrorCode {
	return e.code
}

// Fields returns the field errors
func (e *Error) Fields() ErrorFields {
	return e.fields
}

func (e *Error) Error() string {
	// explicit message
	if e.msg != "" {
		return e.msg
	}

	// custom code message
	if message, ok := messages[e.code]; ok {
		return message
	}

	// base error
	if e.err != nil {
		return e.err.Error()
	}

	// message from http code
	if code, ok := httpStatusCodes[e.code]; ok {
		return http.StatusText(code)
	}

	return "unknown"
}

// AddField adds a field error
func (e *Error) AddField(field string, err *FieldError) {
	e.fields[field] = err
}

// HTTPStatusCode returns the HTTP status code
func (e *Error) HTTPStatusCode() int {
	// code-based status code
	if code, ok := httpStatusCodes[e.code]; ok {
		return code
	}

	return http.StatusInternalServerError
}

// HTTPStatusText returns the HTTP status text
func (e *Error) HTTPStatusText() string {
	return http.StatusText(e.HTTPStatusCode())
}

// IsError checks if the given error is an Error
func IsError(err error) bool {
	if err == nil {
		return false
	}

	var e *Error
	return errors.As(err, &e)
}

// As checks if the given error is an Error and returns it
func As(err error) (*Error, bool) {
	var e *Error
	ok := errors.As(err, &e)
	return e, ok
}

// New creates a new error with the given code and message
func New(code ErrorCode, msg string) *Error {
	return &Error{code: code, msg: msg}
}

// Msg creates an error with the given message
func Msg(msg string) *Error {
	return &Error{msg: msg}
}

// Wrap wraps an error with the given code
func Wrap(code ErrorCode, err error) *Error {
	return &Error{code: code, err: err}
}

// WrapUnknown wraps an unknown error
func WrapUnknown(err error) *Error {
	return &Error{err: err}
}

// NewWithFields creates a new error with the given code, message and fields
func NewWithFields(code ErrorCode, msg string, fields ErrorFields) *Error {
	return &Error{code: code, msg: msg, fields: fields}
}

// MsgWithFields creates an error with the given message and fields
func MsgWithFields(msg string, fields ErrorFields) *Error {
	return &Error{msg: msg, fields: fields}
}

// Validation creates a new error with the UnprocessableEntity code and the given fields
func Validation(fields ErrorFields) *Error {
	return &Error{code: CodeValidation, fields: fields}
}

// MARK: - Private constructors

// code creates an error with the given code
func code(code ErrorCode) *Error {
	return &Error{code: code}
}
