package parser

import (
	"fmt"
	"strings"
	"unicode"
)

// TokenType represents the type of a token
type TokenType int

const (
	TokenEOF TokenType = iota
	TokenNewline
	TokenWhitespace
	TokenComment
	TokenIdentifier
	TokenConstant
	TokenInstanceVariable
	TokenClassVariable
	TokenGlobalVariable
	TokenSymbol
	TokenString
	TokenNumber
	TokenKeyword
	TokenOperator
	TokenPunctuation
	TokenRegex
	TokenHEREDoc
)

// Token represents a lexical token
type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

// NodeType represents the type of an AST node
type NodeType string

const (
	NodeProgram          NodeType = "program"
	NodeClass            NodeType = "class"
	NodeModule           NodeType = "module"
	NodeSingletonClass   NodeType = "singleton_class"
	NodeMethod           NodeType = "method"
	NodeSingletonMethod  NodeType = "singleton_method"
	NodeBlock            NodeType = "block"
	NodeIf               NodeType = "if"
	NodeUnless           NodeType = "unless"
	NodeElsif            NodeType = "elsif"
	NodeElse             NodeType = "else"
	NodeCase             NodeType = "case"
	NodeWhen             NodeType = "when"
	NodeWhile            NodeType = "while"
	NodeUntil            NodeType = "until"
	NodeFor              NodeType = "for"
	NodeBegin            NodeType = "begin"
	NodeRescue           NodeType = "rescue"
	NodeEnsure           NodeType = "ensure"
	NodeLambda           NodeType = "lambda"
	NodeAssignment       NodeType = "assignment"
	NodeConstant         NodeType = "constant"
	NodeInstanceVariable NodeType = "instance_variable"
	NodeClassVariable    NodeType = "class_variable"
	NodeGlobalVariable   NodeType = "global_variable"
	NodeAttrAccessor     NodeType = "attr_accessor"
	NodeAlias            NodeType = "alias"
	NodeRequire          NodeType = "require"
	NodeCall             NodeType = "call"
	NodeHash             NodeType = "hash"
	NodeArray            NodeType = "array"
	NodeString           NodeType = "string"
	NodeSymbol           NodeType = "symbol"
	NodeComment          NodeType = "comment"
)

// Position represents a position in the source
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range represents a range in the source
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

// Node represents an AST node
type Node struct {
	Type         NodeType `json:"type"`
	Name         string   `json:"name,omitempty"`
	NamePosition Position `json:"namePosition,omitempty"`
	Value        string   `json:"value,omitempty"`
	Range        Range    `json:"range"`
	Children     []*Node  `json:"children,omitempty"`
	Parent       *Node    `json:"-"`
	Visibility   string   `json:"visibility,omitempty"`
	Detail       string   `json:"detail,omitempty"`
}

// Contains checks if a position is within the node's range
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

// RubyParser parses Ruby source code into a simplified AST
type RubyParser struct {
	source string
	tokens []Token
	pos    int
	lines  []string
}

// Parse parses Ruby source code and returns an AST
func Parse(source string) (ast *Node, err error) {
	defer Recover(&err)
	return parse(source)
}

func parse(source string) (*Node, error) {
	parser := &RubyParser{
		source: source,
		lines:  strings.Split(source, "\n"),
	}

	if err := parser.tokenize(); err != nil {
		return nil, err
	}
	ast := parser.parse()

	return ast, nil
}

