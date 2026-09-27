package handlers

import (
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/humberto/ruby-lsp-go/documents"
	"github.com/humberto/ruby-lsp-go/indexer"
	"github.com/humberto/ruby-lsp-go/lsp/types"
	"github.com/humberto/ruby-lsp-go/parser"
	"github.com/humberto/ruby-lsp-go/store"
	"github.com/humberto/ruby-lsp-go/workspace"
)

// Context holds shared dependencies for LSP handlers.
type Context struct {
	State  *workspace.State
	Store  *store.Store
	Index  *indexer.IndexStore
	Logger *log.Logger
}

// Definition handles textDocument/definition.
func Definition(ctx *Context, params interface{}) interface{} {
	uri, pos := extractPosition(params)
	if uri == "" {
		return []interface{}{}
	}

	doc, ok := ctx.Store.GetDocument(uri)
	if !ok {
		return []interface{}{}
	}

	if doc.ShouldDelegate(pos.Line, pos.Character) {
		return delegateError()
	}

	word := parser.GetWordAtPosition(doc.Source(), pos.Line, pos.Character)
	if word == "" {
		return []interface{}{}
	}

	cleanWord := strings.TrimPrefix(word, ":")
	locations := []interface{}{}
	currentFilePath := uriToPath(uri)

	if ctx.Index != nil {
		// Try the symbol as-is, then capitalized form (handles
		// snake_case identifiers clicked on PascalCase classes).
		entries := ctx.Index.LookupLegacy(cleanWord)
		if len(entries) == 0 && !isCapitalized(cleanWord) {
			entries = ctx.Index.LookupLegacy(capitalize(cleanWord))
		}
		// Also try with common Rails table-name prefixes stripped
		// (e.g. "g_tipo_unidade_gestora" -> "tipo_unidade_gestora").
		if len(entries) == 0 {
			stripped := stripRailsTablePrefix(cleanWord)
			if stripped != cleanWord {
				entries = ctx.Index.LookupLegacy(capitalize(stripped))
				if len(entries) == 0 {
					entries = ctx.Index.LookupLegacy(stripped)
				}
			}
		}
		// Last resort: convention lookup (file-system based).
		if len(entries) == 0 {
			lookupWord := cleanWord
			if !isCapitalized(lookupWord) {
				lookupWord = capitalize(lookupWord)
			}
			entries = ctx.Index.LookupByConvention(lookupWord)
		}

		// Deduplicate and drop same-file hits that are at (or within
		// 1 line of) the click position. When the user clicks on a
		// PascalCase identifier in `include DataTable` and the index
		// also has the class declared in the *current* file (e.g.
		// `class EAmbientePredial < ApplicationRecord`), naively
		// returning the same-file hit makes the editor silently
		// no-op (cursor moves to the declaration in place, no new
		// tab), which is confusing. Returning a same-file hit for a
		// local token in `belongs_to :foo` has the same problem.
		//
		// We deliberately do NOT re-order by "same-file first" the
		// way the previous version did: re-ordering still picks the
		// local declaration and produces the no-op jump. The fix is
		// to *discard* the local hit so a cross-file hit (if any) is
		// what we return.
		//
		// Convention hits carry Line=1/Character=0, which makes the
		// editor select the whole first line of the file
		// ("selecionou tudo"); resolve them to the actual
		// declaration line of the file before returning.
		seen := make(map[string]bool)
		filtered := []indexer.SymbolEntry{}
		clickLine := pos.Line + 1 // pos is 0-based, entry.Line is 1-based
		for _, entry := range entries {
			key := entry.FilePath + ":" + indexItoa(entry.Line) + ":" + entry.Name
			if seen[key] {
				continue
			}
			seen[key] = true
			if entry.Line <= 1 && entry.Character == 0 {
				if precise, ok := locateClassInFile(entry.FilePath, entry.Name); ok {
					entry = precise
				}
			}
			if entry.FilePath == currentFilePath {
				lineDelta := entry.Line - clickLine
				if lineDelta < 0 {
					lineDelta = -lineDelta
				}
				if lineDelta <= 1 {
					// Same-file hit at the click position: the user
					// is already on the declaration. Drop it so the
					// next hit (typically cross-file) wins.
					continue
				}
			}
			filtered = append(filtered, entry)
		}
		if len(filtered) > 1 {
			filtered = filtered[:1]
		}
		for _, entry := range filtered {
			locations = append(locations, symbolEntryToLocation(entry))
		}
	}

	// AST-based fallback for identifiers in the same file (e.g. local
	// classes/modules) that may not be in the workspace index.
	if len(locations) == 0 {
		if node := doc.NodeAtPosition(pos.Line, pos.Character); node != nil && node.Name != "" {
			locations = append(locations, nodeToLocation(uri, node))
		}
	}

	return locations
}

