package documents

import (
	"strings"
	"unicode/utf8"

	"github.com/humberto/ruby-lsp-go/parser"
)

// RubyDocument represents a Ruby source document.
type RubyDocument struct {
	uri         string
	source      string
	version     int
	languageID  string
	parseResult *parser.ParseResult
}

// NewRubyDocument creates a Ruby document and parses it.
func NewRubyDocument(uri, source string, version int, languageID string) *RubyDocument {
	doc := &RubyDocument{
		uri:        uri,
		source:     source,
		version:    version,
		languageID: languageID,
	}
	doc.parse()
	return doc
}

// New creates a RubyDocument (backward compat alias).
func New(uri, source string, version int, languageID string) *RubyDocument {
	return NewRubyDocument(uri, source, version, languageID)
}

func (d *RubyDocument) URI() string                      { return d.uri }
func (d *RubyDocument) Source() string                   { return d.source }
func (d *RubyDocument) Version() int                     { return d.version }
func (d *RubyDocument) LanguageID() string               { return d.languageID }
func (d *RubyDocument) ParseResult() *parser.ParseResult { return d.parseResult }
func (d *RubyDocument) ShouldDelegate(_, _ int) bool     { return false }

func (d *RubyDocument) parse() {
	result, err := parser.ParseSource(d.source)
	d.parseResult = nil
	if err == nil {
		d.parseResult = result
	}
}

// ApplyEdits applies LSP text edits and re-parses.
func (d *RubyDocument) ApplyEdits(edits []TextEdit) {
	d.source = ApplyTextEdits(d.source, edits)
	d.version++
	d.parse()
}

// InvalidateCache forces re-parse.
func (d *RubyDocument) InvalidateCache() {
	d.parse()
}

// NodeAtPosition returns AST node at LSP position.
func (d *RubyDocument) NodeAtPosition(line, col int) *parser.Node {
	if d.parseResult == nil || d.parseResult.AST == nil {
		return nil
	}
	return parser.GetNodeAtPosition(d.parseResult.AST, parser.Position{Line: line, Character: col})
}

// Edit represents an edit operation.
type Edit struct {
	Range *Range `json:"range"`
}

// Range represents a range in the document.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Position represents a position in the document.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// TextEdit represents a single text edit.
type TextEdit struct {
	Range   *Range `json:"range"`
	NewText string `json:"newText"`
}

// ApplyTextEdits applies LSP text edits without parsing the document.
func ApplyTextEdits(source string, edits []TextEdit) string {
	runes := []rune(source)
	doc := &RubyDocument{source: source}

	for i := len(edits) - 1; i >= 0; i-- {
		doc.applyEdit(&runes, edits[i])
	}
	return string(runes)
}

func (d *RubyDocument) applyEdit(source *[]rune, edit TextEdit) {
	if edit.Range == nil {
		*source = []rune(edit.NewText)
		return
	}
	startPos := d.positionToOffset(edit.Range.Start)
	endPos := d.positionToOffset(edit.Range.End)
	if startPos >= 0 && endPos <= len(*source) {
		newSource := make([]rune, 0, len(*source)-endPos+startPos+len([]rune(edit.NewText)))
		newSource = append(newSource, (*source)[:startPos]...)
		newSource = append(newSource, []rune(edit.NewText)...)
		newSource = append(newSource, (*source)[endPos:]...)
		*source = newSource
	}
}

func (d *RubyDocument) positionToOffset(pos Position) int {
	lines := strings.Split(d.source, "\n")
	offset := 0
	for i := 0; i < pos.Line && i < len(lines); i++ {
		offset += len([]rune(lines[i])) + 1
	}
	if pos.Line < len(lines) {
		line := []rune(lines[pos.Line])
		if pos.Character <= len(line) {
			return offset + pos.Character
		}
		return offset + len(line)
	}
	return len([]rune(d.source))
}

// GetFoldingRanges returns folding ranges for the document.
func (d *RubyDocument) GetFoldingRanges() []FoldingRange {
	if d.parseResult == nil || d.parseResult.AST == nil {
		return nil
	}
	return extractFoldingRanges(d.parseResult.AST)
}