// tokenize converts the source into tokens
func (p *RubyParser) tokenize() error {
	p.tokens = []Token{}
	line := 0
	column := 0

	for i := 0; i < len(p.source); i++ {
		ch := p.source[i]

		if ch == '\n' {
			p.tokens = append(p.tokens, Token{Type: TokenNewline, Literal: "\n", Line: line, Column: column})
			line++
			column = 0
			continue
		}

		if ch == ' ' || ch == '\t' || ch == '\r' {
			p.tokens = append(p.tokens, Token{Type: TokenWhitespace, Literal: string(ch), Line: line, Column: column})
			column++
			continue
		}

		if ch == '#' {
			// Comment
			start := i
			for i < len(p.source) && p.source[i] != '\n' {
				i++
			}
			p.tokens = append(p.tokens, Token{
				Type:    TokenComment,
				Literal: p.source[start:i],
				Line:    line,
				Column:  column,
			})
			column += i - start
			i-- // back up one since the loop will increment
			continue
		}

		if ch == '"' || ch == '\'' {
			end, err := p.scanLiteral(i, i, TokenString, &line, &column)
			if err != nil {
				return err
			}
			i = end - 1 // the outer loop advances to the next byte
			continue
		}

		if ch == '/' && i+1 < len(p.source) && p.source[i+1] != ' ' && p.source[i+1] != '\n' {
			// Could be regex - check previous token
			isRegex := true
			if len(p.tokens) > 0 {
				prev := p.tokens[len(p.tokens)-1]
				if prev.Type == TokenIdentifier || prev.Type == TokenConstant || prev.Type == TokenNumber ||
					prev.Type == TokenString || prev.Type == TokenSymbol || prev.Type == TokenPunctuation &&
					(prev.Literal == ")" || prev.Literal == "]") {
					isRegex = false
				}
			}

			if isRegex {
				end, err := p.scanLiteral(i, i, TokenRegex, &line, &column)
				if err != nil {
					return err
				}
				i = end - 1
				continue
			}
		}

		if ch == ':' && i+1 < len(p.source) {
			nextCh := p.source[i+1]
			if nextCh == '\'' || nextCh == '"' {
				end, err := p.scanLiteral(i, i+1, TokenSymbol, &line, &column)
				if err != nil {
					return err
				}
				i = end - 1
				continue
			} else if unicode.IsLetter(rune(nextCh)) || nextCh == '_' {
				// Symbol literal
				start := i
				i++
				for i < len(p.source) && (unicode.IsLetter(rune(p.source[i])) || unicode.IsDigit(rune(p.source[i])) || p.source[i] == '_') {
					i++
				}
				p.tokens = append(p.tokens, Token{
					Type:    TokenSymbol,
					Literal: p.source[start:i],
					Line:    line,
					Column:  column,
				})
				column += i - start
				i--
				continue
			}
		}

		if ch == '@' && i+1 < len(p.source) && p.source[i+1] == '@' {
			// Class variable
			start := i
			i += 2
			for i < len(p.source) && (unicode.IsLetter(rune(p.source[i])) || unicode.IsDigit(rune(p.source[i])) || p.source[i] == '_') {
				i++
			}
			p.tokens = append(p.tokens, Token{
				Type:    TokenClassVariable,
				Literal: p.source[start:i],
				Line:    line,
				Column:  column,
			})
			column += i - start
			i--
			continue
		}

		if ch == '@' {
			// Instance variable
			start := i
			i++
			for i < len(p.source) && (unicode.IsLetter(rune(p.source[i])) || unicode.IsDigit(rune(p.source[i])) || p.source[i] == '_') {
				i++
			}
			p.tokens = append(p.tokens, Token{
				Type:    TokenInstanceVariable,
				Literal: p.source[start:i],
				Line:    line,
				Column:  column,
			})
			column += i - start
			i--
			continue
		}

		if ch == '$' {
			// Global variable
			start := i
			i++
			for i < len(p.source) && (unicode.IsLetter(rune(p.source[i])) || unicode.IsDigit(rune(p.source[i])) || p.source[i] == '_') {
				i++
			}
			p.tokens = append(p.tokens, Token{
				Type:    TokenGlobalVariable,
				Literal: p.source[start:i],
				Line:    line,
				Column:  column,
			})
			column += i - start
			i--
			continue
		}

		if unicode.IsDigit(rune(ch)) {
			start := i
			for i < len(p.source) && (unicode.IsDigit(rune(p.source[i])) || p.source[i] == '.' || p.source[i] == '_') {
				i++
			}
			p.tokens = append(p.tokens, Token{
				Type:    TokenNumber,
				Literal: p.source[start:i],
				Line:    line,
				Column:  column,
			})
			column += i - start
			i--
			continue
		}

		if unicode.IsUpper(rune(ch)) {
			// Constant
			start := i
			for i < len(p.source) && (unicode.IsLetter(rune(p.source[i])) || unicode.IsDigit(rune(p.source[i])) || p.source[i] == '_' || p.source[i] == ':') {
				i++
			}
			p.tokens = append(p.tokens, Token{
				Type:    TokenConstant,
				Literal: p.source[start:i],
				Line:    line,
				Column:  column,
			})
			column += i - start
			i--
			continue
		}

		if unicode.IsLetter(rune(ch)) || ch == '_' {
			start := i
			for i < len(p.source) && (unicode.IsLetter(rune(p.source[i])) || unicode.IsDigit(rune(p.source[i])) || p.source[i] == '_' || p.source[i] == '!' || p.source[i] == '?' || p.source[i] == '=') {
				i++
			}
			literal := p.source[start:i]

			tokenType := TokenIdentifier
			if isKeyword(literal) {
				tokenType = TokenKeyword
			}

			p.tokens = append(p.tokens, Token{
				Type:    tokenType,
				Literal: literal,
				Line:    line,
				Column:  column,
			})
			column += i - start
			i--
			continue
		}

		if isOperator(string(ch)) {
			// Multi-char operators
			if i+1 < len(p.source) {
				twoChar := p.source[i : i+2]
				if isOperator(twoChar) {
					p.tokens = append(p.tokens, Token{
						Type:    TokenOperator,
						Literal: twoChar,
						Line:    line,
						Column:  column,
					})
					column += 2
					i++
					continue
				}
			}
			p.tokens = append(p.tokens, Token{
				Type:    TokenOperator,
				Literal: string(ch),
				Line:    line,
				Column:  column,
			})
			column++
			continue
		}

		if ch == '(' || ch == ')' || ch == '{' || ch == '}' || ch == '[' || ch == ']' || ch == ',' || ch == ';' || ch == '.' {
			p.tokens = append(p.tokens, Token{
				Type:    TokenPunctuation,
				Literal: string(ch),
				Line:    line,
				Column:  column,
			})
			column++
			continue
		}

		// Unknown character, skip
		column++
	}

	p.tokens = append(p.tokens, Token{Type: TokenEOF, Literal: "", Line: line, Column: column})
	return nil
}

// scanLiteral returns an exclusive end offset only after finding an unescaped
// closing delimiter. Incomplete tokens are never passed to AST consumers.
func (p *RubyParser) scanLiteral(start, opening int, kind TokenType, line, column *int) (int, error) {
	delimiter := p.source[opening]
	end := opening + 1
	for end < len(p.source) {
		ch := p.source[end]
		end++
		if ch == '\\' {
			if end < len(p.source) {
				end++
			}
			continue
		}
		if ch != delimiter {
			continue
		}
		literal := p.source[start:end]
		p.tokens = append(p.tokens, Token{Type: kind, Literal: literal, Line: *line, Column: *column})
		for i := 0; i < len(literal); i++ {
			if literal[i] == '\n' {
				*line++
				*column = 0
			} else {
				*column++
			}
		}
		return end, nil
	}
	name := "string"
	if kind == TokenRegex {
		name = "regex"
	}
	if kind == TokenSymbol {
		name = "quoted symbol"
	}
	return end, &IncompleteLiteralError{Kind: name, Line: *line, Column: *column}
}

// parse builds the AST from tokens
func (p *RubyParser) parse() *Node {
	program := &Node{
		Type: NodeProgram,
		Range: Range{
			Start: Position{Line: 0, Character: 0},
			End:   Position{Line: len(p.lines) - 1, Character: len(p.lines[len(p.lines)-1])},
		},
	}

	p.pos = 0

	for p.pos < len(p.tokens) {
		p.skipWhitespaceAndNewlines()
		if p.pos >= len(p.tokens) || p.tokens[p.pos].Type == TokenEOF {
			break
		}

		// Skip orphaned 'end' tokens at the top level
		if p.tokens[p.pos].Type == TokenKeyword && p.tokens[p.pos].Literal == "end" {
			p.pos++
			continue
		}

		node := p.parseStatement()
		if node != nil {
			node.Parent = program
			program.Children = append(program.Children, node)
		}
	}

	return program
}