// stripRailsTablePrefix removes the common single-letter or two-letter
// Rails table prefix (g_, p_, e_, c_, etc.) so that table_name -> class
// name lookups succeed.
func stripRailsTablePrefix(name string) string {
	idx := strings.Index(name, "_")
	if idx <= 0 || idx > 2 {
		return name
	}
	prefix := name[:idx]
	if !isCapitalized(prefix) {
		return name
	}
	return name[idx+1:]
}

// locateClassInFile finds the actual declaration line of a class/module in
// a file, so convention-based lookups do not return Line=1/Column=0 ranges
// (which cause the editor to select the entire first line).
func locateClassInFile(filePath, name string) (indexer.SymbolEntry, bool) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return indexer.SymbolEntry{}, false
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "class ") && !strings.HasPrefix(trimmed, "module ") {
			continue
		}
		idx := strings.Index(line, name)
		if idx < 0 {
			continue
		}
		// Sanity: identifier boundary check so "User" does not match "UserGroup".
		end := idx + len(name)
		if end < len(line) {
			next := line[end]
			if (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') ||
				(next >= '0' && next <= '9') || next == '_' {
				continue
			}
		}
		return indexer.SymbolEntry{
			Name:               name,
			FullyQualifiedName: name,
			Type:               indexer.SymbolClass,
			FilePath:           filePath,
			Line:               i + 1,
			Character:          idx,
		}, true
	}
	return indexer.SymbolEntry{}, false
}

func indexItoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := false
	if n < 0 {
		negative = true
		n = -n
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if negative {
		s = "-" + s
	}
	return s
}

// Hover handles textDocument/hover.
func Hover(ctx *Context, params interface{}) interface{} {
	uri, pos := extractPosition(params)
	if uri == "" {
		return map[string]interface{}{"contents": ""}
	}

	doc, ok := ctx.Store.GetDocument(uri)
	if !ok {
		return map[string]interface{}{"contents": ""}
	}

	if doc.ShouldDelegate(pos.Line, pos.Character) {
		return delegateError()
	}

	word := parser.GetWordAtPosition(doc.Source(), pos.Line, pos.Character)
	if word == "" {
		return map[string]interface{}{"contents": ""}
	}

	cleanWord := strings.TrimPrefix(word, ":")
	var entries []indexer.SymbolEntry

	if ctx.Index != nil {
		entries = ctx.Index.LookupLegacy(cleanWord)
		if len(entries) == 0 {
			entries = ctx.Index.LookupByConvention(cleanWord)
		}
	}

	if len(entries) == 0 {
		return map[string]interface{}{"contents": ""}
	}

	var mdParts []string
	for _, entry := range entries {
		typeStr := indexer.SymbolTypeString(entry.Type)
		relPath := entry.FilePath
		if ctx.State.WorkspacePath != "" {
			if rel, err := filepath.Rel(ctx.State.WorkspacePath, entry.FilePath); err == nil {
				relPath = rel
			}
		}
		header := "```ruby\n" + typeStr + " " + entry.FullyQualifiedName + "\n```"
		detail := "**Defined in:** `" + relPath + ":" + itoa(entry.Line) + "`"
		if entry.Detail != "" {
			detail += "\n\n**Detail:** `" + entry.Detail + "`"
		}
		mdParts = append(mdParts, header+"\n\n"+detail)
	}

	return map[string]interface{}{
		"contents": map[string]interface{}{
			"kind":  "markdown",
			"value": strings.Join(mdParts, "\n\n---\n\n"),
		},
	}
}

