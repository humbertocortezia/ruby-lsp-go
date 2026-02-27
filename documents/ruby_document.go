package documents

import (
	"strings"
	"unicode/utf8"
)

// RubyDocument represents a Ruby source document
type RubyDocument struct {
	URI        string
	Version    int
	Source     string
	LanguageID string
	LastEdit   *Edit
}

// Edit represents an edit operation
type Edit struct {
	Range *Range `json:"range"`
}

// Range represents a range in the document
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Position represents a position in the document
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Node represents a Ruby AST node
type Node struct {
	Type      string  `json:"type"`
	Name      string  `json:"name"`
	Location  *Range  `json:"location"`
	Children  []*Node `json:"children"`
}

// New creates a new RubyDocument
func New(uri string, source string, version int, languageID string) *RubyDocument {
	doc := &RubyDocument{
		URI:        uri,
		Version:    version,
		Source:     source,
		LanguageID: languageID,
		LastEdit:   nil,
	}
	
	return doc
}

// Parse parses the Ruby document and returns an AST
func (r *RubyDocument) Parse() (*Node, error) {
	// This is a simplified parser for demonstration purposes
	// In a real implementation, we would use a Ruby parser like Prism (Ruby 3.2+) or Ripper
	nodes := r.tokenize()
	return &Node{
		Type:     "program",
		Name:     "root",
		Location: &Range{Start: Position{Line: 0, Character: 0}, End: r.computeEndPosition()},
		Children: nodes,
	}, nil
}

// tokenize creates a basic tokenization for the Ruby document
func (r *RubyDocument) tokenize() []*Node {
	lines := strings.Split(r.Source, "\n")
	nodes := make([]*Node, 0)

	for i, line := range lines {
		lineNodes := r.parseLine(line, i)
		nodes = append(nodes, lineNodes...)
	}

	return nodes
}

// parseLine parses a single line for relevant Ruby constructs
func (r *RubyDocument) parseLine(line string, lineNumber int) []*Node {
	nodes := make([]*Node, 0)

	// Look for class definitions
	if strings.HasPrefix(strings.TrimSpace(line), "class ") {
		className := r.extractClassName(line)
		start := strings.Index(line, "class ")
		
		nodes = append(nodes, &Node{
			Type: "class",
			Name: className,
			Location: &Range{
				Start: Position{Line: lineNumber, Character: start},
				End:   Position{Line: lineNumber, Character: start + 5 + len(className)}, // "class " + className
			},
		})
	}

	// Look for method definitions
	if strings.HasPrefix(strings.TrimSpace(line), "def ") {
		methodName := r.extractMethodName(line)
		start := strings.Index(line, "def ")
		
		nodes = append(nodes, &Node{
			Type: "method",
			Name: methodName,
			Location: &Range{
				Start: Position{Line: lineNumber, Character: start},
				End:   Position{Line: lineNumber, Character: start + 4 + len(methodName)}, // "def " + methodName
			},
		})
	}

	// Look for module definitions
	if strings.HasPrefix(strings.TrimSpace(line), "module ") {
		moduleName := r.extractModuleName(line)
		start := strings.Index(line, "module ")
		
		nodes = append(nodes, &Node{
			Type: "module",
			Name: moduleName,
			Location: &Range{
				Start: Position{Line: lineNumber, Character: start},
				End:   Position{Line: lineNumber, Character: start + 7 + len(moduleName)}, // "module " + moduleName
			},
		})
	}

	return nodes
}

// extractClassName extracts the class name from a class definition
func (r *RubyDocument) extractClassName(line string) string {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "class ") {
		return ""
	}
	
	parts := strings.Fields(trimmed)
	if len(parts) < 2 {
		return ""
	}
	
	// Remove inheritance part if present (e.g., "class MyClass < Parent")
	namePart := strings.Split(parts[1], "<")[0]
	return strings.TrimSpace(namePart)
}

// extractMethodName extracts the method name from a method definition
func (r *RubyDocument) extractMethodName(line string) string {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "def ") {
		return ""
	}
	
	parts := strings.Fields(trimmed)
	if len(parts) < 2 {
		return ""
	}
	
	// Remove parameters part if present (e.g., "def my_method(param1, param2)")
	namePart := strings.Split(parts[1], "(")[0]
	return strings.TrimSpace(namePart)
}

// extractModuleName extracts the module name from a module definition
func (r *RubyDocument) extractModuleName(line string) string {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "module ") {
		return ""
	}
	
	parts := strings.Fields(trimmed)
	if len(parts) < 2 {
		return ""
	}
	
	return strings.TrimSpace(parts[1])
}

// computeEndPosition computes the ending position of the document
func (r *RubyDocument) computeEndPosition() Position {
	lines := strings.Split(r.Source, "\n")
	lastLineIndex := len(lines) - 1
	lastLine := lines[lastLineIndex]
	
	return Position{
		Line:      lastLineIndex,
		Character: utf8.RuneCountInString(lastLine),
	}
}

// Update applies text edits to the document
func (r *RubyDocument) Update(edits []TextEdit) {
	source := []rune(r.Source)
	
	// Apply edits in reverse order to maintain position consistency
	for i := len(edits) - 1; i >= 0; i-- {
		edit := edits[i]
		r.applyEdit(&source, edit)
	}
	
	r.Source = string(source)
	r.Version++
}

// TextEdit represents a single text edit
type TextEdit struct {
	Range   *Range `json:"range"`
	NewText string `json:"newText"`
}

// applyEdit applies a single text edit to the source
func (r *RubyDocument) applyEdit(source *[]rune, edit TextEdit) {
	startPos := r.positionToOffset(edit.Range.Start)
	endPos := r.positionToOffset(edit.Range.End)
	
	if startPos >= 0 && endPos <= len(*source) {
		newSource := make([]rune, 0, len(*source)-endPos+startPos+len([]rune(edit.NewText)))
		newSource = append(newSource, (*source)[:startPos]...)
		newSource = append(newSource, []rune(edit.NewText)...)
		newSource = append(newSource, (*source)[endPos:]...)
		*source = newSource
	}
}

// positionToOffset converts a position to a rune offset in the source
func (r *RubyDocument) positionToOffset(pos Position) int {
	lines := strings.Split(r.Source, "\n")
	offset := 0
	
	for i := 0; i < pos.Line && i < len(lines); i++ {
		offset += len([]rune(lines[i])) + 1 // +1 for newline
	}
	
	if pos.Line < len(lines) {
		line := []rune(lines[pos.Line])
		if pos.Character <= len(line) {
			return offset + pos.Character
		}
		return offset + len(line)
	}
	
	return len([]rune(r.Source))
}

// GetSymbolAtPosition returns the symbol at a given position
func (r *RubyDocument) GetSymbolAtPosition(pos Position) *Node {
	ast, err := r.Parse()
	if err != nil {
		return nil
	}
	
	return r.findNodeAtPosition(ast, pos)
}

// findNodeAtPosition recursively finds the node at a given position
func (r *RubyDocument) findNodeAtPosition(node *Node, pos Position) *Node {
	if node.Location.Contains(pos) {
		for _, child := range node.Children {
			if found := r.findNodeAtPosition(child, pos); found != nil {
				return found
			}
		}
		return node
	}
	return nil
}

// Contains checks if a position is within a range
func (r *Range) Contains(pos Position) bool {
	if pos.Line < r.Start.Line || pos.Line > r.End.Line {
		return false
	}
	
	if pos.Line == r.Start.Line && pos.Character < r.Start.Character {
		return false
	}
	
	if pos.Line == r.End.Line && pos.Character > r.End.Character {
		return false
	}
	
	return true
}

