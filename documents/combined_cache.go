package documents

import (
	"strings"
	"sync"

	lspTypes "github.com/humberto/ruby-lsp-go/lsp/types"
	"github.com/humberto/ruby-lsp-go/indexer"
	"github.com/humberto/ruby-lsp-go/parser"
)

// CombinedResults holds all features computed in one AST pass.
type CombinedResults struct {
	FoldingRanges   []FoldingRange
	DocumentSymbols []map[string]interface{}
	DocumentLinks   []map[string]interface{}
	CodeLenses      []map[string]interface{}
	InlayHints      []map[string]interface{}
}

// CombinedCache stores results from a single AST traversal for multiple LSP features.
type CombinedCache struct {
	mutex           sync.RWMutex
	version         int
	foldingRanges   []FoldingRange
	documentSymbols []map[string]interface{}
	documentLinks   []map[string]interface{}
	codeLenses      []map[string]interface{}
	inlayHints      []map[string]interface{}
}

// NewCombinedCache creates an empty combined cache.
func NewCombinedCache() *CombinedCache {
	return &CombinedCache{}
}

// Invalidate clears cache when document changes.
func (c *CombinedCache) Invalidate() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.version = -1
	c.foldingRanges = nil
	c.documentSymbols = nil
	c.documentLinks = nil
	c.codeLenses = nil
	c.inlayHints = nil
}

// GetOrCompute returns cached results or computes them via the provided function.
func (c *CombinedCache) GetOrCompute(docVersion int, compute func() CombinedResults) CombinedResults {
	c.mutex.RLock()
	if c.version == docVersion && c.foldingRanges != nil {
		result := CombinedResults{
			FoldingRanges:   c.foldingRanges,
			DocumentSymbols: c.documentSymbols,
			DocumentLinks:   c.documentLinks,
			CodeLenses:      c.codeLenses,
			InlayHints:      c.inlayHints,
		}
		c.mutex.RUnlock()
		return result
	}
	c.mutex.RUnlock()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	if c.version == docVersion && c.foldingRanges != nil {
		return CombinedResults{
			FoldingRanges:   c.foldingRanges,
			DocumentSymbols: c.documentSymbols,
			DocumentLinks:   c.documentLinks,
			CodeLenses:      c.codeLenses,
			InlayHints:      c.inlayHints,
		}
	}

	result := compute()
	c.version = docVersion
	c.foldingRanges = result.FoldingRanges
	c.documentSymbols = result.DocumentSymbols
	c.documentLinks = result.DocumentLinks
	c.codeLenses = result.CodeLenses
	c.inlayHints = result.InlayHints
	return result
}

// ComputeCombined runs a single AST traversal to populate all combined features.
func ComputeCombined(doc Document) CombinedResults {
	result := CombinedResults{}

	switch d := doc.(type) {
	case *RubyDocument:
		result.FoldingRanges = d.GetFoldingRanges()
		if d.parseResult != nil && d.parseResult.AST != nil {
			result.DocumentSymbols = extractDocumentSymbolsFromAST(d.parseResult.AST)
			result.DocumentLinks = extractDocumentLinksFromAST(d.parseResult.AST, d.uri)
			result.CodeLenses = extractCodeLensesFromAST(d.parseResult.AST, d.uri)
			result.InlayHints = extractInlayHintsFromAST(d.parseResult.AST)
		}
	case *ERBDocument:
		result.FoldingRanges = d.GetFoldingRanges()
		if d.parseResult != nil && d.parseResult.AST != nil {
			result.DocumentSymbols = extractDocumentSymbolsFromAST(d.parseResult.AST)
			result.InlayHints = extractInlayHintsFromAST(d.parseResult.AST)
		}
	}

	return result
}