// parseStatement parses a single statement
func (p *RubyParser) parseStatement() *Node {
	p.skipWhitespaceAndNewlines()

	if p.pos >= len(p.tokens) || p.tokens[p.pos].Type == TokenEOF {
		return nil
	}

	tok := p.tokens[p.pos]

	// Skip comments at statement level
	if tok.Type == TokenComment {
		p.pos++
		return p.parseComment()
	}

	if tok.Type == TokenKeyword {
		switch tok.Literal {
		case "class":
			return p.parseClass()
		case "module":
			return p.parseModule()
		case "def":
			return p.parseMethod()
		case "if":
			return p.parseIf()
		case "unless":
			return p.parseUnless()
		case "case":
			return p.parseCase()
		case "while":
			return p.parseWhile()
		case "until":
			return p.parseUntil()
		case "for":
			return p.parseFor()
		case "begin":
			return p.parseBegin()
		case "alias":
			return p.parseAlias()
		case "return", "next", "break", "yield", "redo", "retry", "super":
			return p.parseKeywordStatement()
		}
	}

	if tok.Type == TokenConstant && p.peekToken().Literal == "=" {
		return p.parseConstantAssignment()
	}

	if tok.Type == TokenInstanceVariable && p.peekToken().Literal == "=" {
		return p.parseInstanceVariableAssignment()
	}

	if tok.Type == TokenClassVariable && p.peekToken().Literal == "=" {
		return p.parseClassVariableAssignment()
	}

	if tok.Type == TokenGlobalVariable && p.peekToken().Literal == "=" {
		return p.parseGlobalVariableAssignment()
	}

	// Check for attr_accessor, attr_reader, attr_writer
	if tok.Type == TokenIdentifier && (tok.Literal == "attr_accessor" || tok.Literal == "attr_reader" || tok.Literal == "attr_writer") {
		return p.parseAttrAccessor()
	}

	// Check for visibility modifiers with arguments
	if tok.Type == TokenIdentifier && (tok.Literal == "private" || tok.Literal == "protected" || tok.Literal == "public") {
		return p.parseVisibilityModifier()
	}

	// Check for include/extend/prepend
	if tok.Type == TokenIdentifier && (tok.Literal == "include" || tok.Literal == "extend" || tok.Literal == "prepend") {
		return p.parseModuleOperation()
	}

	// Check for scope, belongs_to, has_many, etc.
	if tok.Type == TokenIdentifier && isRailsMacro(tok.Literal) {
		return p.parseRailsMacro()
	}

	// Check for require/require_relative
	if tok.Type == TokenIdentifier && (tok.Literal == "require" || tok.Literal == "require_relative") {
		return p.parseRequire()
	}

	// Default: parse as expression
	return p.parseExpression()
}

// parseClass parses a class definition
func (p *RubyParser) parseClass() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	p.pos++ // consume 'class'

	p.skipWhitespaceAndNewlines()

	className := ""
	nameLine := 0
	nameCol := 0
	if p.pos < len(p.tokens) && (p.tokens[p.pos].Type == TokenConstant || p.tokens[p.pos].Type == TokenIdentifier) {
		className = p.tokens[p.pos].Literal
		nameLine = p.tokens[p.pos].Line
		nameCol = p.tokens[p.pos].Column
		p.pos++
	}

	// Skip superclass if present
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenKeyword && p.tokens[p.pos].Literal != "end" {
		if p.tokens[p.pos].Type == TokenNewline {
			break
		}
		p.pos++
	}

	classNode := &Node{
		Type: NodeClass,
		Name: className,
		NamePosition: Position{
			Line:      nameLine,
			Character: nameCol,
		},
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol},
		},
	}

	p.parseBody(classNode, "end")

	// Update end position
	if len(classNode.Children) > 0 {
		lastChild := classNode.Children[len(classNode.Children)-1]
		classNode.Range.End = Position{Line: lastChild.Range.End.Line + 1, Character: 0}
	} else {
		classNode.Range.End = Position{Line: startLine + 1, Character: 3} // "end"
	}

	return classNode
}

// parseModule parses a module definition
func (p *RubyParser) parseModule() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	p.pos++ // consume 'module'

	p.skipWhitespaceAndNewlines()

	moduleName := ""
	nameLine := 0
	nameCol := 0
	if p.pos < len(p.tokens) && (p.tokens[p.pos].Type == TokenConstant || p.tokens[p.pos].Type == TokenIdentifier) {
		moduleName = p.tokens[p.pos].Literal
		nameLine = p.tokens[p.pos].Line
		nameCol = p.tokens[p.pos].Column
		p.pos++
	}

	moduleNode := &Node{
		Type: NodeModule,
		Name: moduleName,
		NamePosition: Position{
			Line:      nameLine,
			Character: nameCol,
		},
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol},
		},
	}

	p.parseBody(moduleNode, "end")

	if len(moduleNode.Children) > 0 {
		lastChild := moduleNode.Children[len(moduleNode.Children)-1]
		moduleNode.Range.End = Position{Line: lastChild.Range.End.Line + 1, Character: 0}
	} else {
		moduleNode.Range.End = Position{Line: startLine + 1, Character: 3}
	}

	return moduleNode
}

// parseMethod parses a method definition
func (p *RubyParser) parseMethod() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	p.pos++ // consume 'def'

	nodeType := NodeMethod
	methodName := ""
	nameLine := 0
	nameCol := 0
	p.skipWhitespaceAndNewlines()

	if p.pos < len(p.tokens) && p.tokens[p.pos].Literal == "self" {
		p.pos++ // consume 'self'
		p.skipWhitespaceAndNewlines()
		if p.pos < len(p.tokens) && p.tokens[p.pos].Literal == "." {
			p.pos++ // consume '.'
			p.skipWhitespaceAndNewlines()
			nodeType = NodeSingletonMethod
		}
	}

	if p.pos < len(p.tokens) && (p.tokens[p.pos].Type == TokenIdentifier || p.tokens[p.pos].Type == TokenConstant || p.tokens[p.pos].Type == TokenOperator) {
		methodName = p.tokens[p.pos].Literal
		nameLine = p.tokens[p.pos].Line
		nameCol = p.tokens[p.pos].Column
		p.pos++
	}

	// Skip parameters
	if p.pos < len(p.tokens) && p.tokens[p.pos].Literal == "(" {
		p.skipParens()
	}

	// Skip to end of line
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenNewline {
		if p.tokens[p.pos].Type == TokenEOF {
			break
		}
		p.pos++
	}

	methodNode := &Node{
		Type: nodeType,
		Name: methodName,
		NamePosition: Position{
			Line:      nameLine,
			Character: nameCol,
		},
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol},
		},
	}

	p.parseBody(methodNode, "end")

	if len(methodNode.Children) > 0 {
		lastChild := methodNode.Children[len(methodNode.Children)-1]
		methodNode.Range.End = Position{Line: lastChild.Range.End.Line + 1, Character: 0}
	} else {
		methodNode.Range.End = Position{Line: startLine + 1, Character: 3}
	}

	return methodNode
}

