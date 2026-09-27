package types

// Position in a document (0-based line/character).
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range in a document.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Location with URI and range.
type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// TextEdit for document changes.
type TextEdit struct {
	Range   Range  `json:"range"`
	NewText string `json:"newText"`
}

// TextDocumentItem for didOpen.
type TextDocumentItem struct {
	URI        string `json:"uri"`
	Version    int    `json:"version"`
	LanguageID string `json:"languageId"`
	Text       string `json:"text"`
}

// TextDocumentIdentifier references a document by URI.
type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

// TextDocumentPositionParams for position-based requests.
type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     Position               `json:"position"`
}

// CompletionItem for completion responses.
type CompletionItem struct {
	Label         string      `json:"label"`
	Kind          int         `json:"kind,omitempty"`
	Detail        string      `json:"detail,omitempty"`
	Documentation interface{} `json:"documentation,omitempty"`
	InsertText    string      `json:"insertText,omitempty"`
	SortText      string      `json:"sortText,omitempty"`
}

// CompletionList wraps completion items.
type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

// Hover content.
type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

// MarkupContent for markdown hover.
type MarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// DocumentSymbol for outline.
type DocumentSymbol struct {
	Name           string           `json:"name"`
	Detail         string           `json:"detail,omitempty"`
	Kind           int              `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// SymbolInformation for workspace symbols.
type SymbolInformation struct {
	Name          string   `json:"name"`
	Kind          int      `json:"kind"`
	Location      Location `json:"location"`
	ContainerName string   `json:"containerName,omitempty"`
}

// FoldingRange for code folding.
type FoldingRange struct {
	StartLine      int    `json:"startLine"`
	EndLine        int    `json:"endLine"`
	StartCharacter int    `json:"startCharacter,omitempty"`
	EndCharacter   int    `json:"endCharacter,omitempty"`
	Kind           string `json:"kind,omitempty"`
}

// SelectionRange for smart selection.
type SelectionRange struct {
	Range  Range             `json:"range"`
	Parent *SelectionRange   `json:"parent,omitempty"`
}

// DocumentHighlight for occurrence highlighting.
type DocumentHighlight struct {
	Range Range `json:"range"`
	Kind  int   `json:"kind,omitempty"`
}

// Diagnostic for lint/syntax errors.
type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity,omitempty"`
	Code     string `json:"code,omitempty"`
	Source   string `json:"source,omitempty"`
	Message  string `json:"message"`
}

// SemanticTokens legend token types.
var SemanticTokenTypes = []string{
	"namespace", "type", "class", "enum", "interface", "struct",
	"typeParameter", "parameter", "variable", "property", "enumMember",
	"event", "function", "method", "macro", "keyword", "modifier",
	"comment", "string", "number", "regexp", "operator", "decorator",
}

// SemanticTokenModifiers legend.
var SemanticTokenModifiers = []string{
	"declaration", "definition", "readonly", "static", "deprecated",
	"abstract", "async", "modification", "documentation", "defaultLibrary",
}

// SignatureHelp for method call signatures.
type SignatureHelp struct {
	Signatures      []SignatureInformation `json:"signatures"`
	ActiveSignature int                    `json:"activeSignature"`
	ActiveParameter int                    `json:"activeParameter"`
}

// SignatureInformation describes a method signature.
type SignatureInformation struct {
	Label         string              `json:"label"`
	Documentation interface{}         `json:"documentation,omitempty"`
	Parameters    []ParameterInformation `json:"parameters,omitempty"`
}

// ParameterInformation for signature help.
type ParameterInformation struct {
	Label         interface{} `json:"label"`
	Documentation interface{} `json:"documentation,omitempty"`
}

// InlayHint for inline hints.
type InlayHint struct {
	Position   Position    `json:"position"`
	Label      interface{} `json:"label"`
	Kind       int         `json:"kind,omitempty"`
	PaddingLeft  bool      `json:"paddingLeft,omitempty"`
	PaddingRight bool      `json:"paddingRight,omitempty"`
}

