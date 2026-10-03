package documents

import (
	"github.com/humberto/ruby-lsp-go/parser"
)

// ERBDocument represents an ERB template with embedded Ruby.
type ERBDocument struct {
	BaseDocument
	scanner *parser.ERBScanner
}

// NewERBDocument creates an ERB document.
func NewERBDocument(uri, source string, version int, languageID string) *ERBDocument {
	doc := &ERBDocument{
		BaseDocument: BaseDocument{
			uri:        uri,
			source:     source,
			version:    version,
			languageID: languageID,
		},
	}
	doc.scanner = parser.NewERBScanner(source)
	doc.parseERB()
	return doc
}

func (d *ERBDocument) parseERB() {
	d.parseResult = nil
	if d.scanner.Err() != nil {
		return
	}
	rubySource := d.scanner.RubyContent()
	result, err := parser.ParseSource(rubySource)
	d.parseResult = nil
	if err == nil {
		d.parseResult = result
	}
}

// ApplyEdits applies edits and re-scans ERB.
func (d *ERBDocument) ApplyEdits(edits []TextEdit) {
	d.source = ApplyTextEdits(d.source, edits)
	d.version++
	d.scanner = parser.NewERBScanner(d.source)
	d.parseERB()
}

// InvalidateCache forces re-parse.
func (d *ERBDocument) InvalidateCache() {
	d.scanner = parser.NewERBScanner(d.source)
	d.parseERB()
}

// ShouldDelegate returns true if cursor is in host language (HTML).
func (d *ERBDocument) ShouldDelegate(line, col int) bool {
	if d.scanner == nil {
		return false
	}
	return d.scanner.InsideHostLanguageAtLineCol(line, col)
}

// RubyContent returns extracted Ruby source.
func (d *ERBDocument) RubyContent() string {
	if d.scanner == nil {
		return ""
	}
	return d.scanner.RubyContent()
}

// HostContent returns HTML/host content.
func (d *ERBDocument) HostContent() string {
	if d.scanner == nil {
		return ""
	}
	return d.scanner.HostContent()
}

// Scanner returns the ERB scanner for position mapping.
func (d *ERBDocument) Scanner() *parser.ERBScanner {
	return d.scanner
}

// GetFoldingRanges returns folding ranges for Ruby portions.
func (d *ERBDocument) GetFoldingRanges() []FoldingRange {
	if d.parseResult == nil || d.parseResult.AST == nil {
		return nil
	}
	return extractFoldingRanges(d.parseResult.AST)
}

// GetSelectionRanges returns selection ranges.
func (d *ERBDocument) GetSelectionRanges(positions []Position) []SelectionRange {
	if d.parseResult == nil || d.parseResult.AST == nil {
		return nil
	}
	var results []SelectionRange
	for _, pos := range positions {
		if d.ShouldDelegate(pos.Line, pos.Character) {
			continue
		}
		node := parser.GetNodeAtPosition(d.parseResult.AST, parser.Position{Line: pos.Line, Character: pos.Character})
		if node != nil {
			results = append(results, buildSelectionRange(node))
		}
	}
	return results
}

// GetDocumentHighlights returns highlights in Ruby portions.
func (d *ERBDocument) GetDocumentHighlights(pos Position) []DocumentHighlight {
	if d.ShouldDelegate(pos.Line, pos.Character) {
		return nil
	}
	if d.parseResult == nil || d.parseResult.AST == nil {
		return nil
	}
	targetNode := parser.GetNodeAtPosition(d.parseResult.AST, parser.Position{Line: pos.Line, Character: pos.Character})
	if targetNode == nil || targetNode.Name == "" {
		return nil
	}
	var highlights []DocumentHighlight
	findOccurrences(d.parseResult.AST, targetNode.Name, &highlights)
	return highlights
}