// GetSelectionRanges returns selection ranges for the given positions.
func (d *RubyDocument) GetSelectionRanges(positions []Position) []SelectionRange {
	if d.parseResult == nil || d.parseResult.AST == nil {
		return nil
	}
	var results []SelectionRange
	for _, pos := range positions {
		node := parser.GetNodeAtPosition(d.parseResult.AST, parser.Position{Line: pos.Line, Character: pos.Character})
		if node != nil {
			results = append(results, buildSelectionRange(node))
		}
	}
	return results
}

// GetDocumentHighlights returns highlight ranges for a symbol at position.
func (d *RubyDocument) GetDocumentHighlights(pos Position) []DocumentHighlight {
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

// GetSymbolAtPosition returns the symbol at a given position.
func (d *RubyDocument) GetSymbolAtPosition(pos Position) *parser.Node {
	return d.NodeAtPosition(pos.Line, pos.Character)
}

// ComputeEndPosition computes the ending position of the document.
func (d *RubyDocument) ComputeEndPosition() Position {
	lines := strings.Split(d.source, "\n")
	lastLineIndex := len(lines) - 1
	lastLine := lines[lastLineIndex]
	return Position{
		Line:      lastLineIndex,
		Character: utf8.RuneCountInString(lastLine),
	}
}

// FoldingRange represents a foldable region.
type FoldingRange struct {
	StartLine      int    `json:"startLine"`
	EndLine        int    `json:"endLine"`
	StartCharacter int    `json:"startCharacter,omitempty"`
	EndCharacter   int    `json:"endCharacter,omitempty"`
	Kind           string `json:"kind,omitempty"`
}

func extractFoldingRanges(node *parser.Node) []FoldingRange {
	var ranges []FoldingRange
	if node == nil {
		return ranges
	}

	switch node.Type {
	case parser.NodeClass, parser.NodeModule, parser.NodeMethod, parser.NodeSingletonMethod,
		parser.NodeIf, parser.NodeUnless, parser.NodeCase, parser.NodeWhile, parser.NodeUntil,
		parser.NodeFor, parser.NodeBegin, parser.NodeBlock, parser.NodeLambda,
		parser.NodeRescue, parser.NodeEnsure, parser.NodeElsif, parser.NodeElse, parser.NodeWhen:
		if node.Range.End.Line > node.Range.Start.Line+1 {
			ranges = append(ranges, FoldingRange{
				StartLine: node.Range.Start.Line,
				EndLine:   node.Range.End.Line - 1,
				Kind:      "region",
			})
		}
	}

	for _, child := range node.Children {
		ranges = append(ranges, extractFoldingRanges(child)...)
	}
	return ranges
}

// SelectionRange represents a selection range hierarchy.
type SelectionRange struct {
	Range  Range           `json:"range"`
	Parent *SelectionRange `json:"parent,omitempty"`
}

func buildSelectionRange(node *parser.Node) SelectionRange {
	sr := SelectionRange{Range: fromParserRange(node.Range)}
	if node.Parent != nil && node.Parent.Type != parser.NodeProgram {
		parent := buildSelectionRange(node.Parent)
		sr.Parent = &parent
	}
	return sr
}

// DocumentHighlight represents a highlight in the document.
type DocumentHighlight struct {
	Range Range `json:"range"`
	Kind  int   `json:"kind,omitempty"`
}

func findOccurrences(node *parser.Node, name string, highlights *[]DocumentHighlight) {
	if node == nil {
		return
	}
	if node.Name == name {
		kind := 1
		switch node.Type {
		case parser.NodeMethod, parser.NodeSingletonMethod, parser.NodeClass, parser.NodeModule,
			parser.NodeConstant, parser.NodeInstanceVariable, parser.NodeClassVariable, parser.NodeGlobalVariable:
			kind = 3
		}
		*highlights = append(*highlights, DocumentHighlight{
			Range: fromParserRange(node.Range),
			Kind:  kind,
		})
	}
	for _, child := range node.Children {
		findOccurrences(child, name, highlights)
	}
}

func fromParserRange(r parser.Range) Range {
	return Range{
		Start: Position{Line: r.Start.Line, Character: r.Start.Character},
		End:   Position{Line: r.End.Line, Character: r.End.Character},
	}
}