// Completion handles textDocument/completion.
func Completion(ctx *Context, params interface{}) interface{} {
	uri, pos := extractPosition(params)
	if uri == "" {
		return emptyCompletion()
	}

	doc, ok := ctx.Store.GetDocument(uri)
	if !ok {
		return emptyCompletion()
	}

	if doc.ShouldDelegate(pos.Line, pos.Character) {
		return delegateError()
	}

	word := parser.GetWordAtPosition(doc.Source(), pos.Line, pos.Character)

	var entries []indexer.SymbolEntry
	if ctx.Index != nil && word != "" {
		entries = ctx.Index.PrefixSearchLegacy(word)
	}

	// Add Ruby keywords
	if word != "" {
		for _, kw := range rubyKeywords {
			if strings.HasPrefix(kw, word) {
				entries = append(entries, indexer.SymbolEntry{Name: kw, Type: indexer.SymbolMethod})
			}
		}
	}

	items := []interface{}{}
	seen := make(map[string]bool)
	for _, entry := range entries {
		if seen[entry.Name] {
			continue
		}
		seen[entry.Name] = true
		items = append(items, map[string]interface{}{
			"label":  entry.Name,
			"kind":   indexer.CompletionKindFromType(entry.Type),
			"detail": indexer.SymbolTypeString(entry.Type),
		})
		if len(items) >= 50 {
			break
		}
	}

	return map[string]interface{}{
		"isIncomplete": len(items) >= 50,
		"items":        items,
	}
}

var rubyKeywords = []string{
	"if", "else", "elsif", "end", "unless", "while", "until", "for", "do", "begin",
	"rescue", "ensure", "class", "module", "def", "return", "yield", "break", "next",
	"case", "when", "then", "true", "false", "nil", "self", "super", "defined?",
	"and", "or", "not", "in", "require", "require_relative", "include", "extend",
	"attr_reader", "attr_writer", "attr_accessor", "private", "protected", "public",
}

func emptyCompletion() map[string]interface{} {
	return map[string]interface{}{"isIncomplete": false, "items": []interface{}{}}
}

// DocumentSymbol handles textDocument/documentSymbol.
func DocumentSymbol(ctx *Context, params interface{}) interface{} {
	uri := extractURI(params)
	if uri == "" {
		return []interface{}{}
	}

	doc, ok := ctx.Store.GetDocument(uri)
	if !ok {
		return []interface{}{}
	}

	cache := ctx.Store.GetCombinedCache(uri)
	results := cache.GetOrCompute(doc.Version(), func() documents.CombinedResults {
		return documents.ComputeCombined(doc)
	})

	if len(results.DocumentSymbols) > 0 {
		return results.DocumentSymbols
	}

	// Fallback to index
	filePath := uriToPath(uri)
	if ctx.Index != nil {
		entries := ctx.Index.GetFileSymbols(filePath)
		symbols := []interface{}{}
		for _, entry := range entries {
			symbols = append(symbols, symbolEntryToDocumentSymbol(entry))
		}
		return symbols
	}
	return []interface{}{}
}

// WorkspaceSymbol handles workspace/symbol.
func WorkspaceSymbol(ctx *Context, params interface{}) interface{} {
	if ctx.Index == nil {
		return []interface{}{}
	}

	query := ""
	if paramMap, ok := params.(map[string]interface{}); ok {
		if q, ok := paramMap["query"].(string); ok {
			query = q
		}
	}
	if len(query) < 2 {
		return []interface{}{}
	}

	entries := ctx.Index.PrefixSearchLegacy(query)
	symbols := []interface{}{}
	for _, entry := range entries {
		relPath := entry.FilePath
		if ctx.State.WorkspacePath != "" {
			if rel, err := filepath.Rel(ctx.State.WorkspacePath, entry.FilePath); err == nil {
				relPath = rel
			}
		}
		symbols = append(symbols, map[string]interface{}{
			"name": entry.FullyQualifiedName,
			"kind": indexer.SymbolKindToLSP(entry.Type),
			"location": symbolEntryToLocation(entry),
			"containerName": relPath,
		})
		if len(symbols) >= 50 {
			break
		}
	}
	return symbols
}

