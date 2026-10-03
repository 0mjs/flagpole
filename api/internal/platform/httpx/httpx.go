// Package httpx holds the small HTTP helpers every module shares: field
// errors and short constructors for the errors handlers return.
package httpx

import (
	"fmt"
	"sort"
	"strings"

	"github.com/0mjs/zinc"
)

// FieldErrors are invalid fields, by name, and what's wrong with each.
// Returned through Invalid, they become a 422 that lists them.
type FieldErrors map[string]string

func (f FieldErrors) Error() string {
	keys := make([]string, 0, len(f))
	for k := range f {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + " " + f[k]
	}
	return strings.Join(parts, "; ")
}

func (f FieldErrors) Fields() map[string]string { return f }

// Invalid is a 422 naming the fields, or nil when there are none.
func Invalid(fields FieldErrors) error {
	if len(fields) == 0 {
		return nil
	}
	return &zinc.ValidationError{Err: fields}
}

func NotFound(format string, args ...any) error {
	return zinc.NotFound(fmt.Sprintf(format, args...))
}

func Conflict(format string, args ...any) error {
	return zinc.Conflict(fmt.Sprintf(format, args...))
}

func Forbidden(format string, args ...any) error {
	return zinc.Forbidden(fmt.Sprintf(format, args...))
}
