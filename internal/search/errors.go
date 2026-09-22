package search

import (
	"errors"
	"fmt"
)

type Kind uint8

const (
	KindUnknown Kind = iota
	KindMalformedQuery
	KindNotFound
	KindStale
	KindIndexUnavailable
	KindDependencyUnavailable
	KindCorruptRecord
)

func (k Kind) String() string {
	switch k {
	case KindMalformedQuery:
		return "malformed_query"
	case KindNotFound:
		return "not_found"
	case KindStale:
		return "stale"
	case KindIndexUnavailable:
		return "index_unavailable"
	case KindDependencyUnavailable:
		return "dependency_unavailable"
	case KindCorruptRecord:
		return "corrupt_record"
	default:
		return "unknown"
	}
}

type Error struct {
	Kind Kind
	Op   string
	Err  error
}

func (e *Error) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("%s: %s", e.Op, e.Kind)
	}
	return fmt.Sprintf("%s: %s: %v", e.Op, e.Kind, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

func Errf(kind Kind, op string, cause error, format string, args ...any) *Error {
	if format == "" {
		return &Error{Kind: kind, Op: op, Err: cause}
	}
	if cause == nil {
		return &Error{Kind: kind, Op: op, Err: fmt.Errorf(format, args...)}
	}
	return &Error{Kind: kind, Op: op, Err: fmt.Errorf(format+": %w", append(args, cause)...)}
}

func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindUnknown
}
