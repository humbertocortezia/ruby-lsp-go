package parser

import (
	"errors"
	"strings"
	"testing"
)

func TestRecoverInParsingGoroutine(t *testing.T) {
	for _, value := range []interface{}{"private source text", nil} {
		failure := make(chan error, 1)
		go func() {
			guarded := func() (err error) {
				defer Recover(&err)
				panic(value)
			}
			failure <- guarded()
		}()
		err := <-failure
		var recovered *PanicError
		if !errors.As(err, &recovered) || len(recovered.Stack) == 0 {
			t.Fatalf("missing panic information: %v", err)
		}
		if strings.Contains(err.Error(), "private source text") || strings.Contains(string(recovered.Stack), "private source text") {
			t.Fatal("panic payload leaked into error")
		}
	}
}