// parseIf parses an if statement
func (p *RubyParser) parseIf() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	p.pos++ // consume 'if'

	ifNode := &Node{
		Type: NodeIf,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol},
		},
	}

	// Skip condition
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenNewline && p.tokens[p.pos].Type != TokenEOF &&
		!(p.tokens[p.pos].Type == TokenKeyword && p.tokens[p.pos].Literal == "then") {
		p.pos++
	}
	if p.pos < len(p.tokens) && p.tokens[p.pos].Literal == "then" {
		p.pos++
	}

	p.parseBodyWithElsif(ifNode, "end")

	if len(ifNode.Children) > 0 {
		lastChild := ifNode.Children[len(ifNode.Children)-1]
		ifNode.Range.End = Position{Line: lastChild.Range.End.Line + 1, Character: 0}
	} else {
		ifNode.Range.End = Position{Line: startLine + 1, Character: 3}
	}

	return ifNode
}

// parseUnless parses an unless statement
func (p *RubyParser) parseUnless() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	p.pos++ // consume 'unless'

	unlessNode := &Node{
		Type: NodeUnless,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol},
		},
	}

	// Skip condition
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenNewline && p.tokens[p.pos].Type != TokenEOF &&
		!(p.tokens[p.pos].Type == TokenKeyword && p.tokens[p.pos].Literal == "then") {
		p.pos++
	}
	if p.pos < len(p.tokens) && p.tokens[p.pos].Literal == "then" {
		p.pos++
	}

	p.parseBody(unlessNode, "end")

	if len(unlessNode.Children) > 0 {
		lastChild := unlessNode.Children[len(unlessNode.Children)-1]
		unlessNode.Range.End = Position{Line: lastChild.Range.End.Line + 1, Character: 0}
	} else {
		unlessNode.Range.End = Position{Line: startLine + 1, Character: 3}
	}

	return unlessNode
}

// parseCase parses a case statement
func (p *RubyParser) parseCase() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	p.pos++ // consume 'case'

	caseNode := &Node{
		Type: NodeCase,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol},
		},
	}

	// Skip expression
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenNewline && p.tokens[p.pos].Type != TokenEOF {
		p.pos++
	}

	p.parseBodyWithWhen(caseNode, "end")

	if len(caseNode.Children) > 0 {
		lastChild := caseNode.Children[len(caseNode.Children)-1]
		caseNode.Range.End = Position{Line: lastChild.Range.End.Line + 1, Character: 0}
	} else {
		caseNode.Range.End = Position{Line: startLine + 1, Character: 3}
	}

	return caseNode
}

// parseWhile parses a while loop
func (p *RubyParser) parseWhile() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	p.pos++ // consume 'while'

	whileNode := &Node{
		Type: NodeWhile,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol},
		},
	}

	// Skip condition
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenNewline && p.tokens[p.pos].Type != TokenEOF &&
		!(p.tokens[p.pos].Type == TokenKeyword && p.tokens[p.pos].Literal == "do") {
		p.pos++
	}
	if p.pos < len(p.tokens) && p.tokens[p.pos].Literal == "do" {
		p.pos++
	}

	p.parseBody(whileNode, "end")

	if len(whileNode.Children) > 0 {
		lastChild := whileNode.Children[len(whileNode.Children)-1]
		whileNode.Range.End = Position{Line: lastChild.Range.End.Line + 1, Character: 0}
	} else {
		whileNode.Range.End = Position{Line: startLine + 1, Character: 3}
	}

	return whileNode
}

// parseUntil parses an until loop
func (p *RubyParser) parseUntil() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	p.pos++ // consume 'until'

	untilNode := &Node{
		Type: NodeUntil,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol},
		},
	}

	// Skip condition
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenNewline && p.tokens[p.pos].Type != TokenEOF &&
		!(p.tokens[p.pos].Type == TokenKeyword && p.tokens[p.pos].Literal == "do") {
		p.pos++
	}
	if p.pos < len(p.tokens) && p.tokens[p.pos].Literal == "do" {
		p.pos++
	}

	p.parseBody(untilNode, "end")

	if len(untilNode.Children) > 0 {
		lastChild := untilNode.Children[len(untilNode.Children)-1]
		untilNode.Range.End = Position{Line: lastChild.Range.End.Line + 1, Character: 0}
	} else {
		untilNode.Range.End = Position{Line: startLine + 1, Character: 3}
	}

	return untilNode
}

// parseFor parses a for loop
func (p *RubyParser) parseFor() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	p.pos++ // consume 'for'

	forNode := &Node{
		Type: NodeFor,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol},
		},
	}

	// Skip iteration variables and expression
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenNewline && p.tokens[p.pos].Type != TokenEOF &&
		!(p.tokens[p.pos].Type == TokenKeyword && p.tokens[p.pos].Literal == "do") {
		p.pos++
	}
	if p.pos < len(p.tokens) && p.tokens[p.pos].Literal == "do" {
		p.pos++
	}

	p.parseBody(forNode, "end")

	if len(forNode.Children) > 0 {
		lastChild := forNode.Children[len(forNode.Children)-1]
		forNode.Range.End = Position{Line: lastChild.Range.End.Line + 1, Character: 0}
	} else {
		forNode.Range.End = Position{Line: startLine + 1, Character: 3}
	}

	return forNode
}

// parseBegin parses a begin block
func (p *RubyParser) parseBegin() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	p.pos++ // consume 'begin'

	beginNode := &Node{
		Type: NodeBegin,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol},
		},
	}

	p.parseBodyWithRescue(beginNode, "end")

	if len(beginNode.Children) > 0 {
		lastChild := beginNode.Children[len(beginNode.Children)-1]
		beginNode.Range.End = Position{Line: lastChild.Range.End.Line + 1, Character: 0}
	} else {
		beginNode.Range.End = Position{Line: startLine + 1, Character: 3}
	}

	return beginNode
}

