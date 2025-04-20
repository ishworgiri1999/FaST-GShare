package errx

import "net/http"

var httpStatusCodes = map[ErrorCode]int{
	CodeUnknown:            http.StatusInternalServerError,
	CodeInvalidArgument:    http.StatusBadRequest,
	CodeDeadlineExceeded:   http.StatusRequestTimeout,
	CodeNotFound:           http.StatusNotFound,
	CodeConflict:           http.StatusConflict,
	CodeUnauthenticated:    http.StatusUnauthorized,
	CodePermissionDenied:   http.StatusForbidden,
	CodeResourceExhausted:  http.StatusTooManyRequests,
	CodeFailedPrecondition: http.StatusPreconditionFailed,
	CodeAborted:            http.StatusConflict,
	CodeUnimplemented:      http.StatusNotImplemented,
	CodeValidation:         http.StatusUnprocessableEntity,
	CodeInternal:           http.StatusInternalServerError,
	CodeUnavailable:        http.StatusServiceUnavailable,
}
