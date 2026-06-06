// Package errs implements the typed-error + exit-code contract for the nostos CLI.
//
// Exit codes follow the AI-friendly CLI spec:
//
//	 0 success
//	10 validation_failed
//	11 network_error
//	12 auth_error
//	13 conflict
//	14 not_found
//	15 timeout
//	 1 generic (legacy / un-classified)
//
// Error JSON is written to STDOUT (parseable by callers); a one-line hint
// is written to STDERR for humans.
package errs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Category is the broad error class, also used as the "error" JSON field.
type Category string

const (
	CatValidation Category = "validation_failed"
	CatNetwork    Category = "network_error"
	CatAuth       Category = "auth_error"
	CatConflict   Category = "conflict"
	CatNotFound   Category = "not_found"
	CatTimeout    Category = "timeout"
	CatInternal   Category = "internal_error"
)

// ExitCode maps each category to its documented exit code.
var ExitCode = map[Category]int{
	CatValidation: 10,
	CatNetwork:    11,
	CatAuth:       12,
	CatConflict:   13,
	CatNotFound:   14,
	CatTimeout:    15,
	CatInternal:   1,
}

// Catalog returns a stable JSON-friendly map of code -> category.
func Catalog() map[string]string {
	out := map[string]string{"0": "success"}
	for cat, code := range ExitCode {
		out[fmt.Sprintf("%d", code)] = string(cat)
	}
	return out
}

// Error is the typed error returned from RunE. It implements the standard
// error interface plus marshals as JSON-RPC-friendly object.
type Error struct {
	Category Category       `json:"error"`
	Code     string         `json:"code"`
	Message  string         `json:"message"`
	Details  map[string]any `json:"details,omitempty"`
	Hint     string         `json:"hint,omitempty"`
	wrapped  error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.wrapped }

// Exit returns the exit code for this error.
func (e *Error) Exit() int {
	if c, ok := ExitCode[e.Category]; ok {
		return c
	}
	return 1
}

// Wrap attaches an underlying cause without changing the surface category.
func (e *Error) Wrap(cause error) *Error {
	e.wrapped = cause
	return e
}

// WithDetails attaches structured details.
func (e *Error) WithDetails(d map[string]any) *Error {
	if e.Details == nil {
		e.Details = map[string]any{}
	}
	for k, v := range d {
		e.Details[k] = v
	}
	return e
}

// WithHint attaches a stderr hint.
func (e *Error) WithHint(h string) *Error {
	e.Hint = h
	return e
}

// New constructs an Error in the given category.
func New(cat Category, code, msg string) *Error {
	return &Error{Category: cat, Code: code, Message: msg}
}

// Validation, Network, Auth, Conflict, NotFound, Timeout, Internal are
// shorthand constructors.
func Validation(code, msg string) *Error { return New(CatValidation, code, msg) }
func Network(code, msg string) *Error    { return New(CatNetwork, code, msg) }
func Auth(code, msg string) *Error       { return New(CatAuth, code, msg) }
func Conflict(code, msg string) *Error   { return New(CatConflict, code, msg) }
func NotFound(code, msg string) *Error   { return New(CatNotFound, code, msg) }
func Timeout(code, msg string) *Error    { return New(CatTimeout, code, msg) }
func Internal(code, msg string) *Error   { return New(CatInternal, code, msg) }

// FromGo classifies a stdlib/third-party error into the closest Category.
// Used to wrap legacy error sites without bespoke conversion.
func FromGo(err error) *Error {
	if err == nil {
		return nil
	}
	var typed *Error
	if errors.As(err, &typed) {
		return typed
	}
	msg := err.Error()
	// Light heuristics; callers should prefer typed errors at the source.
	switch {
	case containsAny(msg, "not found", "no such", "no controlplane", "unknown"):
		return NotFound("E_NOT_FOUND", msg).Wrap(err)
	case containsAny(msg, "timeout", "timed out", "deadline exceeded"):
		return Timeout("E_TIMEOUT", msg).Wrap(err)
	case containsAny(msg, "connection refused", "no route", "DNS", "dial tcp", "i/o timeout"):
		return Network("E_NETWORK", msg).Wrap(err)
	case containsAny(msg, "unauthorized", "forbidden", "401", "403", "credential", "sudo required"):
		return Auth("E_AUTH", msg).Wrap(err)
	case containsAny(msg, "already exists", "conflict", "in use", "locked"):
		return Conflict("E_CONFLICT", msg).Wrap(err)
	case containsAny(msg, "invalid", "must match", "must be", "refusing"):
		return Validation("E_VALIDATION", msg).Wrap(err)
	default:
		return Internal("E_INTERNAL", msg).Wrap(err)
	}
}

func containsAny(s string, needles ...string) bool {
	for _, n := range needles {
		if indexFold(s, n) >= 0 {
			return true
		}
	}
	return false
}

// indexFold is a tiny case-insensitive substring search to avoid pulling
// strings.EqualFold-style patterns through the hot path.
func indexFold(s, sub string) int {
	if sub == "" {
		return 0
	}
	if len(s) < len(sub) {
		return -1
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		ok := true
		for j := 0; j < len(sub); j++ {
			a := s[i+j]
			b := sub[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

// Emit writes the JSON payload to stdout and the human hint to stderr.
// outputJSON controls whether the JSON is human-pretty or compact.
func Emit(stdout, stderr io.Writer, e *Error) {
	if e == nil {
		return
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(e)
	if e.Hint != "" {
		fmt.Fprintln(stderr, "hint: "+e.Hint)
	} else {
		fmt.Fprintf(stderr, "error[%s/%s]: %s\n", e.Category, e.Code, e.Message)
	}
}