// parseAlias parses an alias statement
func (p *RubyParser) parseAlias() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	p.pos++ // consume 'alias'

	p.skipWhitespaceAndNewlines()

	newName := ""
	if p.pos < len(p.tokens) {
		newName = p.tokens[p.pos].Literal
		p.pos++
	}

	p.skipWhitespaceAndNewlines()

	oldName := ""
	if p.pos < len(p.tokens) {
		oldName = p.tokens[p.pos].Literal
		p.pos++
	}

	return &Node{
		Type:  NodeAlias,
		Name:  newName,
		Value: oldName,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol + len("alias") + len(newName) + len(oldName) + 2},
		},
	}
}

// parseAttrAccessor parses attr_accessor, attr_reader, attr_writer
func (p *RubyParser) parseAttrAccessor() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	accessorType := startTok.Literal
	p.pos++

	p.skipWhitespaceAndNewlines()

	// Collect attribute names
	var attrs []string
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenNewline && p.tokens[p.pos].Type != TokenEOF {
		if p.tokens[p.pos].Type == TokenSymbol || p.tokens[p.pos].Type == TokenString {
			attrName := p.tokens[p.pos].Literal
			if p.tokens[p.pos].Type == TokenSymbol {
				attrName = strings.TrimPrefix(attrName, ":")
			} else if p.tokens[p.pos].Type == TokenString {
				attrName = strings.Trim(attrName, `"'`)
			}
			attrs = append(attrs, attrName)
		}
		p.pos++
	}

	attrNode := &Node{
		Type:   NodeAttrAccessor,
		Name:   accessorType,
		Value:  strings.Join(attrs, ", "),
		Detail: accessorType,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol},
		},
	}

	// Find end of line
	endCol := startCol
	if startLine < len(p.lines) {
		endCol = len(p.lines[startLine])
	}
	attrNode.Range.End = Position{Line: startLine, Character: endCol}

	return attrNode
}

// parseVisibilityModifier parses private/protected/public
func (p *RubyParser) parseVisibilityModifier() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	visibility := startTok.Literal
	p.pos++

	p.skipWhitespaceAndNewlines()

	// Check if followed by method names or method definition
	if p.pos < len(p.tokens) && (p.tokens[p.pos].Type == TokenIdentifier || p.tokens[p.pos].Type == TokenSymbol || p.tokens[p.pos].Type == TokenString) {
		// Method names listed
		var methods []string
		for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenNewline && p.tokens[p.pos].Type != TokenEOF {
			if p.tokens[p.pos].Type == TokenIdentifier || p.tokens[p.pos].Type == TokenSymbol || p.tokens[p.pos].Type == TokenString {
				methodName := p.tokens[p.pos].Literal
				if p.tokens[p.pos].Type == TokenSymbol {
					methodName = strings.TrimPrefix(methodName, ":")
				} else if p.tokens[p.pos].Type == TokenString {
					methodName = strings.Trim(methodName, `"'`)
				}
				methods = append(methods, methodName)
			}
			p.pos++
		}

		return &Node{
			Type:       NodeCall,
			Name:       visibility,
			Value:      strings.Join(methods, ", "),
			Visibility: visibility,
			Range: Range{
				Start: Position{Line: startLine, Character: startCol},
				End:   Position{Line: startLine, Character: startCol},
			},
		}
	}

	// Just a visibility change statement
	return &Node{
		Type:       NodeCall,
		Name:       visibility,
		Visibility: visibility,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol + len(visibility)},
		},
	}
}

// parseModuleOperation parses include/extend/prepend
func (p *RubyParser) parseModuleOperation() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	operation := startTok.Literal
	p.pos++

	p.skipWhitespaceAndNewlines()

	moduleName := ""
	if p.pos < len(p.tokens) && (p.tokens[p.pos].Type == TokenConstant || p.tokens[p.pos].Type == TokenIdentifier) {
		moduleName = p.tokens[p.pos].Literal
		p.pos++
	}

	return &Node{
		Type:  NodeCall,
		Name:  operation,
		Value: moduleName,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol},
		},
	}
}

// parseRailsMacro parses Rails macros like scope, belongs_to, has_many, etc.
func (p *RubyParser) parseRailsMacro() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	macroName := startTok.Literal
	p.pos++

	p.skipWhitespaceAndNewlines()

	macroValue := ""
	if p.pos < len(p.tokens) && (p.tokens[p.pos].Type == TokenSymbol || p.tokens[p.pos].Type == TokenString || p.tokens[p.pos].Type == TokenIdentifier) {
		macroValue = p.tokens[p.pos].Literal
		if p.tokens[p.pos].Type == TokenSymbol {
			macroValue = strings.TrimPrefix(macroValue, ":")
		} else if p.tokens[p.pos].Type == TokenString {
			macroValue = strings.Trim(macroValue, `"'`)
		}
		p.pos++
	}

	// Skip rest of line
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenNewline && p.tokens[p.pos].Type != TokenEOF {
		p.pos++
	}

	nodeType := NodeCall
	if macroName == "scope" {
		nodeType = NodeMethod // Scopes are callable
	}

	return &Node{
		Type:   nodeType,
		Name:   macroValue,
		Value:  macroName,
		Detail: macroName,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol},
		},
	}
}

// parseRequire parses require/require_relative
func (p *RubyParser) parseRequire() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	requireType := startTok.Literal
	p.pos++

	p.skipWhitespaceAndNewlines()

	moduleName := ""
	if p.pos < len(p.tokens) && (p.tokens[p.pos].Type == TokenString || p.tokens[p.pos].Type == TokenIdentifier) {
		moduleName = p.tokens[p.pos].Literal
		if p.tokens[p.pos].Type == TokenString {
			moduleName = strings.Trim(moduleName, `"'`)
		}
		p.pos++
	}

	return &Node{
		Type:  NodeRequire,
		Name:  requireType,
		Value: moduleName,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol},
		},
	}
}

// parseConstantAssignment parses constant assignment
func (p *RubyParser) parseConstantAssignment() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	constName := startTok.Literal
	p.pos += 2 // consume constant and '='

	return &Node{
		Type: NodeConstant,
		Name: constName,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol + len(constName)},
		},
	}
}

