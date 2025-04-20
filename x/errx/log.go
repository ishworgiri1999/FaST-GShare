package errx

import (
	"errors"

	"github.com/KontonGu/FaST-GShare/internal/db/ent"
	"github.com/samber/oops"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func LogFromError(log *zap.Logger, err error) (*zap.Logger, zapcore.Level) {
	if err == nil {
		return log, zapcore.InfoLevel
	}

	level := zapcore.ErrorLevel

	var notFoundErr *ent.NotFoundError
	if errors.As(err, &notFoundErr) {
		level = zapcore.DebugLevel
	}

	var notSingularErr *ent.NotSingularError
	if errors.As(err, &notSingularErr) {
		level = zapcore.WarnLevel
	}

	var internalErr *Error
	if errors.As(err, &internalErr) {
		level = zapcore.DebugLevel
		log = log.With(zap.Int("error_code", int(internalErr.Code())))
		// TODO: also add fields?
	}

	var oopsErr oops.OopsError
	if !errors.As(err, &oopsErr) {
		// NOTE: ensure we have an oops error
		oopsErr = oops.Wrap(err).(oops.OopsError)
	}

	fields := oopsErr.ToMap()

	if level < zapcore.WarnLevel {
		delete(fields, "stacktrace")
	}

	// TODO: only do this in dev mode
	// } else {
	// 	fmt.Println(oopsErr.Stacktrace())
	// }

	// attach the oops error data here
	log = log.With(zap.Any("error", fields))

	return log, level
}