// CodeLens for test run buttons.
type CodeLens struct {
	Range   Range                  `json:"range"`
	Command map[string]interface{} `json:"command,omitempty"`
	Data    interface{}            `json:"data,omitempty"`
}

// CodeAction for quick fixes.
type CodeAction struct {
	Title       string      `json:"title"`
	Kind        string      `json:"kind,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	Edit        *WorkspaceEdit `json:"edit,omitempty"`
}

// WorkspaceEdit for multi-file edits.
type WorkspaceEdit struct {
	Changes map[string][]TextEdit `json:"changes,omitempty"`
}

// DocumentLink for require paths.
type DocumentLink struct {
	Range  Range  `json:"range"`
	Target string `json:"target,omitempty"`
	Tooltip string `json:"tooltip,omitempty"`
}

// TypeHierarchyItem for type hierarchy.
type TypeHierarchyItem struct {
	Name  string `json:"name"`
	Kind  int    `json:"kind"`
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

// LSP SymbolKind constants.
const (
	SymbolFile          = 1
	SymbolModule        = 2
	SymbolNamespace     = 3
	SymbolPackage       = 4
	SymbolClass         = 5
	SymbolMethod        = 6
	SymbolProperty      = 7
	SymbolField         = 8
	SymbolConstructor   = 9
	SymbolEnum          = 10
	SymbolInterface     = 11
	SymbolFunction      = 12
	SymbolVariable      = 13
	SymbolConstant      = 14
	SymbolString        = 15
	SymbolNumber        = 16
	SymbolBoolean       = 17
	SymbolArray         = 18
	SymbolObject        = 19
	SymbolKey           = 20
	SymbolNull          = 21
	SymbolEnumMember    = 22
	SymbolStruct        = 23
	SymbolEvent         = 24
	SymbolOperator      = 25
	SymbolTypeParameter = 26
)

// Indexer symbol kinds. These are used internally by the workspace indexer
// and are distinct from the LSP SymbolKind constants above. Values are
// kept stable for serialization; new kinds must be appended.
type SymbolType int

const (
	IndexerSymbolNamespace       SymbolType = iota // 0
	IndexerSymbolClass                             // 1
	IndexerSymbolModule                            // 2
	IndexerSymbolMethod                            // 3
	IndexerSymbolSingletonMethod                   // 4
	IndexerSymbolClassVariable                     // 5
	IndexerSymbolAttrAccessor                      // 6
	IndexerSymbolConstant                          // 7
	IndexerSymbolInstanceVariable                  // 8
	IndexerSymbolGlobalVariable                    // 9
	IndexerSymbolScope                             // 10
	IndexerSymbolAssociation                       // 11
)

// SymbolEntry is a flat, serializable representation of an indexed Ruby
// symbol. It is intentionally simpler than the richer Entry interface in
// package indexer; the latter is converted to this form via
// indexer.EntryToSymbolEntry for downstream consumers.
type SymbolEntry struct {
	Name               string
	FullyQualifiedName string
	Type               SymbolType
	FilePath           string
	Line               int
	EndLine            int
	Character          int
	EndCharacter       int
	Parent             string
	Visibility         string
	Detail             string
	Parameters         []string
}

// CompletionItemKind constants.
const (
	CompletionText          = 1
	CompletionMethod        = 2
	CompletionFunction      = 3
	CompletionConstructor   = 4
	CompletionField         = 5
	CompletionVariable      = 6
	CompletionClass         = 7
	CompletionInterface     = 8
	CompletionModule        = 9
	CompletionProperty      = 10
	CompletionUnit          = 11
	CompletionValue         = 12
	CompletionEnum          = 13
	CompletionKeyword       = 14
	CompletionSnippet       = 15
	CompletionColor         = 16
	CompletionFile          = 17
	CompletionReference     = 18
	CompletionFolder        = 19
	CompletionEnumMember    = 20
	CompletionConstant      = 21
	CompletionStruct        = 22
	CompletionEvent         = 23
	CompletionOperator      = 24
	CompletionTypeParameter = 25
)
