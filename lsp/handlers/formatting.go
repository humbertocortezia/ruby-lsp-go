package handlers

import (
	"bytes"
	"os/exec"
	"strings"

	"github.com/humberto/ruby-lsp-go/documents"
)

// Formatting handles textDocument/formatting.
func Formatting(ctx *Context, params interface{}) interface{} {
	uri := extractURI(params)
	doc, ok := ctx.Store.GetDocument(uri)
	if !ok {
		return []interface{}{}
	}

	formatted := formatSource(doc.Source(), uriToPath(uri))
	if formatted == doc.Source() {
		return []interface{}{}
	}

	return []interface{}{
		map[string]interface{}{
			"range": map[string]interface{}{
				"start": map[string]interface{}{"line": 0, "character": 0},
				"end":   endPosition(doc.Source()),
			},
			"newText": formatted,
		},
	}
}

// RangeFormatting handles textDocument/rangeFormatting.
func RangeFormatting(ctx *Context, params interface{}) interface{} {
	return Formatting(ctx, params)
}

// OnTypeFormatting handles textDocument/onTypeFormatting.
func OnTypeFormatting(ctx *Context, params interface{}) interface{} {
	uri := extractURI(params)
	doc, ok := ctx.Store.GetDocument(uri)
	if !ok {
		return nil
	}

	ch := ""
	if paramMap, ok := params.(map[string]interface{}); ok {
		if c, ok := paramMap["ch"].(string); ok {
			ch = c
		}
	}

	pos := extractPositionFromParams(params)
	edits := onTypeEdits(doc.Source(), pos.Line, pos.Character, ch)
	if len(edits) == 0 {
		return nil
	}
	return edits
}

func formatSource(source, filePath string) string {
	// Try rubocop --autocorrect --stdin
	cmd := exec.Command("rubocop", "--autocorrect", "--stdin", filePath, "--force-exclusion")
	cmd.Stdin = strings.NewReader(source)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err == nil && stdout.Len() > 0 {
		return stdout.String()
	}

	// Try syntax_tree (stree)
	cmd2 := exec.Command("stree", "format", "--print-width", "120")
	cmd2.Stdin = strings.NewReader(source)
	stdout2 := &bytes.Buffer{}
	cmd2.Stdout = stdout2
	if err := cmd2.Run(); err == nil && stdout2.Len() > 0 {
		return stdout2.String()
	}

	return source
}

func endPosition(source string) map[string]interface{} {
	lines := strings.Split(source, "\n")
	lastLine := len(lines) - 1
	lastChar := len(lines[lastLine])
	return map[string]interface{}{
		"line":      lastLine,
		"character": lastChar,
	}
}

func onTypeEdits(source string, line, col int, ch string) []interface{} {
	lines := strings.Split(source, "\n")
	if line >= len(lines) {
		return nil
	}

	switch ch {
	case "\n":
		// Auto-indent after do, {, (, [
		lineText := strings.TrimSpace(lines[line])
		if strings.HasSuffix(lineText, " do") || strings.HasSuffix(lineText, "{") ||
			strings.HasSuffix(lineText, "(") || strings.HasSuffix(lineText, "[") {
			indent := detectIndent(lines[line])
			return []interface{}{
				map[string]interface{}{
					"range": map[string]interface{}{
						"start": map[string]interface{}{"line": line + 1, "character": 0},
						"end":   map[string]interface{}{"line": line + 1, "character": 0},
					},
					"newText": indent + "  ",
				},
			}
		}
	case "d":
		// Auto-close 'end' after 'end' typing on new line with only 'end'
		if line < len(lines) && strings.TrimSpace(lines[line]) == "en" {
			return []interface{}{
				map[string]interface{}{
					"range": map[string]interface{}{
						"start": map[string]interface{}{"line": line, "character": col},
						"end":   map[string]interface{}{"line": line, "character": col},
					},
					"newText": "d",
				},
			}
		}
	}
	return nil
}

func detectIndent(line string) string {
	indent := ""
	for _, ch := range line {
		if ch == ' ' || ch == '\t' {
			indent += string(ch)
		} else {
			break
		}
	}
	return indent
}

func extractPositionFromParams(params interface{}) documents.Position {
	_, pos := extractPosition(params)
	return documents.Position{Line: pos.Line, Character: pos.Character}
}