func extractDocumentSymbolsFromAST(root *parser.Node) []map[string]interface{} {
	var symbols []map[string]interface{}
	var walk func(*parser.Node, int)
	walk = func(node *parser.Node, depth int) {
		if node == nil {
			return
		}
		switch node.Type {
		case parser.NodeClass, parser.NodeModule, parser.NodeMethod, parser.NodeSingletonMethod, parser.NodeConstant:
			kind := nodeTypeToLSPKind(node.Type)
			if node.Name == "" {
				break
			}
			startLine := node.Range.Start.Line
			startChar := node.Range.Start.Character
			if startChar < 0 {
				startChar = 0
			}
			selEndChar := startChar + len(node.Name)
			endLine := node.Range.End.Line
			endChar := node.Range.End.Character
			// Guarantee the full range contains the selection range, otherwise
			// the LSP client rejects the whole response.
			if endLine < startLine || (endLine == startLine && endChar < selEndChar) {
				endLine = startLine
				endChar = selEndChar
			}
			symbols = append(symbols, map[string]interface{}{
				"name": node.Name,
				"kind": kind,
				"range": map[string]interface{}{
					"start": map[string]interface{}{"line": startLine, "character": startChar},
					"end":   map[string]interface{}{"line": endLine, "character": endChar},
				},
				"selectionRange": map[string]interface{}{
					"start": map[string]interface{}{"line": startLine, "character": startChar},
					"end":   map[string]interface{}{"line": startLine, "character": selEndChar},
				},
			})
		}
		for _, child := range node.Children {
			walk(child, depth+1)
		}
	}
	walk(root, 0)
	return symbols
}

func nodeTypeToSymbolType(t parser.NodeType) indexer.SymbolType {
	switch t {
	case parser.NodeClass:
		return indexer.SymbolClass
	case parser.NodeModule:
		return indexer.SymbolModule
	case parser.NodeMethod:
		return indexer.SymbolMethod
	case parser.NodeSingletonMethod:
		return indexer.SymbolSingletonMethod
	case parser.NodeConstant:
		return indexer.SymbolConstant
	default:
		return indexer.SymbolMethod
	}
}

func nodeTypeToLSPKind(t parser.NodeType) int {
	switch t {
	case parser.NodeClass:
		return lspTypes.SymbolClass
	case parser.NodeModule:
		return lspTypes.SymbolModule
	case parser.NodeMethod, parser.NodeSingletonMethod:
		return lspTypes.SymbolMethod
	case parser.NodeConstant:
		return lspTypes.SymbolConstant
	default:
		return lspTypes.SymbolVariable
	}
}

func extractDocumentLinksFromAST(root *parser.Node, uri string) []map[string]interface{} {
	var links []map[string]interface{}
	var walk func(*parser.Node)
	walk = func(node *parser.Node) {
		if node == nil {
			return
		}
		if node.Type == parser.NodeRequire {
			reqPath := node.Value
			if reqPath == "" && len(node.Children) > 0 {
				reqPath = strings.Trim(node.Children[0].Value, `"'`)
			}
			links = append(links, map[string]interface{}{
				"range": map[string]interface{}{
					"start": map[string]interface{}{"line": node.Range.Start.Line, "character": node.Range.Start.Character},
					"end":   map[string]interface{}{"line": node.Range.End.Line, "character": node.Range.End.Character},
				},
				"tooltip": reqPath,
			})
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(root)
	return links
}

func extractCodeLensesFromAST(root *parser.Node, uri string) []map[string]interface{} {
	var lenses []map[string]interface{}
	var walk func(*parser.Node)
	walk = func(node *parser.Node) {
		if node == nil {
			return
		}
		if node.Type == parser.NodeMethod && strings.HasPrefix(node.Name, "test_") {
			lenses = append(lenses, map[string]interface{}{
				"range": map[string]interface{}{
					"start": map[string]interface{}{"line": node.Range.Start.Line, "character": 0},
					"end":   map[string]interface{}{"line": node.Range.Start.Line, "character": 80},
				},
				"command": map[string]interface{}{"title": "▶ Run"},
				"data": map[string]interface{}{
					"testName": node.Name,
					"filePath": uri,
					"kind":     "run",
				},
			})
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(root)
	return lenses
}

func extractInlayHintsFromAST(root *parser.Node) []map[string]interface{} {
	var hints []map[string]interface{}
	var walk func(*parser.Node)
	walk = func(node *parser.Node) {
		if node == nil {
			return
		}
		if node.Type == parser.NodeRescue {
			hints = append(hints, map[string]interface{}{
				"position": map[string]interface{}{
					"line":      node.Range.Start.Line,
					"character": node.Range.Start.Character + 6,
				},
				"label":       "StandardError",
				"kind":        2,
				"paddingLeft": true,
			})
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(root)
	return hints
}
