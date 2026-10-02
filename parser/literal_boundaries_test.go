package parser

import (
	"errors"
	"strings"
	"testing"
)

func boundaryCases() []string {
	var sources []string
	for _, opener := range []string{"<<TEXT", "<<-TEXT", "<<~TEXT", "<<'TEXT'", "<<-'TEXT'", "<<~'TEXT'", "<<\"TEXT\"", "<<-`TEXT`"} {
		closing := "TEXT"
		if strings.ContainsAny(opener, "-~") {
			closing = "  TEXT"
		}
		for _, expression := range []string{opener, opener + ".chomp", "consume(" + opener + ")"} {
			sources = append(sources, "value = "+expression+"\nIt's a read/write helper. } \"\n"+closing+"\n")
		}
	}
	return append(sources,
		"total = 12\nvalue = total /count\n",
		"values = []\nvalues <<ITEM\n",
		"def divide(total, count)\n  total /count\nend\n",
		"total = 12\nvalue = \"#{total /count}\"\n",
		"flag = true\nvalue = flag ?'yes' : 'no'\n",
		"super(:/, left, right)\n",
		"operators = [:/, :`, :[], :[]=, :+@, :<=>, :valid?, :save!]\n",
		"consume(<<ONE, <<-'TWO')\nfirst ' /\nONE\nsecond #{opaque \" } /\n  TWO\n",
		"value = <<~TEXT\n#{ {key: \"value\"}.fetch(:key) }\nTEXT\n",
		"value = <<'TEXT'\n#{not Ruby \" }\nTEXT\n",
		"value = <<'END TEXT'\n' /\nEND TEXT\n",
		"value = <<TEXT\nTEXT suffix is content ' /\nTEXT\n",
		"value = <<TEXT\r\nIt's a read/write helper\r\nTEXT\r\n",
		"value = \"#{<<TEXT}\"\nIt's a read/write helper\nTEXT\n",
		"value = <<OUTER\n#{<<INNER}\n' /\nINNER\nOUTER\n",
		"=begin documentation\nYou may redistribute and/or modify it. \"' }\n=end note\nrequire 'widget/gadget'\n",
		"=begin\n=endless isn't the terminator /\n=end\n",
		"puts \"#{?%.inspect} is the sign\"\n",
		"def separator?(c)\n  c == ?'\nend\n",
		"chars = [?', ?\", ?), ?,, ?%, ?}, ?#, ?/, ?あ, ?\\n, ?\\u{41}, ?\\C-\\M-a]\n",
		"flag = true\nvalue = flag ? 'yes' : 'no'\n",
		"value = %Q'#{\"nested\"}'\n",
		"value = %r'#{\"nested\"}'\n",
	)
}

func TestRubyLiteralBoundaries(t *testing.T) {
	for _, prefix := range boundaryCases() {
		t.Run(prefix, func(t *testing.T) {
			source := prefix + "class BoundaryAfter\n  def still_indexed\n  end\nend\n"
			p := &RubyParser{source: source}
			if err := p.tokenize(); err != nil {
				t.Fatal(err)
			}
			for _, token := range p.tokens {
				if token.Literal == "BoundaryAfter" {
					if token.Line != strings.Count(prefix, "\n") || token.Column != 6 {
						t.Fatalf("wrong position: %+v", token)
					}
					return
				}
			}
			t.Fatal("lexer swallowed the declaration following the literal")
		})
	}
}

func TestMissingTerminatorsPointToOpeners(t *testing.T) {
	for _, tc := range []struct {
		source, kind string
		line, column int
	}{
		{"# intro\nvalue = <<~TEXT\nIt's a read/write helper.\n", "heredoc", 1, 8},
		{"value = <<TEXT\n  TEXT\n", "heredoc", 0, 8},
		{"value = <<TEXT\nTEXT \n", "heredoc", 0, 8},
		{"value = <<TEXT", "heredoc", 0, 8},
		{"# intro\n=begin\nIt's a read/write helper.\n", "block comment", 1, 0},
		{"value = ?\\", "character", 0, 8},
	} {
		t.Run(tc.source, func(t *testing.T) {
			result, err := ParseSource(tc.source)
			var incomplete *IncompleteLiteralError
			if result != nil || !errors.As(err, &incomplete) || incomplete.Kind != tc.kind || incomplete.Line != tc.line || incomplete.Column != tc.column {
				t.Fatalf("wrong error: result=%v err=%v", result, err)
			}
		})
	}
}

func TestDataSectionIsNotRubyCode(t *testing.T) {
	source := "class BeforeData\nend\n__END__\nIt's / data #{ \"\nclass NotCode\nend\n"
	result, err := ParseSource(source)
	if err != nil || result == nil || len(result.AST.Children) != 1 || result.AST.Children[0].Name != "BeforeData" {
		t.Fatalf("data interpreted as Ruby: %v %v", result, err)
	}
}

func TestCharacterTokens(t *testing.T) {
	for _, literal := range []string{"?'", "?\"", "?)", "?,", "?%", "?}", "?#", "?/", "?あ", `?\n`, `?\u{41}`, `?\u0041`, `?\x41`, `?\101`, `?\M-\C-a`, `?\c\M-a`} {
		p := &RubyParser{source: literal + ";Next"}
		if err := p.tokenize(); err != nil {
			t.Fatal(err)
		}
		if p.tokens[0].Type != TokenString || p.tokens[0].Literal != literal || p.tokens[len(p.tokens)-2].Literal != "Next" {
			t.Fatalf("wrong character boundary for %q: %+v", literal, p.tokens)
		}
	}
}
