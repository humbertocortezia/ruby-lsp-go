package handlers

import (
	"strings"

	"github.com/humberto/ruby-lsp-go/indexer"
	"github.com/humberto/ruby-lsp-go/parser"
)

// SignatureHelp handles textDocument/signatureHelp.
func SignatureHelp(ctx *Context, params interface{}) interface{} {
	uri, pos := extractPosition(params)
	doc, ok := ctx.Store.GetDocument(uri)
	if !ok || ctx.Index == nil {
		return nil
	}

	if doc.ShouldDelegate(pos.Line, pos.Character) {
		return delegateError()
	}

	// Find method call at cursor
	methodName := findMethodCallAtPosition(doc.Source(), pos.Line, pos.Character)
	if methodName == "" {
		return nil
	}

	entries := ctx.Index.LookupLegacy(methodName)
	if len(entries) == 0 {
		return nil
	}

	var signatures []interface{}
	for _, entry := range entries {
		if entry.Type != indexer.SymbolMethod && entry.Type != indexer.SymbolSingletonMethod {
			continue
		}

		label := entry.FullyQualifiedName + "("
		paramInfos := []interface{}{}
		for i, p := range entry.Parameters {
			if i > 0 {
				label += ", "
			}
			label += p
			paramInfos = append(paramInfos, map[string]interface{}{
				"label": p,
			})
		}
		label += ")"

		signatures = append(signatures, map[string]interface{}{
			"label":      label,
			"parameters": paramInfos,
		})
	}

	if len(signatures) == 0 {
		return nil
	}

	activeParam := countCommasBeforeCursor(doc.Source(), pos.Line, pos.Character)

	return map[string]interface{}{
		"signatures":      signatures,
		"activeSignature": 0,
		"activeParameter": activeParam,
	}
}

func findMethodCallAtPosition(source string, line, col int) string {
	lines := strings.Split(source, "\n")
	if line >= len(lines) {
		return ""
	}
	lineText := lines[line]
	if col > len(lineText) {
		col = len(lineText)
	}

	// Walk backwards to find method name before '(' or before cursor
	before := lineText[:col]
	parenIdx := strings.LastIndex(before, "(")
	searchEnd := col
	if parenIdx >= 0 {
		searchEnd = parenIdx
	}

	// Find identifier before '('
	text := lineText[:searchEnd]
	parts := strings.FieldsFunc(text, func(r rune) bool {
		return r == ' ' || r == '.' || r == '(' || r == ')'
	})
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

func countCommasBeforeCursor(source string, line, col int) int {
	lines := strings.Split(source, "\n")
	if line >= len(lines) {
		return 0
	}
	lineText := lines[line][:col]
	return strings.Count(lineText, ",")
}

// InlayHints handles textDocument/inlayHint.
func InlayHints(ctx *Context, params interface{}) interface{} {
	uri := extractURI(params)
	doc, ok := ctx.Store.GetDocument(uri)
	if !ok {
		return []interface{}{}
	}

	parseResult := doc.ParseResult()
	if parseResult == nil || parseResult.AST == nil {
		return []interface{}{}
	}

	hints := []interface{}{}
	collectInlayHints(parseResult.AST, &hints)
	return hints
}

func collectInlayHints(node *parser.Node, hints *[]interface{}) {
	if node == nil {
		return
	}

	// Implicit rescue: rescue => shows (StandardError)
	if node.Type == parser.NodeRescue && node.Detail == "" {
		*hints = append(*hints, map[string]interface{}{
			"position": map[string]interface{}{
				"line":      node.Range.Start.Line,
				"character": node.Range.Start.Character + 6, // after "rescue"
			},
			"label":        "StandardError",
			"kind":         2, // Type
			"paddingLeft":  true,
			"paddingRight": false,
		})
	}

	for _, child := range node.Children {
		collectInlayHints(child, hints)
	}
}
