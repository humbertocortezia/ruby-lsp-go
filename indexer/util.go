package indexer

import (
	"strings"
	"unicode"

	lspTypes "github.com/humberto/ruby-lsp-go/lsp/types"
)

// skipDirs is the set of directory names that the workspace indexer should
// not descend into. They are typical Rails / Ruby project noise that does
// not contribute meaningful symbols to navigation or completion.
var skipDirs = map[string]bool{
	"vendor":       true,
	"node_modules": true,
	".git":         true,
	"tmp":          true,
	"log":          true,
	".bundle":      true,
	"coverage":     true,
	"public":       true,
	"storage":      true,
}

// camelToSnake converts a CamelCase identifier to snake_case, preserving
// the `::` namespace separator (rendered as `/` for filesystem lookups).
//
//	GTipoUnidadeGestora -> g_tipo_unidade_gestora
//	HTTP::Client        -> http/client
func camelToSnake(s string) string {
	s = strings.ReplaceAll(s, "::", "/")

	var result strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 && runes[i-1] != '/' {
				// Insert an underscore at the boundary between
				// consecutive uppercase + lowercase (HTMLParser -> html_parser)
				// or between a lowercase/digit and an uppercase (FooBar -> foo_bar).
				if i+1 < len(runes) && unicode.IsLower(runes[i+1]) && unicode.IsUpper(runes[i-1]) {
					result.WriteRune('_')
				} else if !unicode.IsUpper(runes[i-1]) {
					result.WriteRune('_')
				}
			}
			result.WriteRune(unicode.ToLower(r))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// capitalize upper-cases the first rune of s without altering the rest.
//
//	foo         -> Foo
//	g_unidade   -> G_unidade (intentional: the leading "g_" is the Rails
//	                      table-name prefix and is preserved as-is)
func capitalize(s string) string {
	if s == "" {
		return s
	}
	runes := []rune(s)
	if unicode.IsLower(runes[0]) {
		runes[0] = unicode.ToUpper(runes[0])
	}
	return string(runes)
}

// isCapitalized reports whether s starts with an uppercase letter.
func isCapitalized(s string) bool {
	if s == "" {
		return false
	}
	r := []rune(s)[0]
	return unicode.IsUpper(r)
}

// SymbolKindToLSP maps an indexer SymbolType to the LSP SymbolKind
// integer (see LSP §3.16).
func SymbolKindToLSP(t SymbolType) int {
	switch t {
	case SymbolClass:
		return lspTypes.SymbolClass
	case SymbolModule:
		return lspTypes.SymbolModule
	case SymbolMethod, SymbolSingletonMethod:
		return lspTypes.SymbolMethod
	case SymbolConstant:
		return lspTypes.SymbolConstant
	case SymbolScope, SymbolAssociation, SymbolAttrAccessor:
		return lspTypes.SymbolProperty
	default:
		return lspTypes.SymbolFile
	}
}

// CompletionKindFromType maps an indexer SymbolType to the LSP
// CompletionItemKind integer.
func CompletionKindFromType(t SymbolType) int {
	switch t {
	case SymbolClass:
		return lspTypes.CompletionClass
	case SymbolModule:
		return lspTypes.CompletionModule
	case SymbolMethod, SymbolSingletonMethod:
		return lspTypes.CompletionMethod
	case SymbolConstant:
		return lspTypes.CompletionValue
	case SymbolScope:
		return lspTypes.CompletionMethod
	case SymbolAssociation:
		return lspTypes.CompletionField
	case SymbolAttrAccessor:
		return lspTypes.CompletionProperty
	default:
		return lspTypes.CompletionText
	}
}

// SymbolTypeString returns a human-readable label for a symbol type,
// suitable for use in hover or signature help.
func SymbolTypeString(t SymbolType) string {
	switch t {
	case SymbolClass:
		return "class"
	case SymbolModule:
		return "module"
	case SymbolMethod:
		return "method"
	case SymbolSingletonMethod:
		return "class method"
	case SymbolConstant:
		return "constant"
	case SymbolScope:
		return "scope"
	case SymbolAssociation:
		return "association"
	case SymbolAttrAccessor:
		return "attribute"
	default:
		return "symbol"
	}
}
