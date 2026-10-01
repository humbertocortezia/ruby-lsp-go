package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/humberto/ruby-lsp-go/documents"
	"github.com/humberto/ruby-lsp-go/parser"
)

const maxDiagnosticFileSize = 100000

func clampNonNegative(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// Diagnostics handles textDocument/diagnostic (pull diagnostics).
func Diagnostics(ctx *Context, params interface{}) interface{} {
	uri := extractURI(params)
	doc, ok := ctx.Store.GetDocument(uri)
	if !ok {
		return map[string]interface{}{"kind": "full", "items": []interface{}{}}
	}

	if len(doc.Source()) > maxDiagnosticFileSize {
		return map[string]interface{}{"kind": "full", "items": []interface{}{}}
	}

	items := []interface{}{}

	// For ERB/HTML-embedded files, the Go parser must not see the
	// surrounding HTML — feeding it `<html><body>...</body></html>`
	// produces a flood of false-positive "syntax errors" on every tag.
	// The fix is to either (a) skip diagnostics entirely, or (b) parse
	// only the extracted Ruby content. We do (b) so users still get
	// useful diagnostics for the Ruby inside `<% %>` blocks.
	parseSource := doc.Source()
	if isERB(uri) {
		parseSource = extractRubyForDiagnostics(doc)
	}

	// Try RuboCop first. We still pass the full document for ERB so a
	// project that has rubocop-erb installed can lint both halves, but
	// the syntax fallback below is the part that must skip HTML.
	rubocopDiags := runRuboCop(doc.Source(), uriToPath(uri))
	items = append(items, rubocopDiags...)

	// Fallback: AST-based syntax check on the Ruby portion only.
	if len(items) == 0 {
		syntaxDiags := syntaxDiagnostics(parseSource)
		items = append(items, syntaxDiags...)
	}

	return map[string]interface{}{
		"kind":  "full",
		"items": items,
	}
}

// isERB returns true if the document is an ERB template.
func isERB(uri string) bool {
	return strings.HasSuffix(uri, ".erb") || strings.HasSuffix(uri, ".rhtml")
}

// extractRubyForDiagnostics returns the Ruby content of an ERB document
// for the syntax fallback. For non-ERB documents it returns the source
// unchanged. We use a type assertion to access the scanner when the
// document is an ERBDocument; otherwise we fall back to the raw source.
func extractRubyForDiagnostics(doc documents.Document) string {
	if erb, ok := doc.(interface {
		Scanner() *parser.ERBScanner
	}); ok && erb.Scanner() != nil {
		return erb.Scanner().RubyContent()
	}
	return doc.Source()
}

type ruboCopOutput struct {
	Files []struct {
		Path     string `json:"path"`
		Offenses []struct {
			Severity string `json:"severity"`
			Message  string `json:"message"`
			Location struct {
				StartLine   int `json:"start_line"`
				StartColumn int `json:"start_column"`
				LastLine    int `json:"last_line"`
				LastColumn  int `json:"last_column"`
			} `json:"location"`
			CopName string `json:"cop_name"`
		} `json:"offenses"`
	} `json:"files"`
}

func runRuboCop(source, filePath string) []interface{} {
	cmd := exec.Command("rubocop", "--format", "json", "--stdin", filePath, "--force-exclusion")
	cmd.Stdin = strings.NewReader(source)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// rubocop returns non-zero when offenses found
		if stdout.Len() == 0 {
			return nil
		}
	}

	var output ruboCopOutput
	if err := json.Unmarshal(stdout.Bytes(), &output); err != nil {
		return nil
	}

	var diags []interface{}
	for _, file := range output.Files {
		for _, offense := range file.Offenses {
			severity := 2 // Warning
			if offense.Severity == "error" || offense.Severity == "fatal" {
				severity = 1
			}
			startLine := clampNonNegative(offense.Location.StartLine - 1)
			startChar := clampNonNegative(offense.Location.StartColumn - 1)
			endLine := clampNonNegative(offense.Location.LastLine - 1)
			endChar := clampNonNegative(offense.Location.LastColumn - 1)
			if endLine < startLine {
				endLine = startLine
			}
			diag := map[string]interface{}{
				"range": map[string]interface{}{
					"start": map[string]interface{}{
						"line":      startLine,
						"character": startChar,
					},
					"end": map[string]interface{}{
						"line":      endLine,
						"character": endChar,
					},
				},
				"severity": severity,
				"source":   "rubocop",
				"code":     offense.CopName,
				"message":  offense.Message,
			}
			diags = append(diags, diag)
		}
	}
	return diags
}

func syntaxDiagnostics(source string) []interface{} {
	_, err := parser.ParseSource(source)
	if err != nil {
		message := fmt.Sprintf("Syntax error: %v", err)
		var failure *parser.PanicError
		if errors.As(err, &failure) {
			message = err.Error()
		}
		return []interface{}{
			map[string]interface{}{
				"range": map[string]interface{}{
					"start": map[string]interface{}{"line": 0, "character": 0},
					"end":   map[string]interface{}{"line": 0, "character": 1},
				},
				"severity": 1,
				"source":   "ruby-lsp-go",
				"message":  message,
			},
		}
	}

	// Fallback: count only unambiguous brace delimiters (`{`/`}` and
	// `(`/`)`). We deliberately do NOT count `def`/`do`/`end` as
	// delimiters because `end` also closes `class`, `module`, `if`,
	// `case`, `while`, `begin`, `unless`, `until` — counting only
	// `def`/`do` would produce false positives on every valid Ruby file
	// that contains a `class` or `if`. Real syntax diagnostics should
	// come from an external Ruby tool (e.g. `ruby -c`).
	if unbalanced, which := countUnbalancedBraces(source); unbalanced {
		return []interface{}{
			map[string]interface{}{
				"range": map[string]interface{}{
					"start": map[string]interface{}{"line": 0, "character": 0},
					"end":   map[string]interface{}{"line": 0, "character": len(source)},
				},
				"severity": 1,
				"source":   "ruby-lsp-go",
				"message":  fmt.Sprintf("Unbalanced %q detected", which),
			},
		}
	}

	return nil
}

// countUnbalancedBraces returns (true, "{") if the source has more "{" than "}",
// (true, "}") if it has more "}" than "{", (true, "(") or (true, ")") for parens,
// and (false, "") otherwise. Comments (# to end of line) and strings ("..." / '...')
// are skipped so identifiers inside them do not affect the count.
func countUnbalancedBraces(source string) (bool, string) {
	opensBraces := 0
	closesBraces := 0
	opensParens := 0
	closesParens := 0
	for i := 0; i < len(source); i++ {
		c := source[i]
		// Skip line comments
		if c == '#' {
			for i < len(source) && source[i] != '\n' {
				i++
			}
			continue
		}
		// Skip strings (single and double quoted). We do not attempt to
		// handle percent-strings (%w, %i, %q, %r) or heredocs because
		// those are rare edge cases and a false negative there is less
		// harmful than a false positive on every class/if block.
		if c == '"' || c == '\'' {
			quote := c
			i++
			for i < len(source) && source[i] != quote {
				if source[i] == '\\' && i+1 < len(source) {
					i += 2
					continue
				}
				i++
			}
			continue
		}
		switch c {
		case '{':
			opensBraces++
		case '}':
			closesBraces++
		case '(':
			opensParens++
		case ')':
			closesParens++
		}
	}
	switch {
	case opensBraces > closesBraces:
		return true, "{"
	case closesBraces > opensBraces:
		return true, "}"
	case opensParens > closesParens:
		return true, "("
	case closesParens > opensParens:
		return true, ")"
	}
	return false, ""
}