// parseInstanceVariableAssignment parses instance variable assignment
func (p *RubyParser) parseInstanceVariableAssignment() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	varName := startTok.Literal
	p.pos += 2 // consume variable and '='

	return &Node{
		Type: NodeInstanceVariable,
		Name: varName,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol + len(varName)},
		},
	}
}

// parseClassVariableAssignment parses class variable assignment
func (p *RubyParser) parseClassVariableAssignment() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	varName := startTok.Literal
	p.pos += 2 // consume variable and '='

	return &Node{
		Type: NodeClassVariable,
		Name: varName,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol + len(varName)},
		},
	}
}

// parseGlobalVariableAssignment parses global variable assignment
func (p *RubyParser) parseGlobalVariableAssignment() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	varName := startTok.Literal
	p.pos += 2 // consume variable and '='

	return &Node{
		Type: NodeGlobalVariable,
		Name: varName,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol + len(varName)},
		},
	}
}

// parseKeywordStatement parses return, next, break, yield, etc.
func (p *RubyParser) parseKeywordStatement() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	keyword := startTok.Literal
	p.pos++

	// Skip rest of line
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenNewline && p.tokens[p.pos].Type != TokenEOF {
		p.pos++
	}

	return &Node{
		Type: NodeCall,
		Name: keyword,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol + len(keyword)},
		},
	}
}

// parseExpression parses a generic expression
func (p *RubyParser) parseExpression() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column

	// Skip to end of statement
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenNewline && p.tokens[p.pos].Type != TokenEOF &&
		!(p.tokens[p.pos].Type == TokenKeyword && p.tokens[p.pos].Literal == "end") {
		// Check for inline if/unless
		if p.tokens[p.pos].Type == TokenKeyword && (p.tokens[p.pos].Literal == "if" || p.tokens[p.pos].Literal == "unless") {
			// This is an inline modifier, skip condition
			p.pos++
			for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenNewline && p.tokens[p.pos].Type != TokenEOF {
				p.pos++
			}
			break
		}
		p.pos++
	}

	return &Node{
		Type: NodeCall,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol},
		},
	}
}

// parseComment parses a comment
func (p *RubyParser) parseComment() *Node {
	startTok := p.tokens[p.pos-1]
	return &Node{
		Type:  NodeComment,
		Value: startTok.Literal,
		Range: Range{
			Start: Position{Line: startTok.Line, Character: startTok.Column},
			End:   Position{Line: startTok.Line, Character: startTok.Column + len(startTok.Literal)},
		},
	}
}

// parseBody parses the body of a block until an end keyword is found
func (p *RubyParser) parseBody(parent *Node, endKeyword string) {
	depth := 1

	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenEOF {
		p.skipWhitespaceAndNewlines()

		if p.pos >= len(p.tokens) || p.tokens[p.pos].Type == TokenEOF {
			break
		}

		// Check for nested structures that increase depth
		if p.tokens[p.pos].Type == TokenKeyword {
			switch p.tokens[p.pos].Literal {
			case "class", "module", "def", "if", "unless", "case", "while", "until", "for", "begin", "do":
				depth++
			case "end":
				depth--
				if depth == 0 {
					p.pos++ // consume 'end'
					return
				}
			}
		}

		node := p.parseStatement()
		if node != nil {
			node.Parent = parent
			parent.Children = append(parent.Children, node)
		} else if p.pos < len(p.tokens) && p.tokens[p.pos].Type == TokenKeyword && p.tokens[p.pos].Literal == "end" {
			// If parseStatement didn't advance and we're on 'end', let the depth check handle it
			p.pos++
			depth--
			if depth == 0 {
				return
			}
		}
	}
}

// parseBodyWithElsif parses body with elsif/else support
func (p *RubyParser) parseBodyWithElsif(parent *Node, endKeyword string) {
	depth := 1

	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenEOF {
		p.skipWhitespaceAndNewlines()

		if p.pos >= len(p.tokens) || p.tokens[p.pos].Type == TokenEOF {
			break
		}

		if p.tokens[p.pos].Type == TokenKeyword {
			switch p.tokens[p.pos].Literal {
			case "elsif":
				elsifNode := p.parseElsif()
				elsifNode.Parent = parent
				parent.Children = append(parent.Children, elsifNode)
				continue
			case "else":
				elseNode := p.parseElse()
				elseNode.Parent = parent
				parent.Children = append(parent.Children, elseNode)
				continue
			case "class", "module", "def", "if", "unless", "case", "while", "until", "for", "begin", "do":
				depth++
			case "end":
				depth--
				if depth == 0 {
					p.pos++ // consume 'end'
					return
				}
			}
		}

		node := p.parseStatement()
		if node != nil {
			node.Parent = parent
			parent.Children = append(parent.Children, node)
		} else if p.pos < len(p.tokens) && p.tokens[p.pos].Type == TokenKeyword && p.tokens[p.pos].Literal == "end" {
			p.pos++
			depth--
			if depth == 0 {
				return
			}
		}
	}
}

// parseBodyWithWhen parses body with when/else support
func (p *RubyParser) parseBodyWithWhen(parent *Node, endKeyword string) {
	depth := 1

	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenEOF {
		p.skipWhitespaceAndNewlines()

		if p.pos >= len(p.tokens) || p.tokens[p.pos].Type == TokenEOF {
			break
		}

		if p.tokens[p.pos].Type == TokenKeyword {
			switch p.tokens[p.pos].Literal {
			case "when":
				whenNode := p.parseWhen()
				whenNode.Parent = parent
				parent.Children = append(parent.Children, whenNode)
				continue
			case "else":
				elseNode := p.parseElse()
				elseNode.Parent = parent
				parent.Children = append(parent.Children, elseNode)
				continue
			case "class", "module", "def", "if", "unless", "case", "while", "until", "for", "begin", "do":
				depth++
			case "end":
				depth--
				if depth == 0 {
					p.pos++ // consume 'end'
					return
				}
			}
		}

		node := p.parseStatement()
		if node != nil {
			node.Parent = parent
			parent.Children = append(parent.Children, node)
		} else if p.pos < len(p.tokens) && p.tokens[p.pos].Type == TokenKeyword && p.tokens[p.pos].Literal == "end" {
			p.pos++
			depth--
			if depth == 0 {
				return
			}
		}
	}
}

