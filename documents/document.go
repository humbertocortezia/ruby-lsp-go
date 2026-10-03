package documents

import (
	"github.com/humberto/ruby-lsp-go/parser"
	"strings"
)

// Document is the interface for all document types (Ruby, ERB, RBS).
type Document interface {
	URI() string
	Source() string
	Version() int
	LanguageID() string
	ParseResult() *parser.ParseResult
	ApplyEdits(edits []TextEdit)
	InvalidateCache()
	NodeAtPosition(line, col int) *parser.Node
	ShouldDelegate(line, col int) bool
}

// BaseDocument holds common document fields.
type BaseDocument struct {
	uri         string
	source      string
	version     int
	languageID  string
	parseResult *parser.ParseResult
}

func (d *BaseDocument) URI() string                      { return d.uri }
func (d *BaseDocument) Source() string                   { return d.source }
func (d *BaseDocument) Version() int                     { return d.version }
func (d *BaseDocument) LanguageID() string               { return d.languageID }
func (d *BaseDocument) ParseResult() *parser.ParseResult { return d.parseResult }
func (d *BaseDocument) ShouldDelegate(_ int, _ int) bool { return false }

func (d *BaseDocument) parse() {
	result, err := parser.ParseSource(d.source)
	d.parseResult = nil
	if err == nil {
		d.parseResult = result
	}
}

func (d *BaseDocument) applyEdits(edits []TextEdit) {
	d.source = ApplyTextEdits(d.source, edits)
	d.version++
	d.parse()
}

func (d *BaseDocument) NodeAtPosition(line, col int) *parser.Node {
	if d.parseResult == nil || d.parseResult.AST == nil {
		return nil
	}
	return parser.GetNodeAtPosition(d.parseResult.AST, parser.Position{Line: line, Character: col})
}

// NewDocument creates the appropriate document type based on language ID or URI.
func NewDocument(uri, source string, version int, languageID string) Document {
	if strings.HasSuffix(strings.ToLower(uri), ".erb") || strings.HasSuffix(strings.ToLower(uri), ".rhtml") {
		return NewERBDocument(uri, source, version, languageID)
	}
	switch languageID {
	case "erb", "html.erb":
		return NewERBDocument(uri, source, version, languageID)
	case "rbs":
		return NewRBSDocument(uri, source, version, languageID)
	default:
		return NewRubyDocument(uri, source, version, languageID)
	}
}

// NewDocumentFromExtension creates document based on file extension.
func NewDocumentFromExtension(uri, source string, version int) Document {
	if len(uri) > 4 && uri[len(uri)-4:] == ".erb" {
		return NewERBDocument(uri, source, version, "erb")
	}
	if len(uri) > 4 && uri[len(uri)-4:] == ".rbs" {
		return NewRBSDocument(uri, source, version, "rbs")
	}
	return NewRubyDocument(uri, source, version, "ruby")
}