// References handles textDocument/references.
func References(ctx *Context, params interface{}) interface{} {
	uri, pos := extractPosition(params)
	if uri == "" || ctx.Index == nil {
		return []interface{}{}
	}

	doc, ok := ctx.Store.GetDocument(uri)
	if !ok {
		return []interface{}{}
	}

	word := parser.GetWordAtPosition(doc.Source(), pos.Line, pos.Character)
	if word == "" {
		return []interface{}{}
	}

	includeDeclaration := true
	if paramMap, ok := params.(map[string]interface{}); ok {
		if ctx_, ok := paramMap["context"].(map[string]interface{}); ok {
			if inc, ok := ctx_["includeDeclaration"].(bool); ok {
				includeDeclaration = inc
			}
		}
	}

	finder := indexer.NewReferenceFinder(ctx.Index)
	refs := finder.FindReferencesInDocument(doc.Source(), uri, word, includeDeclaration)

	results := []interface{}{}
	for _, ref := range refs {
		results = append(results, map[string]interface{}{
			"uri": ref.URI,
			"range": map[string]interface{}{
				"start": map[string]interface{}{"line": ref.Range.Start.Line, "character": ref.Range.Start.Character},
				"end":   map[string]interface{}{"line": ref.Range.End.Line, "character": ref.Range.End.Character},
			},
		})
	}
	return results
}

// PrepareRename handles textDocument/prepareRename.
func PrepareRename(ctx *Context, params interface{}) interface{} {
	uri, pos := extractPosition(params)
	doc, ok := ctx.Store.GetDocument(uri)
	if !ok {
		return nil
	}

	rng, placeholder, ok := indexer.PrepareRename(doc.Source(), pos.Line, pos.Character)
	if !ok {
		return nil
	}

	return map[string]interface{}{
		"range": map[string]interface{}{
			"start": map[string]interface{}{"line": rng.Start.Line, "character": rng.Start.Character},
			"end":   map[string]interface{}{"line": rng.End.Line, "character": rng.End.Character},
		},
		"placeholder": placeholder,
	}
}

// Rename handles textDocument/rename.
func Rename(ctx *Context, params interface{}) interface{} {
	uri, pos := extractPosition(params)
	if uri == "" || ctx.Index == nil {
		return nil
	}

	doc, ok := ctx.Store.GetDocument(uri)
	if !ok {
		return nil
	}

	newName := ""
	if paramMap, ok := params.(map[string]interface{}); ok {
		if n, ok := paramMap["newName"].(string); ok {
			newName = n
		}
	}
	if newName == "" {
		return nil
	}

	word := parser.GetWordAtPosition(doc.Source(), pos.Line, pos.Character)
	if word == "" {
		return nil
	}

	finder := indexer.NewReferenceFinder(ctx.Index)
	changes := indexer.BuildRenameEdit(finder, word, newName)

	changesMap := make(map[string]interface{})
	for uri, edits := range changes {
		var editList []interface{}
		for _, e := range edits {
			editList = append(editList, map[string]interface{}{
				"range": map[string]interface{}{
					"start": map[string]interface{}{"line": e.Range.Start.Line, "character": e.Range.Start.Character},
					"end":   map[string]interface{}{"line": e.Range.End.Line, "character": e.Range.End.Character},
				},
				"newText": e.NewText,
			})
		}
		changesMap[uri] = editList
	}

	return map[string]interface{}{"changes": changesMap}
}

