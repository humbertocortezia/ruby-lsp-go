package handlers

import (
	"strings"

	"github.com/humberto/ruby-lsp-go/documents"
	"github.com/humberto/ruby-lsp-go/parser"
)

// Semantic token type indices matching LSP legend.
const (
	TokenKeyword = 16
	TokenString  = 18
	TokenNumber  = 19
	TokenComment = 17
	TokenClass   = 2
	TokenMethod  = 13
	TokenVariable = 8
	TokenParameter = 7
	TokenOperator = 21
)

// SemanticTokensFull handles textDocument/semanticTokens/full.
func SemanticTokensFull(ctx *Context, params interface{}) interface{} {
	uri := extractURI(params)
	doc, ok := ctx.Store.GetDocument(uri)
	if !ok {
		return map[string]interface{}{"data": []int{}}
	}

	if doc.ShouldDelegate(0, 0) {
		return map[string]interface{}{"data": []int{}}
	}

	tokens := collectSemanticTokens(doc)
	if tokens == nil {
		tokens = []int{}
	}
	return map[string]interface{}{
		"data": tokens,
	}
}

// SemanticTokensRange handles textDocument/semanticTokens/range.
func SemanticTokensRange(ctx *Context, params interface{}) interface{} {
	return SemanticTokensFull(ctx, params)
}

func collectSemanticTokens(doc documents.Document) []int {
	parseResult := doc.ParseResult()
	if parseResult == nil || parseResult.AST == nil {
		return highlightFromSource(doc.Source())
	}
	return highlightFromAST(parseResult.AST, doc.Source())
}

func highlightFromSource(source string) []int {
	var tokens []int
	lines := strings.Split(source, "\n")
	prevLine := 0
	prevStart := 0

	for lineIdx, line := range lines {
		// Comments
		if idx := strings.Index(line, "#"); idx >= 0 {
			tokens = appendToken(tokens, lineIdx, prevLine, idx, prevStart, idx, len(line)-idx, TokenComment, 0)
			prevLine = lineIdx
			prevStart = idx
		}
		// Strings
		for _, quote := range []string{`"`, `'`, "`"} {
			if idx := strings.Index(line, quote); idx >= 0 {
				endIdx := strings.LastIndex(line, quote)
				if endIdx > idx {
					length := endIdx - idx + 1
					tokens = appendToken(tokens, lineIdx, prevLine, idx, prevStart, idx, length, TokenString, 0)
					prevLine = lineIdx
					prevStart = idx
				}
			}
		}
	}
	return tokens
}

func highlightFromAST(root *parser.Node, source string) []int {
	var tokens []int
	prevLine := 0
	prevStart := 0

	var walk func(*parser.Node)
	walk = func(node *parser.Node) {
		if node == nil {
			return
		}
		tokenType, modifiers := nodeToTokenType(node)
		if tokenType >= 0 {
			line := node.Range.Start.Line
			col := node.Range.Start.Character
			length := node.Range.End.Character - node.Range.Start.Character
			if length <= 0 && node.Name != "" {
				length = len(node.Name)
			}
			if length > 0 {
				tokens = appendToken(tokens, line, prevLine, col, prevStart, col, length, tokenType, modifiers)
				prevLine = line
				prevStart = col
			}
		}
		for _, child := range node.Children {
			walk(child)
		}
	}
	walk(root)

	if len(tokens) == 0 {
		return highlightFromSource(source)
	}
	return tokens
}

func nodeToTokenType(node *parser.Node) (int, int) {
	switch node.Type {
	case parser.NodeClass, parser.NodeModule:
		return TokenClass, 0
	case parser.NodeMethod, parser.NodeSingletonMethod:
		return TokenMethod, 0
	case parser.NodeConstant:
		return TokenClass, 0
	case parser.NodeInstanceVariable, parser.NodeClassVariable, parser.NodeGlobalVariable:
		return TokenVariable, 0
	case parser.NodeString:
		return TokenString, 0
	case parser.NodeSymbol:
		return TokenString, 0
	case parser.NodeComment:
		return TokenComment, 0
	case parser.NodeIf, parser.NodeUnless, parser.NodeWhile, parser.NodeUntil,
		parser.NodeFor, parser.NodeCase, parser.NodeWhen, parser.NodeBegin,
		parser.NodeRescue, parser.NodeEnsure, parser.NodeElse, parser.NodeElsif:
		return TokenKeyword, 0
	default:
		if isRubyKeyword(node.Name) {
			return TokenKeyword, 0
		}
		return -1, 0
	}
}

func isRubyKeyword(name string) bool {
	keywords := map[string]bool{
		"if": true, "else": true, "elsif": true, "end": true, "unless": true,
		"while": true, "until": true, "for": true, "do": true, "begin": true,
		"rescue": true, "ensure": true, "class": true, "module": true, "def": true,
		"return": true, "yield": true, "break": true, "next": true, "case": true,
		"when": true, "then": true, "true": true, "false": true, "nil": true,
		"self": true, "super": true, "and": true, "or": true, "not": true,
	}
	return keywords[name]
}

func appendToken(tokens []int, line, prevLine, col, prevStart, _, length, tokenType, modifiers int) []int {
	deltaLine := line - prevLine
	deltaStart := col
	if deltaLine > 0 {
		deltaStart = col
	} else {
		deltaStart = col - prevStart
	}
	return append(tokens, deltaLine, deltaStart, length, tokenType, modifiers)
}