// parseBodyWithRescue parses body with rescue/else/ensure support
func (p *RubyParser) parseBodyWithRescue(parent *Node, endKeyword string) {
	depth := 1

	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenEOF {
		p.skipWhitespaceAndNewlines()

		if p.pos >= len(p.tokens) || p.tokens[p.pos].Type == TokenEOF {
			break
		}

		if p.tokens[p.pos].Type == TokenKeyword {
			switch p.tokens[p.pos].Literal {
			case "rescue":
				rescueNode := p.parseRescue()
				rescueNode.Parent = parent
				parent.Children = append(parent.Children, rescueNode)
				continue
			case "else":
				elseNode := p.parseElse()
				elseNode.Parent = parent
				parent.Children = append(parent.Children, elseNode)
				continue
			case "ensure":
				ensureNode := p.parseEnsure()
				ensureNode.Parent = parent
				parent.Children = append(parent.Children, ensureNode)
				continue
			case "class", "module", "def", "if", "unless", "case", "while", "until", "for", "begin", "do":
				depth++
			case "end":
				depth--
				if depth == 0 {
					p.pos++ // consume 'end'
					return
				}
			}
		}

		node := p.parseStatement()
		if node != nil {
			node.Parent = parent
			parent.Children = append(parent.Children, node)
		} else if p.pos < len(p.tokens) && p.tokens[p.pos].Type == TokenKeyword && p.tokens[p.pos].Literal == "end" {
			p.pos++
			depth--
			if depth == 0 {
				return
			}
		}
	}
}

// parseElsif parses an elsif clause
func (p *RubyParser) parseElsif() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	p.pos++ // consume 'elsif'

	elsifNode := &Node{
		Type: NodeElsif,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol},
		},
	}

	// Skip condition
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenNewline && p.tokens[p.pos].Type != TokenEOF &&
		!(p.tokens[p.pos].Type == TokenKeyword && p.tokens[p.pos].Literal == "then") {
		p.pos++
	}
	if p.pos < len(p.tokens) && p.tokens[p.pos].Literal == "then" {
		p.pos++
	}

	// Parse body until elsif, else, or end
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenEOF {
		p.skipWhitespaceAndNewlines()

		if p.pos >= len(p.tokens) {
			break
		}

		if p.tokens[p.pos].Type == TokenKeyword {
			switch p.tokens[p.pos].Literal {
			case "elsif", "else", "end":
				// Find end of elsif body
				if len(elsifNode.Children) > 0 {
					lastChild := elsifNode.Children[len(elsifNode.Children)-1]
					elsifNode.Range.End = Position{Line: lastChild.Range.End.Line + 1, Character: 0}
				} else {
					elsifNode.Range.End = Position{Line: startLine + 1, Character: 0}
				}
				return elsifNode
			case "class", "module", "def", "if", "unless", "case", "while", "until", "for", "begin":
				// Nested structure
				node := p.parseStatement()
				if node != nil {
					node.Parent = elsifNode
					elsifNode.Children = append(elsifNode.Children, node)
				}
				continue
			}
		}

		node := p.parseStatement()
		if node != nil {
			node.Parent = elsifNode
			elsifNode.Children = append(elsifNode.Children, node)
		}
	}

	if len(elsifNode.Children) > 0 {
		lastChild := elsifNode.Children[len(elsifNode.Children)-1]
		elsifNode.Range.End = Position{Line: lastChild.Range.End.Line + 1, Character: 0}
	}

	return elsifNode
}

// parseElse parses an else clause
func (p *RubyParser) parseElse() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	p.pos++ // consume 'else'

	elseNode := &Node{
		Type: NodeElse,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol + 4},
		},
	}

	// Parse body until end
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenEOF {
		p.skipWhitespaceAndNewlines()

		if p.pos >= len(p.tokens) {
			break
		}

		if p.tokens[p.pos].Type == TokenKeyword && p.tokens[p.pos].Literal == "end" {
			if len(elseNode.Children) > 0 {
				lastChild := elseNode.Children[len(elseNode.Children)-1]
				elseNode.Range.End = Position{Line: lastChild.Range.End.Line + 1, Character: 0}
			}
			return elseNode
		}

		node := p.parseStatement()
		if node != nil {
			node.Parent = elseNode
			elseNode.Children = append(elseNode.Children, node)
		}
	}

	if len(elseNode.Children) > 0 {
		lastChild := elseNode.Children[len(elseNode.Children)-1]
		elseNode.Range.End = Position{Line: lastChild.Range.End.Line + 1, Character: 0}
	}

	return elseNode
}

// parseWhen parses a when clause
func (p *RubyParser) parseWhen() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	p.pos++ // consume 'when'

	whenNode := &Node{
		Type: NodeWhen,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol},
		},
	}

	// Skip condition
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenNewline && p.tokens[p.pos].Type != TokenEOF &&
		!(p.tokens[p.pos].Type == TokenKeyword && p.tokens[p.pos].Literal == "then") {
		p.pos++
	}
	if p.pos < len(p.tokens) && p.tokens[p.pos].Literal == "then" {
		p.pos++
	}

	// Parse body until when, else, or end
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenEOF {
		p.skipWhitespaceAndNewlines()

		if p.pos >= len(p.tokens) {
			break
		}

		if p.tokens[p.pos].Type == TokenKeyword {
			switch p.tokens[p.pos].Literal {
			case "when", "else", "end":
				if len(whenNode.Children) > 0 {
					lastChild := whenNode.Children[len(whenNode.Children)-1]
					whenNode.Range.End = Position{Line: lastChild.Range.End.Line + 1, Character: 0}
				}
				return whenNode
			}
		}

		node := p.parseStatement()
		if node != nil {
			node.Parent = whenNode
			whenNode.Children = append(whenNode.Children, node)
		}
	}

	if len(whenNode.Children) > 0 {
		lastChild := whenNode.Children[len(whenNode.Children)-1]
		whenNode.Range.End = Position{Line: lastChild.Range.End.Line + 1, Character: 0}
	}

	return whenNode
}