// FoldingRange handles textDocument/foldingRange.
func FoldingRange(ctx *Context, params interface{}) interface{} {
	uri := extractURI(params)
	doc, ok := ctx.Store.GetDocument(uri)
	if !ok {
		return []interface{}{}
	}

	cache := ctx.Store.GetCombinedCache(uri)
	results := cache.GetOrCompute(doc.Version(), func() documents.CombinedResults {
		return documents.ComputeCombined(doc)
	})

	frs := []interface{}{}
	for _, fr := range results.FoldingRanges {
		item := map[string]interface{}{
			"startLine": fr.StartLine,
			"endLine":   fr.EndLine,
		}
		if fr.Kind != "" {
			item["kind"] = fr.Kind
		}
		frs = append(frs, item)
	}

	if len(frs) == 0 {
		switch d := doc.(type) {
		case *documents.RubyDocument:
			for _, fr := range d.GetFoldingRanges() {
				frs = append(frs, map[string]interface{}{
					"startLine": fr.StartLine,
					"endLine":   fr.EndLine,
				})
			}
		case *documents.ERBDocument:
			for _, fr := range d.GetFoldingRanges() {
				frs = append(frs, map[string]interface{}{
					"startLine": fr.StartLine,
					"endLine":   fr.EndLine,
				})
			}
		}
	}
	return frs
}

// SelectionRange handles textDocument/selectionRange.
func SelectionRange(ctx *Context, params interface{}) interface{} {
	uri := extractURI(params)
	if uri == "" {
		return []interface{}{}
	}

	var positions []documents.Position
	if paramMap, ok := params.(map[string]interface{}); ok {
		if posArr, ok := paramMap["positions"].([]interface{}); ok {
			for _, p := range posArr {
				if posMap, ok := p.(map[string]interface{}); ok {
					line, _ := posMap["line"].(float64)
					char, _ := posMap["character"].(float64)
					positions = append(positions, documents.Position{Line: int(line), Character: int(char)})
				}
			}
		}
	}

	doc, ok := ctx.Store.GetDocument(uri)
	if !ok {
		return []interface{}{}
	}

	var srs []documents.SelectionRange
	switch d := doc.(type) {
	case *documents.RubyDocument:
		srs = d.GetSelectionRanges(positions)
	case *documents.ERBDocument:
		srs = d.GetSelectionRanges(positions)
	}

	results := []interface{}{}
	for _, sr := range srs {
		results = append(results, selectionRangeToMap(sr))
	}
	return results
}

// DocumentHighlight handles textDocument/documentHighlight.
func DocumentHighlight(ctx *Context, params interface{}) interface{} {
	uri, pos := extractPosition(params)
	doc, ok := ctx.Store.GetDocument(uri)
	if !ok {
		return []interface{}{}
	}

	docPos := documents.Position{Line: pos.Line, Character: pos.Character}
	var highlights []documents.DocumentHighlight

	switch d := doc.(type) {
	case *documents.RubyDocument:
		highlights = d.GetDocumentHighlights(docPos)
	case *documents.ERBDocument:
		highlights = d.GetDocumentHighlights(docPos)
	}

	results := []interface{}{}
	for _, hl := range highlights {
		results = append(results, map[string]interface{}{
			"range": map[string]interface{}{
				"start": map[string]interface{}{"line": hl.Range.Start.Line, "character": hl.Range.Start.Character},
				"end":   map[string]interface{}{"line": hl.Range.End.Line, "character": hl.Range.End.Character},
			},
			"kind": hl.Kind,
		})
	}
	return results
}

// --- Helper functions ---

func extractPosition(params interface{}) (string, types.Position) {
	var uri string
	var pos types.Position
	if paramMap, ok := params.(map[string]interface{}); ok {
		if textDoc, ok := paramMap["textDocument"].(map[string]interface{}); ok {
			uri, _ = textDoc["uri"].(string)
		}
		if posParam, ok := paramMap["position"].(map[string]interface{}); ok {
			if line, ok := posParam["line"].(float64); ok {
				pos.Line = int(line)
			}
			if char, ok := posParam["character"].(float64); ok {
				pos.Character = int(char)
			}
		}
	}
	return uri, pos
}

