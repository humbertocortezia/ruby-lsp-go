package parser

import (
	"errors"
	"os"
	"testing"
	"time"
)

func FuzzParseSource(f *testing.F) {
	for _, source := range boundaryCases() {
		f.Add(source)
	}
	for _, source := range incompleteLiterals() {
		f.Add(source)
	}
	source, err := os.ReadFile("testdata/issue9.rb")
	if err != nil {
		f.Fatal(err)
	}
	f.Add(string(source))
	for _, source := range []string{
		"", "'ok'", `"ok"`, `:'ok'`, `:"ok"`, `/ok/`,
		`'escaped\''`, `"escaped\""`, `:'escaped\''`, `:"escaped\""`, `/escaped\//`, `"backslash\\"`,
		"class Example\n  def run\n    puts 'ok'\n  end\nend\n",
		"<html><%= 'ok' %></html>", "<% value = \"unfinished %>",
		"message = <<~TEXT\nhello\nTEXT\n", "message = <<'TEXT'\nunfinished",
		"%q{hello}", "%Q(hello #{name})", "%q{unfinished", "%Q(unfinished\\",
		"class Outer\nclass Inner\nend\nend\n",
		`"outer #{"inner #{name}"} tail"`,
		`"#{ {key: "value"}.fetch(:key) }"`,
		`"#{ %Q{inner #{"value"}} }"`,
		`"#{name.gsub(/\}/, "}")}"`,
		`process_structclass(name, $')`,
		"pattern = /\n  (?<year>\\d{4})\n/x.match(path)",
	} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		type outcome struct {
			result *ParseResult
			err    error
		}
		done := make(chan []outcome, 1)
		go func() {
			var outcomes []outcome
			for _, ruby := range []string{source, NewERBScanner(source).RubyContent()} {
				result, err := ParseSource(ruby)
				outcomes = append(outcomes, outcome{result, err})
			}
			done <- outcomes
		}()
		// Bound each input as well as the overall fuzz run. A stuck parse must
		// fail even if the campaign ends while that input is still running.
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		var outcomes []outcome
		select {
		case outcomes = <-done:
		case <-timer.C:
			t.Fatal("parsing did not terminate within two seconds")
		}
		for _, outcome := range outcomes {
			result, err := outcome.result, outcome.err
			var failure *PanicError
			if errors.As(err, &failure) {
				t.Fatalf("%v\n%s", err, failure.Stack)
			}
			if err != nil && result != nil {
				t.Fatal("published a result after failure")
			}
			if err == nil && (result == nil || result.AST == nil) {
				t.Fatal("success without AST")
			}
		}
	})
}
