package linkoerr

import (
	"errors"
	"fmt"
	"log/slog"

	pkgerr "github.com/pkg/errors"
)

type StackTracer interface {
	error
	StackTrace() pkgerr.StackTrace
}

type MultiError interface {
	error
	Unwrap() []error
}

// argsToAttr turns a list of typed or untyped values into a slice of [slog.Attr].
// args[i] is treated as a key if it is a string or an [slog.Attr]; otherwise, it
// is treated as a value with key "!BADKEY".
func argsToAttr(args []any) []slog.Attr {
	attrs := make([]slog.Attr, 0, len(args))
	for i := 0; i < len(args); {
		switch key := args[i].(type) {
		case slog.Attr:
			attrs = append(attrs, key)
			i++
		case string:
			if i+1 >= len(args) {
				attrs = append(attrs, slog.String("!BADKEY", key))
				i++
			} else {
				attrs = append(attrs, slog.Any(key, args[i+1]))
				i += 2
			}
		default:
			attrs = append(attrs, slog.Any("!BADKEY", args[i]))
			i++
		}
	}
	return attrs
}

type errWithAttrs struct {
	error
	attrs []slog.Attr
}

func WithAttrs(err error, args ...any) error {
	return &errWithAttrs{
		error: err,
		attrs: argsToAttr(args),
	}
}

func (e *errWithAttrs) Unwrap() error {
	return e.error
}

func (e *errWithAttrs) Attrs() []slog.Attr {
	return e.attrs
}

type attrError interface {
	Attrs() []slog.Attr
}

func ErrorAttrs(err error) []slog.Attr {
	attrs := []slog.Attr{
		{Key: "message", Value: slog.StringValue(err.Error())},
	}
	attrs = append(attrs, Attrs(err)...)
	if stackErr, ok := errors.AsType[StackTracer](err); ok {
		attrs = append(attrs, slog.Attr{
			Key:   "stack_trace",
			Value: slog.StringValue(fmt.Sprintf("%+v", stackErr.StackTrace())),
		})
	}
	return attrs
}

// Attrs recursively extracts all logging attributes from an error chain. In the
// case of duplicate keys, the outermost value takes precedence.
func Attrs(err error) []slog.Attr {
	var attrs []slog.Attr
	for err != nil {
		if ae, ok := err.(attrError); ok {
			attrs = append(attrs, ae.Attrs()...)
		}
		err = errors.Unwrap(err)
	}
	return attrs
}
