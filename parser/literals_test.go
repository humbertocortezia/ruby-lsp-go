package parser

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func incompleteLiterals() []string {
	var sources []string
	for _, opening := range []string{"'", "\"", ":'", ":\"", "/"} {
		for _, body := range []string{"", "value", "value\\", "value\\" + opening[len(opening)-1:]} {
			// A lone slash is ambiguous with an operator in this tokenizer.
			if opening == "/" && body == "" {
				continue
			}
			for _, ending := range []string{"", "\n"} {
				sources = append(sources, opening+body+ending)
			}
		}
	}
	return sources
}

func TestIncompleteLiterals(t *testing.T) {
	for _, source := range incompleteLiterals() {
		for _, prefix := range []string{"", "require ", "attr_accessor ", "class Before\nend\n"} {
			t.Run(fmt.Sprintf("%q", prefix+source), func(t *testing.T) {
				ast, err := Parse(prefix + source)
				if err == nil || ast != nil {
					t.Fatalf("expected error without AST, got %v, %v", ast, err)
				}
				var incomplete *IncompleteLiteralError
				if !errors.As(err, &incomplete) {
					t.Fatalf("expected incomplete literal error, got %T: %v", err, err)
				}
				if incomplete.Line != strings.Count(prefix, "\n") || incomplete.Column != len(prefix)-strings.LastIndex(prefix, "\n")-1 {
					t.Fatalf("wrong literal start position: %+v", incomplete)
				}
				result, err := ParseSource(prefix + source)
				if err == nil || result != nil {
					t.Fatalf("expected error without ParseResult, got %v, %v", result, err)
				}
			})
		}
	}
}

func TestClosedLiteralTokens(t *testing.T) {
	for _, tc := range []struct {
		opening string
		kind    TokenType
	}{{"'", TokenString}, {"\"", TokenString}, {":'", TokenSymbol}, {":\"", TokenSymbol}, {"/", TokenRegex}} {
		closing := tc.opening[len(tc.opening)-1:]
		for _, body := range []string{"", "value", "line\nvalue", "escaped\\" + closing, "backslash\\\\"} {
			literal := tc.opening + body + closing
			for _, suffix := range []string{"", "\nNext", ";Next"} {
				t.Run(fmt.Sprintf("%q", literal+suffix), func(t *testing.T) {
					p := &RubyParser{source: literal + suffix}
					if err := p.tokenize(); err != nil {
						t.Fatal(err)
					}
					if len(p.tokens) < 2 || p.tokens[0].Type != tc.kind || p.tokens[0].Literal != literal {
						t.Fatalf("wrong literal token: %+v", p.tokens)
					}
					if suffix != "" && p.tokens[len(p.tokens)-2].Literal != "Next" {
						t.Fatalf("cursor skipped following token: %+v", p.tokens)
					}
					if result, err := ParseSource(literal + suffix); err != nil || result == nil {
						t.Fatalf("valid literal: %v", err)
					}
				})
			}
		}
	}
}

func TestMultilineLiteralPosition(t *testing.T) {
	p := &RubyParser{source: "\"one\ntwo\";Next"}
	if err := p.tokenize(); err != nil {
		t.Fatal(err)
	}
	if next := p.tokens[2]; next.Literal != "Next" || next.Line != 1 || next.Column != 5 {
		t.Fatalf("wrong position after multiline literal: %+v", next)
	}
}
