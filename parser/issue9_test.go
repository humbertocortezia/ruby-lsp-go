package parser

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestIssue9ValidRubyLiterals(t *testing.T) {
	for _, source := range []string{
		"def to_s\n  \"Ruby DataSource\"\nend\n",
		`"Some text (for #{arguments.size} arguments)" if arguments.size != 1`,
		`process_structclass(name, $')`,
		"log_creation_time_match = /\n  (?<year>\\d{4})-\n  (?<month>\\d{2})-\n  (?<day>\\d{2})_\n  (?<hour>\\d{2})\n  (?<minute>\\d{2})\n  (?<second>\\d{2})\n  \\d\n  \\.log\n/x.match(log_path)\n",
		`"#{ {key: "value"}.fetch(:key) }"`,
		`"outer #{"inner #{name}"} tail"`,
		`"#{"nested"}" + "tail"`,
		`"#{"nested"}" + "another #{"value"}"`,
	} {
		t.Run(source, func(t *testing.T) {
			result, err := ParseSource(source)
			if err != nil || result == nil || result.AST == nil {
				t.Fatalf("valid Ruby was rejected: %v", err)
			}
		})
	}
}

func TestSpecialGlobalVariableTokens(t *testing.T) {
	for _, name := range []string{"$'", "$\"", "$`", "$&", "$~", "$+", "$!", "$@", "$/", "$\\", "$;", "$:", "$<", "$>", "$=", "$.", "$,", "$_", "$?", "$*", "$$", "$0", "$12", "$-w", "$LOAD_PATH"} {
		t.Run(name, func(t *testing.T) {
			p := &RubyParser{source: "consume(" + name + ")\nNext"}
			if err := p.tokenize(); err != nil {
				t.Fatal(err)
			}
			var global *Token
			for i := range p.tokens {
				if p.tokens[i].Type == TokenGlobalVariable {
					global = &p.tokens[i]
				}
			}
			if global == nil || global.Literal != name || p.tokens[len(p.tokens)-2].Literal != "Next" {
				t.Fatalf("wrong global token or consumed subsequent code: %+v", p.tokens)
			}
		})
	}
}

func TestRegexAndDivisionTokens(t *testing.T) {
	for _, tc := range []struct {
		source string
		regex  string
	}{
		{"pattern = /\n  (?<year>\\d{4})\n/x.match(path)\nNext", "/\n  (?<year>\\d{4})\n/x"},
		{"pattern = / text with spaces /ix\nNext", "/ text with spaces /ix"},
		{"pattern = //\nNext", "//"},
		{"pattern=/\n  text\n/x\nNext", "/\n  text\n/x"},
		{"match /hello/\nNext", "/hello/"},
		{"12 / 3\nNext", ""},
		{"total / count\nNext", ""},
		{"total/count\nNext", ""},
		{"(total) / 2\nNext", ""},
		{"values[0] / 2\nNext", ""},
		{"total /= 2\nNext", ""},
		{"$/\nNext", ""},
	} {
		t.Run(tc.source, func(t *testing.T) {
			p := &RubyParser{source: tc.source}
			if err := p.tokenize(); err != nil {
				t.Fatal(err)
			}
			var regexes []string
			for _, token := range p.tokens {
				if token.Type == TokenRegex {
					regexes = append(regexes, token.Literal)
				}
			}
			if tc.regex == "" && len(regexes) != 0 || tc.regex != "" && (len(regexes) != 1 || regexes[0] != tc.regex) {
				t.Fatalf("incorrect regex/division classification: %v", regexes)
			}
			if p.tokens[len(p.tokens)-2].Literal != "Next" {
				t.Fatalf("consumed code after expression: %+v", p.tokens)
			}
		})
	}
}

