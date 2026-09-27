package parser

import (
	"strings"
)

// ParseResult holds the result of parsing Ruby source.
type ParseResult struct {
	Source []byte
	AST    *Node
}

// Parse parses Ruby source and returns a simplified AST.
//
// This implementation is pure Go: it does not depend on tree-sitter, Prism,
// or cgo. It is intentionally permissive — it never returns an error for
// valid Ruby that hits a known edge case. Callers that need a real syntax
// check should run a separate Ruby tool (e.g. `ruby -c`) and feed the
// diagnostics back through the LSP `textDocument/diagnostic` channel.
func ParseSource(source string) (*ParseResult, error) {
	ast, err := Parse(source)
	if err != nil {
		return nil, err
	}
	return &ParseResult{
		Source: []byte(source),
		AST:    ast,
	}, nil
}

// GetWordAtPosition extracts word at LSP position from source string.
func GetWordAtPosition(source string, line, character int) string {
	lines := strings.Split(source, "\n")
	if line < 0 || line >= len(lines) {
		return ""
	}
	lineText := []rune(lines[line])
	if len(lineText) == 0 {
		return ""
	}
	if character < 0 {
		return ""
	}
	if character >= len(lineText) {
		character = len(lineText) - 1
	}
	if !isWordCharRune(lineText[character]) && character > 0 && isWordCharRune(lineText[character-1]) {
		character--
	}
	if !isWordCharRune(lineText[character]) {
		return ""
	}
	start := character
	for start > 0 && isWordCharRune(lineText[start-1]) {
		start--
	}
	end := character
	for end < len(lineText) && isWordCharRune(lineText[end]) {
		end++
	}
	return string(lineText[start:end])
}

func isWordCharRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') || r == '_' || r == ':' || r == '!' || r == '?' || r == '=' || r == '@'
}
