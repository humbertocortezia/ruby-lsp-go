package documents

// RBSDocument represents an RBS type signature file.
type RBSDocument struct {
	BaseDocument
}

// NewRBSDocument creates an RBS document.
func NewRBSDocument(uri, source string, version int, languageID string) *RBSDocument {
	doc := &RBSDocument{
		BaseDocument: BaseDocument{
			uri:        uri,
			source:     source,
			version:    version,
			languageID: languageID,
		},
	}
	doc.parse()
	return doc
}

// ApplyEdits applies edits and re-parses.
func (d *RBSDocument) ApplyEdits(edits []TextEdit) {
	d.applyEdits(edits)
}

// InvalidateCache forces re-parse.
func (d *RBSDocument) InvalidateCache() {
	d.parse()
}

// GetFoldingRanges returns basic folding for RBS (class/module blocks).
func (d *RBSDocument) GetFoldingRanges() []FoldingRange {
	if d.parseResult == nil || d.parseResult.AST == nil {
		return nil
	}
	return extractFoldingRanges(d.parseResult.AST)
}
