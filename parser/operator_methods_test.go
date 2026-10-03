package parser

import (
	"errors"
	"strings"
	"testing"
)

// These fixtures also run through Ruby's syntax oracle and seed the fuzzer.
func operatorMethodCases() []string {
	return []string{
		"class Shell\n  def `(cmd)\n    super\n  end\nend\n",
		"def self.`(cmd); end\n",
		"def self::`(cmd); end\n",
		"def Shell::`(cmd); end\n",
		"def object.`(cmd); end\n",
		"def (object).`(cmd); end\n",
		"def (object).name=(value); end\n",
		"def\n`(cmd); end\n",
		"obj.`(\"echo hi\")\n",
		"obj&.`(\"echo hi\")\n",
		"obj::`(\"echo hi\")\n",
		"Shell::`(\"echo hi\")\n",
		"Namespace::Shell::`(\"echo hi\")\n",
		"obj. # continuation\n`(\"echo hi\")\n",
		"alias_method :old_backtick, :`\n",
		"alias old_backtick `\n",
		"alias ` old_backtick\n",
		"alias :old_backtick\n`\n",
		"undef `, /, %, [], []=, +@, -@, <=>, ===\n",
		"undef !@, ~@\noperators = [:!@, :~@]\n",
		"undef :old_backtick, # continuation\n`\n",
		"alias divide /\nalias modulo %\nalias write= read=\n",
		"obj./(2)\nobj&.%(2)\nobj.[](0)\nobj.[]=(0, 1)\nobj.+@\n",
		"def []=(key, value); end\ndef <=>(other); end\ndef +@; end\n",
		"obj.` `echo #{\"hi\"}`\n",
		"alias old_backtick `; result = `echo hi`\n",
		"undef `\nresult = `echo hi`\n",
		"result = \"#{obj.`(\"echo hi\")}\"\n",
		"result = \"#{obj&.`(\"echo hi\")}\"\n",
		"result = `echo #{obj.`(\"echo hi\")}`\n",
		"range = 1..`echo hi`\n",
		"range = first...`echo hi`\n",
		"alias old def\nresult = `echo hi`\n",
		"obj&. # continuation\nclass /pattern/\n",
		"foo = 1\nobj&.foo /pattern/\n",
		"obj&.foo = 1\nfoo /pattern/\n",
		"class Namespace::Shell\nend\n",
	}
}

func TestOperatorMethodNames(t *testing.T) {
	for _, name := range operatorMethodNames {
		for _, prefix := range []string{"def ", "def self.", "def self::", "def Shell::", "def (obj).", "obj.", "obj&.", "obj::", "Shell::", "alias old ", "alias ", "undef ", "undef old, "} {
			source := prefix + name + "\n"
			t.Run(source, func(t *testing.T) {
				tokens, err := Tokenize(source)
				if err != nil {
					t.Fatal(err)
				}
				for _, token := range tokens {
					if token.Type == TokenOperator && token.Literal == name && token.Line == 0 && token.Column == len(prefix) {
						return
					}
				}
				t.Fatalf("method name %q was not one operator token: %+v", name, tokens)
			})
		}
	}
}

func TestBacktickMethodDeclarations(t *testing.T) {
	for _, prefix := range []string{"def ", "def self.", "def self::", "def Shell::", "def obj.", "def (obj)."} {
		source := prefix + "`(cmd)\n  super\nend\n"
		result, err := ParseSource(source)
		if err != nil || result == nil || len(result.AST.Children) != 1 {
			t.Fatalf("declaration %q rejected: %v", source, err)
		}
		node := result.AST.Children[0]
		wantType := NodeMethod
		if prefix != "def " {
			wantType = NodeSingletonMethod
		}
		if node.Name != "`" || node.Type != wantType || node.NamePosition.Line != 0 || node.NamePosition.Character != len(prefix) {
			t.Fatalf("wrong declaration for %q: %+v", source, node)
		}
	}
}

func TestCommandStringsRemainLiterals(t *testing.T) {
	for _, prefix := range []string{"result = ", "obj.` ", "alias old `; ", "alias old `\n", "alias old def\n", "undef `\n", "def `(cmd)\n", "obj.foo=", "1..", "first...", "(1).."} {
		literal := "`echo #{\"hi\"}`"
		tokens, err := Tokenize(prefix + literal + "\nAfter\n")
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, token := range tokens {
			found = found || token.Type == TokenString && token.Literal == literal
		}
		if !found {
			t.Fatalf("command string lost in %q: %+v", prefix, tokens)
		}
		tokens, err = Tokenize(prefix + "`echo hi")
		var incomplete *IncompleteLiteralError
		line, column := sourcePosition(prefix, len(prefix))
		if tokens != nil || !errors.As(err, &incomplete) || incomplete.Kind != "string" || incomplete.Line != line || incomplete.Column != column {
			t.Fatalf("unterminated command was hidden in %q: tokens=%v err=%v", prefix, tokens, err)
		}
	}
}

func TestMethodContextPreservesAssignmentsAndRanges(t *testing.T) {
	for _, tc := range []struct {
		source string
		want   []string
	}{
		{"obj.foo=1", []string{"obj", ".", "foo", "=", "1"}},
		{"obj&.foo=1", []string{"obj", "&.", "foo", "=", "1"}},
		{"alias write= read=", []string{"alias", "write=", "read="}},
		{"def (obj).name=(v)", []string{"def", "(", "obj", ")", ".", "name=", "(", "v", ")"}},
		{"first...`echo hi`", []string{"first", "...", "`echo hi`"}},
		{"1..`echo hi`", []string{"1", "..", "`echo hi`"}},
		{"1.2...3.4", []string{"1.2", "...", "3.4"}},
		{"1.`(\"echo\")", []string{"1", ".", "`", "(", "\"echo\"", ")"}},
	} {
		tokens, err := Tokenize(tc.source)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, token := range tokens {
			if token.Type != TokenWhitespace && token.Type != TokenEOF {
				got = append(got, token.Literal)
			}
		}
		if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
			t.Fatalf("%q: got %q, want %q", tc.source, got, tc.want)
		}
	}
}
