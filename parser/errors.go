package parser

import (
	"fmt"
	"runtime/debug"
)

// IncompleteLiteralError reports a recognized literal that reached EOF before
// its closing delimiter. Positions are zero-based, like token positions.
type IncompleteLiteralError struct {
	Kind         string
	Line, Column int
}

func (e *IncompleteLiteralError) Error() string {
	return fmt.Sprintf("unterminated %s at line %d, column %d", e.Kind, e.Line+1, e.Column+1)
}

// PanicError distinguishes an internal parser failure from incomplete source.
// The panic value is deliberately omitted: it may contain source text.
type PanicError struct {
	Kind  string
	Stack []byte
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("internal parsing failure (panic of type %s)", e.Kind)
}

// Recover converts a panic into an error in the calling goroutine. Call it
// directly with defer at parsing boundaries, and commit results only after the
// guarded work returns successfully. It does not intercept fatal runtime errors.
func Recover(err *error) {
	if value := recover(); value != nil {
		*err = &PanicError{Kind: fmt.Sprintf("%T", value), Stack: debug.Stack()}
	}
}