func extractURI(params interface{}) string {
	if paramMap, ok := params.(map[string]interface{}); ok {
		if textDoc, ok := paramMap["textDocument"].(map[string]interface{}); ok {
			uri, _ := textDoc["uri"].(string)
			return uri
		}
	}
	return ""
}

func uriToPath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		return strings.TrimPrefix(uri, "file://")
	}
	return uri
}

func pathToURI(path string) string {
	if strings.HasPrefix(path, "/") {
		return "file://" + path
	}
	return "file:///" + path
}

func isCapitalized(s string) bool {
	return len(s) > 0 && s[0] >= 'A' && s[0] <= 'Z'
}

func capitalize(s string) string {
	parts := strings.Split(s, "_")
	var result strings.Builder
	for _, part := range parts {
		if len(part) > 0 {
			result.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
	}
	return result.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

func symbolEntryToLocation(entry indexer.SymbolEntry) map[string]interface{} {
	return map[string]interface{}{
		"uri": pathToURI(entry.FilePath),
		"range": map[string]interface{}{
			"start": map[string]interface{}{"line": entry.Line - 1, "character": entry.Character},
			"end":   map[string]interface{}{"line": entry.Line - 1, "character": entry.Character + len(entry.Name)},
		},
	}
}

func symbolEntryToDocumentSymbol(entry indexer.SymbolEntry) map[string]interface{} {
	line := entry.Line - 1
	if line < 0 {
		line = 0
	}
	startChar := entry.Character
	if startChar < 0 {
		startChar = 0
	}
	endChar := startChar + len(entry.Name)
	return map[string]interface{}{
		"name": entry.Name,
		"kind": indexer.SymbolKindToLSP(entry.Type),
		"range": map[string]interface{}{
			"start": map[string]interface{}{"line": line, "character": startChar},
			"end":   map[string]interface{}{"line": line, "character": endChar},
		},
		"selectionRange": map[string]interface{}{
			"start": map[string]interface{}{"line": line, "character": startChar},
			"end":   map[string]interface{}{"line": line, "character": endChar},
		},
	}
}

// nodeToLocation produces a tight, name-only range for textDocument/definition.
// We deliberately do NOT use node.Range.End because for blocks (class,
// module, def) that points at the closing `end`, not at the name. When
// the parser supplies a NamePosition, we anchor on that; otherwise we
// fall back to node.Range.Start (which points at the keyword) and
// derive the end column from len(node.Name).
func nodeToLocation(uri string, node *parser.Node) map[string]interface{} {
	startLine := node.Range.Start.Line
	startChar := node.Range.Start.Character
	if node.NamePosition.Line > 0 {
		startLine = node.NamePosition.Line - 1
	}
	if node.NamePosition.Character > 0 || node.NamePosition.Line > 0 {
		startChar = node.NamePosition.Character
	}
	endLine := startLine
	endChar := startChar + len(node.Name)
	return map[string]interface{}{
		"uri": uri,
		"range": map[string]interface{}{
			"start": map[string]interface{}{"line": startLine, "character": startChar},
			"end":   map[string]interface{}{"line": endLine, "character": endChar},
		},
		"selectionRange": map[string]interface{}{
			"start": map[string]interface{}{"line": startLine, "character": startChar},
			"end":   map[string]interface{}{"line": endLine, "character": endChar},
		},
	}
}

func selectionRangeToMap(sr documents.SelectionRange) map[string]interface{} {
	result := map[string]interface{}{
		"range": map[string]interface{}{
			"start": map[string]interface{}{"line": sr.Range.Start.Line, "character": sr.Range.Start.Character},
			"end":   map[string]interface{}{"line": sr.Range.End.Line, "character": sr.Range.End.Character},
		},
	}
	if sr.Parent != nil {
		result["parent"] = selectionRangeToMap(*sr.Parent)
	}
	return result
}

func delegateError() map[string]interface{} {
	return map[string]interface{}{
		"error": map[string]interface{}{
			"code":    parser.DelegateRequestError,
			"message": "Delegate to host language",
		},
	}
}