// parseRescue parses a rescue clause
func (p *RubyParser) parseRescue() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	p.pos++ // consume 'rescue'

	rescueNode := &Node{
		Type: NodeRescue,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol + 6},
		},
	}

	// Skip exception class and variable
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenNewline && p.tokens[p.pos].Type != TokenEOF &&
		!(p.tokens[p.pos].Type == TokenKeyword && p.tokens[p.pos].Literal == "then") {
		p.pos++
	}
	if p.pos < len(p.tokens) && p.tokens[p.pos].Literal == "then" {
		p.pos++
	}

	// Parse body until rescue, else, ensure, or end
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenEOF {
		p.skipWhitespaceAndNewlines()

		if p.pos >= len(p.tokens) {
			break
		}

		if p.tokens[p.pos].Type == TokenKeyword {
			switch p.tokens[p.pos].Literal {
			case "rescue", "else", "ensure", "end":
				if len(rescueNode.Children) > 0 {
					lastChild := rescueNode.Children[len(rescueNode.Children)-1]
					rescueNode.Range.End = Position{Line: lastChild.Range.End.Line + 1, Character: 0}
				}
				return rescueNode
			}
		}

		node := p.parseStatement()
		if node != nil {
			node.Parent = rescueNode
			rescueNode.Children = append(rescueNode.Children, node)
		}
	}

	if len(rescueNode.Children) > 0 {
		lastChild := rescueNode.Children[len(rescueNode.Children)-1]
		rescueNode.Range.End = Position{Line: lastChild.Range.End.Line + 1, Character: 0}
	}

	return rescueNode
}

// parseEnsure parses an ensure clause
func (p *RubyParser) parseEnsure() *Node {
	startTok := p.tokens[p.pos]
	startLine := startTok.Line
	startCol := startTok.Column
	p.pos++ // consume 'ensure'

	ensureNode := &Node{
		Type: NodeEnsure,
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: startCol + 6},
		},
	}

	// Parse body until end
	for p.pos < len(p.tokens) && p.tokens[p.pos].Type != TokenEOF {
		p.skipWhitespaceAndNewlines()

		if p.pos >= len(p.tokens) {
			break
		}

		if p.tokens[p.pos].Type == TokenKeyword && p.tokens[p.pos].Literal == "end" {
			if len(ensureNode.Children) > 0 {
				lastChild := ensureNode.Children[len(ensureNode.Children)-1]
				ensureNode.Range.End = Position{Line: lastChild.Range.End.Line + 1, Character: 0}
			}
			return ensureNode
		}

		node := p.parseStatement()
		if node != nil {
			node.Parent = ensureNode
			ensureNode.Children = append(ensureNode.Children, node)
		}
	}

	if len(ensureNode.Children) > 0 {
		lastChild := ensureNode.Children[len(ensureNode.Children)-1]
		ensureNode.Range.End = Position{Line: lastChild.Range.End.Line + 1, Character: 0}
	}

	return ensureNode
}

// Helper methods

func (p *RubyParser) skipWhitespaceAndNewlines() {
	for p.pos < len(p.tokens) && (p.tokens[p.pos].Type == TokenWhitespace || p.tokens[p.pos].Type == TokenNewline) {
		p.pos++
	}
}

func (p *RubyParser) peekToken() Token {
	if p.pos+1 < len(p.tokens) {
		return p.tokens[p.pos+1]
	}
	return Token{Type: TokenEOF}
}

func (p *RubyParser) skipParens() {
	if p.pos < len(p.tokens) && p.tokens[p.pos].Literal == "(" {
		p.pos++
		depth := 1
		for p.pos < len(p.tokens) && depth > 0 {
			if p.tokens[p.pos].Literal == "(" {
				depth++
			} else if p.tokens[p.pos].Literal == ")" {
				depth--
			}
			p.pos++
		}
	}
}

func isKeyword(literal string) bool {
	keywords := []string{
		"class", "module", "def", "if", "unless", "elsif", "else", "case", "when",
		"while", "until", "for", "begin", "end", "do", "return", "next", "break",
		"yield", "redo", "retry", "super", "self", "nil", "true", "false", "and",
		"or", "not", "then", "ensure", "rescue", "alias", "undef", "defined?",
		"__LINE__", "__FILE__", "__ENCODING__", "BEGIN", "END",
	}
	for _, kw := range keywords {
		if kw == literal {
			return true
		}
	}
	return false
}

func isOperator(literal string) bool {
	operators := []string{
		"+", "-", "*", "/", "%", "**", "==", "!=", ">", "<", ">=", "<=", "<=>",
		"===", "=~", "!~", "&&", "||", "!", "&", "|", "^", "~", "<<", ">>",
		"=", "+=", "-=", "*=", "/=", "%=", "**=", "&=", "|=", "^=", "<<=", ">>=",
		"&&=", "||=", "..", "...", "::", "=>", "?", ":",
	}
	for _, op := range operators {
		if op == literal {
			return true
		}
	}
	return false
}

func isRailsMacro(literal string) bool {
	macros := []string{
		"scope", "belongs_to", "has_many", "has_one", "has_and_belongs_to_many",
		"validates", "validates_presence_of", "validates_uniqueness_of",
		"validates_length_of", "validates_format_of", "validates_numericality_of",
		"validates_inclusion_of", "validates_exclusion_of", "validates_associated",
		"before_action", "after_action", "around_action", "before_filter",
		"after_filter", "around_filter", "skip_before_action", "skip_after_action",
		"has_secure_password", "has_secure_token", "serialize", "store",
		"store_accessor", "delegate", "enum", "default_scope", "validate",
	}
	for _, macro := range macros {
		if macro == literal {
			return true
		}
	}
	return false
}

// GetNodeAtPosition finds the deepest node at a given position
func GetNodeAtPosition(node *Node, pos Position) *Node {
	if !node.Range.Contains(pos) {
		return nil
	}

	for _, child := range node.Children {
		if found := GetNodeAtPosition(child, pos); found != nil {
			return found
		}
	}

	return node
}

// FindNodesByType finds all nodes of a given type in the AST
func FindNodesByType(node *Node, nodeType NodeType) []*Node {
	var results []*Node

	if node.Type == nodeType {
		results = append(results, node)
	}

	for _, child := range node.Children {
		results = append(results, FindNodesByType(child, nodeType)...)
	}

	return results
}

// String returns a string representation of the AST node for debugging
func (n *Node) String() string {
	return fmt.Sprintf("%s(%s) [%d:%d-%d:%d]", n.Type, n.Name, n.Range.Start.Line, n.Range.Start.Character, n.Range.End.Line, n.Range.End.Character)
}
