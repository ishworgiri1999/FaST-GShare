package errx

import "errors"

var (
	ErrInternal         = code(CodeInternal)
	ErrNotFound         = code(CodeNotFound)
	ErrUnauthenticated  = code(CodeUnauthenticated)
	ErrConflict         = code(CodeConflict)
	ErrInvalidArgument  = code(CodeInvalidArgument)
	ErrPermissionDenied = code(CodePermissionDenied)
)

func NotFound(err error) error {
	return errors.Join(ErrNotFound, err)
}

func Conflict(err error) error {
	return errors.Join(ErrConflict, err)
}
