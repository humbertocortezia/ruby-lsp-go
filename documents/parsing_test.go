package documents

import "testing"

func TestFailedParseDiscardsOldDocumentAST(t *testing.T) {
	for _, language := range []string{"ruby", "erb"} {
		t.Run(language, func(t *testing.T) {
			wrap := func(source string) string {
				if language == "erb" {
					return "<% " + source + " %>"
				}
				return source
			}
			doc := NewDocument("file:///example."+language, wrap("class Before\nend\n"), 1, language)
			if doc.ParseResult() == nil {
				t.Fatal("missing initial AST")
			}
			for _, source := range []string{`"`, `'abc`, `:"abc`, `:'abc`, `/abc\`} {
				doc.ApplyEdits([]TextEdit{{NewText: wrap(source)}})
				if doc.ParseResult() != nil || doc.NodeAtPosition(0, 0) != nil {
					t.Fatal("failed parse retained old AST")
				}
				doc.InvalidateCache()
				if doc.ParseResult() != nil {
					t.Fatal("invalidating cache published failed AST")
				}
			}
			doc.ApplyEdits([]TextEdit{{NewText: wrap("class After\nend\n")}})
			if result := doc.ParseResult(); result == nil || result.AST.Children[0].Name != "After" {
				t.Fatal("document did not recover")
			}
		})
	}
}

func TestBaseDocumentDiscardsFailedAST(t *testing.T) {
	doc := &BaseDocument{source: "class Before\nend\n"}
	doc.parse()
	if doc.ParseResult() == nil {
		t.Fatal("missing initial AST")
	}
	doc.applyEdits([]TextEdit{{NewText: `"`}})
	if doc.ParseResult() != nil {
		t.Fatal("failed parse retained AST")
	}
}
