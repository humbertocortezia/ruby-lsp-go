package indexer

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	localAssignmentPattern   = regexp.MustCompile(`(?m)^\s*(?:[a-z_][a-zA-Z0-9_]*\s*,\s*)*([a-z_][a-zA-Z0-9_]*)\s*=`)
	methodDeclarationPattern = regexp.MustCompile(`(?m)^\s*def\s+(?:self\.)?([a-zA-Z_][a-zA-Z0-9_]*[!?=]?)`)
	receiverCallPattern      = regexp.MustCompile(`([A-Z][A-Za-z0-9_:]*)\.(?:([a-zA-Z_][a-zA-Z0-9_]*[!?=]?))`)
)

// FindLocalVariableDefinition finds the nearest assignment for a local
// variable in the method containing the requested position. Ruby local
// variables are lexical, so resolving them from the current document is
// more accurate than treating them as workspace methods.
func FindLocalVariableDefinition(source string, line int, name string) (SymbolEntry, bool) {
	lines := strings.Split(source, "\n")
	if line < 0 || line >= len(lines) || name == "" {
		return SymbolEntry{}, false
	}

	start := methodStartLine(lines, line)
	for i := line; i >= start; i-- {
		match := localAssignmentPattern.FindStringSubmatchIndex(lines[i])
		if match == nil || match[2] < 0 {
			continue
		}
		assigned := lines[i][match[2]:match[3]]
		if assigned != name {
			continue
		}
		character := utf8.RuneCountInString(lines[i][:match[2]])
		return SymbolEntry{
			Name:               name,
			FullyQualifiedName: name,
			Type:               SymbolLocalVariable,
			Line:               i + 1,
			Character:          character,
			Parent:             enclosingMethodName(lines, i, start),
		}, true
	}
	return SymbolEntry{}, false
}

// GetReceiverAtPosition returns the constant receiver for a method call at
// the requested position, e.g. MyClass for MyClass.new().
func GetReceiverAtPosition(source string, line, character int) (string, bool) {
	lines := strings.Split(source, "\n")
	if line < 0 || line >= len(lines) || character < 0 {
		return "", false
	}
	lineText := lines[line]
	for _, match := range receiverCallPattern.FindAllStringSubmatchIndex(lineText, -1) {
		if match[2] < 0 || match[4] < 0 {
			continue
		}
		methodStart, methodEnd := match[4], match[5]
		if character >= methodStart && character <= methodEnd {
			return lineText[match[2]:match[3]], true
		}
	}
	return "", false
}

func methodStartLine(lines []string, line int) int {
	depth := 0
	for i := line; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if methodDeclarationPattern.MatchString(lines[i]) {
			return i
		}
		if strings.HasPrefix(trimmed, "end") {
			depth++
			continue
		}
		if depth > 0 && (strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "class ") || strings.HasPrefix(trimmed, "module ")) {
			depth--
		}
	}
	return 0
}

func enclosingMethodName(lines []string, line, start int) string {
	for i := line; i >= start; i-- {
		match := methodDeclarationPattern.FindStringSubmatch(lines[i])
		if len(match) > 1 {
			return match[1]
		}
	}
	return ""
}