func TestNestedInterpolationTokenBoundaries(t *testing.T) {
	for _, literal := range []string{
		`"#{ {key: "value"}.fetch(:key) }"`,
		`"outer #{"inner #{name}"} tail"`,
		`"#{name.gsub(/a/, "b")}"`,
		`"#{ $' }"`,
		`"#{name.gsub(/\}/, "}")}"`,
		`"#{ %q(}) }"`,
		`"#{ %Q{inner #{"value"}} }"`,
		`"#{ %r{a\}b}.match?(name) }"`,
		"\"#{ # a comment with } and quotes \"'\n  \"value\"\n}\"",
		`"quote: #$'"`,
		"`echo #{\"value\"}`",
		`"escaped \#{unexpanded}"`,
		`'#{ not interpolated " }'`,
		":\"#{\"value\"}\"",
	} {
		t.Run(literal, func(t *testing.T) {
			p := &RubyParser{source: literal + "\nclass After\nend\n"}
			if err := p.tokenize(); err != nil {
				t.Fatal(err)
			}
			if p.tokens[0].Literal != literal {
				t.Fatalf("interpolation split the literal: got %q, want %q", p.tokens[0].Literal, literal)
			}
			for _, tok := range p.tokens {
				if tok.Literal == "class" && tok.Line != strings.Count(literal, "\n")+1 {
					t.Fatalf("wrong line following literal: %+v", tok)
				}
			}
		})
	}
}

func TestMethodNameAndAssignmentTokens(t *testing.T) {
	for _, tc := range []struct {
		source string
		want   string
	}{
		{"def name=(value)\nend", "name="},
		{"def self.name=(value)\nend", "name="},
		{"def valid?\nend", "valid?"},
		{"def save!\nend", "save!"},
	} {
		t.Run(tc.source, func(t *testing.T) {
			result, err := ParseSource(tc.source)
			if err != nil || result == nil || len(result.AST.Children) != 1 || result.AST.Children[0].Name != tc.want {
				t.Fatalf("method name changed: result=%v err=%v", result, err)
			}
		})
	}
	for _, tc := range []struct {
		source, name, operator string
	}{
		{"pattern=/pattern/", "pattern", "="},
		{"foo!=bar", "foo", "!="},
	} {
		p := &RubyParser{source: tc.source}
		if err := p.tokenize(); err != nil {
			t.Fatal(err)
		}
		if len(p.tokens) < 3 || p.tokens[0].Literal != tc.name || p.tokens[1].Type != TokenOperator || p.tokens[1].Literal != tc.operator {
			t.Fatalf("operator became part of identifier: %+v", p.tokens)
		}
	}
}

func TestIssue9CompleteFile(t *testing.T) {
	source, err := os.ReadFile("testdata/issue9.rb")
	if err != nil {
		t.Fatal(err)
	}
	if result, err := ParseSource(string(source)); err != nil || result == nil {
		t.Fatalf("issue 9 file was rejected: %v", err)
	}
}

func TestPercentLiteralTokens(t *testing.T) {
	for _, literal := range []string{
		`%q{quote " and nested {braces}}`,
		`%Q{hello #{"nested"}}`,
		`%r{a/b #{"nested"}}ix`,
		`%q(escaped \) delimiter)`,
		`%q'#{opaque}'`,
		`%(#{"nested"})`,
	} {
		t.Run(literal, func(t *testing.T) {
			p := &RubyParser{source: literal + ";Next"}
			if err := p.tokenize(); err != nil {
				t.Fatal(err)
			}
			if p.tokens[0].Literal != literal || p.tokens[len(p.tokens)-2].Literal != "Next" {
				t.Fatalf("wrong percent literal boundary: %+v", p.tokens)
			}
		})
	}
}

func TestIncompleteInterpolationStillReturnsError(t *testing.T) {
	for _, source := range []string{`"#{name`, `"#{"nested`, `"#{name} tail`, "pattern = /\n  unfinished", `"#{ /unfinished`} {
		t.Run(source, func(t *testing.T) {
			result, err := ParseSource(source)
			var incomplete *IncompleteLiteralError
			if result != nil || !errors.As(err, &incomplete) {
				t.Fatalf("expected incomplete literal error without partial AST: result=%v err=%v", result, err)
			}
		})
	}
}
