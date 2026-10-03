package parser

import (
	"strings"
	"unicode"
)

// Longest first: Ruby's operator method names are indivisible in method-name
// and symbol contexts, even when the same punctuation opens a literal elsewhere.
var operatorMethodNames = []string{"[]=", "<=>", "===", "**", "<<", ">>", "<=", ">=", "==", "!=", "=~", "!~", "+@", "-@", "!@", "~@", "[]", "+", "-", "*", "/", "%", "&", "|", "^", "~", "<", ">", "!", "`"}

func operatorMethodEnd(source string, start int) int {
	for _, operator := range operatorMethodNames {
		if strings.HasPrefix(source[start:], operator) {
			return start + len(operator)
		}
	}
	return start
}

func bareSymbolEnd(source string, start int) int {
	i := start + 1
	if i >= len(source) || source[i] == ':' || start > 0 && source[start-1] == ':' {
		return start
	}
	if unicode.IsLetter(rune(source[i])) || source[i] == '_' {
		for i < len(source) && (unicode.IsLetter(rune(source[i])) || unicode.IsDigit(rune(source[i])) || source[i] == '_') {
			i++
		}
		if i < len(source) && strings.ContainsRune("!?=", rune(source[i])) {
			i++
		}
		return i
	}
	// Operator method names are symbols, too. Their slash/backtick/question
	// mark must not open a regexp, command string or character literal.
	if end := operatorMethodEnd(source, i); end > i {
		return end
	}
	return start
}

func sourcePosition(source string, offset int) (line, column int) {
	prefix := source[:offset]
	line = strings.Count(prefix, "\n")
	column = offset - strings.LastIndex(prefix, "\n") - 1
	return line, column
}

// globalVariableEnd consumes the whole name, including Ruby's punctuation
// globals/backreferences ($', $", $/, ...) and command-line switches ($-w).
// Their punctuation must never be scanned again as a literal delimiter.
func globalVariableEnd(source string, start int) int {
	end := start + 1
	if end == len(source) {
		return end
	}
	if strings.ContainsRune("~*$?!@/\\;,.=:<>\"&`'+", rune(source[end])) {
		return end + 1
	}
	if source[end] == '-' {
		end++
		if end < len(source) && (unicode.IsLetter(rune(source[end])) || source[end] == '_') {
			end++
		}
		return end
	}
	for end < len(source) && (unicode.IsLetter(rune(source[end])) || unicode.IsDigit(rune(source[end])) || source[end] == '_') {
		end++
	}
	return end
}

// percentLiteralAt recognizes Ruby percent-literal prefixes and leaves their
// delimiter matching to the same scanner as quotes/regexes. This matters in
// interpolations: braces and quotes in %q/%Q are content, not Ruby code.
func percentLiteralAt(source string, start int) (opening int, kind TokenType, ok bool) {
	opening, kind = start+1, TokenString
	if opening >= len(source) {
		return opening, kind, false
	}
	if strings.ContainsRune("qQwWiIrxs", rune(source[opening])) {
		if source[opening] == 'r' {
			kind = TokenRegex
		} else if source[opening] == 's' {
			kind = TokenSymbol
		}
		opening++
	}
	if opening >= len(source) {
		return opening, kind, false
	}
	delimiter := rune(source[opening])
	return opening, kind, !unicode.IsLetter(delimiter) && !unicode.IsDigit(delimiter) && !unicode.IsSpace(delimiter)
}

func (p *RubyParser) previousToken(before int) (Token, int) {
	for i := before - 1; i >= 0; i-- {
		if p.tokens[i].Type != TokenWhitespace {
			return p.tokens[i], i
		}
	}
	return Token{}, -1
}

// Newlines/comments can appear while a required name is pending (after def,
// a receiver selector, alias's first operand or undef's comma). Semicolons are
// statement boundaries and must never preserve that context.
func (p *RubyParser) previousMethodToken(before int) (Token, int) {
	for i := before - 1; i >= 0; i-- {
		token := p.tokens[i]
		if token.Type != TokenWhitespace && token.Type != TokenComment && !(token.Type == TokenNewline && token.Literal == "\n") {
			return token, i
		}
	}
	return Token{}, -1
}

func isMethodNameToken(token Token) bool {
	switch token.Type {
	case TokenIdentifier, TokenConstant, TokenOperator, TokenSymbol, TokenKeyword:
		return true
	default:
		return false
	}
}

// methodNameContext models the distinction Ruby makes between a name and an
// expression. The second result allows a setter suffix in declarations only:
// obj.name=1 must still tokenize as a call name followed by assignment.
func (p *RubyParser) methodNameContext() (name, declaration bool) {
	previous, i := p.previousMethodToken(len(p.tokens))
	if previous.Type == TokenKeyword {
		switch previous.Literal {
		case "def", "alias", "undef":
			return true, true
		}
	}
	if previous.Literal == "." || previous.Literal == "::" || previous.Literal == "&." {
		last, receiver := p.previousMethodToken(i)
		if last.Type == TokenPunctuation && last.Literal == ")" {
			depth := 1
			for receiver--; receiver >= 0; receiver-- {
				token := p.tokens[receiver]
				if token.Type != TokenPunctuation {
					continue
				}
				if token.Literal == ")" {
					depth++
				} else if token.Literal == "(" {
					depth--
					if depth == 0 {
						break
					}
				}
			}
		}
		before, _ := p.previousMethodToken(receiver)
		return true, before.Type == TokenKeyword && before.Literal == "def"
	}
	// alias has exactly two names, which may themselves be operator symbols.
	before, _ := p.previousMethodToken(i)
	if isMethodNameToken(previous) && before.Type == TokenKeyword && before.Literal == "alias" {
		return true, true
	}
	// undef has a comma-separated name list. Walk only the list, never arbitrary
	// earlier expressions (e.g. an array or call containing a comma).
	for previous.Literal == "," {
		operand, operandIndex := p.previousMethodToken(i)
		if !isMethodNameToken(operand) {
			break
		}
		previous, i = p.previousMethodToken(operandIndex)
		if previous.Type == TokenKeyword && previous.Literal == "undef" {
			return true, true
		}
	}
	return false, false
}

func (p *RubyParser) methodNameExpected() bool {
	_, declaration := p.methodNameContext()
	return declaration
}

// regexExpected distinguishes a regexp from division using the expression
// context, ignoring horizontal whitespace. After an operator or an opening
// delimiter a regexp can begin with whitespace/newlines. After a value, '/' is
// division; a spaced command argument such as `match /pattern/` is a regexp.
func (p *RubyParser) regexExpected(offset int) bool {
	var previous *Token
	previousIndex := -1
	spaced := false
	for i := len(p.tokens) - 1; i >= 0; i-- {
		token := &p.tokens[i]
		if token.Type == TokenWhitespace {
			spaced = true
			continue
		}
		previous = token
		previousIndex = i
		break
	}
	if previous == nil || previous.Type == TokenNewline || previous.Type == TokenComment {
		return true
	}
	switch previous.Type {
	case TokenOperator:
		return previous.Literal != "::"
	case TokenPunctuation:
		return strings.Contains("([{,;", previous.Literal)
	case TokenKeyword:
		switch previous.Literal {
		case "self", "nil", "true", "false", "end", "__FILE__", "__LINE__", "__ENCODING__", "def", "class", "module":
			return false
		default:
			return true
		}
	case TokenIdentifier:
		// An assigned local or a parameter is a value, not a command name.
		// The distinction matters for division, shifts, ternaries and modulo.
		if !spaced || offset+1 >= len(p.source) || unicode.IsSpace(rune(p.source[offset+1])) || p.source[offset+1] == '=' {
			return false
		}
		beforeReceiver, _ := p.previousToken(previousIndex)
		if beforeReceiver.Literal == "." || beforeReceiver.Literal == "::" || beforeReceiver.Literal == "&." {
			return true
		}
		return !p.visibleLocals()[previous.Literal]
	default:
		return false
	}
}

func (p *RubyParser) interpolationParser() *RubyParser {
	// Lexing is synchronous: the parent's token stream does not advance while
	// its interpolation is scanned. Most interpolations need no binding lookup.
	return &RubyParser{source: p.source, heredocEnds: p.heredocEnds, outerLexer: p}
}

type localFrame struct {
	names      map[string]bool
	closing    string
	loopHeader bool
}

// Derive the bindings visible at this point from already emitted code tokens.
// Literal bodies/comments never enter this stream. Class/module/method scopes
// isolate bindings, conditionals share them, and blocks inherit surrounding
// bindings. This is lexical context for ambiguous literal openers, not a second
// Ruby AST parser or a retry after a literal failed to close.
func (p *RubyParser) visibleLocals() map[string]bool {
	if p.localsCache != nil && p.localsTokenCount == len(p.tokens) {
		return p.localsCache
	}
	names := make(map[string]bool)
	if p.outerLexer != nil {
		names = copyLocals(p.outerLexer.visibleLocals())
	}
	frames := []localFrame{{names: names}}
	var tokens []Token
	for _, token := range p.tokens {
		if token.Type != TokenWhitespace && token.Type != TokenComment {
			tokens = append(tokens, token)
		}
	}
	blockParams := false
	for i, token := range tokens {
		frame := &frames[len(frames)-1]
		if token.Type == TokenNewline {
			frame.loopHeader = false
			continue
		}
		if token.Type == TokenKeyword {
			switch token.Literal {
			case "def", "class", "module":
				names = make(map[string]bool)
				frames = append(frames, localFrame{names: names, closing: "end"})
				if token.Literal == "def" {
					collectParameters(tokens[i+1:], names)
				}
			case "if", "unless", "while", "until":
				if expressionBegins(tokens, i) {
					frames = append(frames, localFrame{names: frame.names, closing: "end", loopHeader: token.Literal == "while" || token.Literal == "until"})
				}
			case "case", "begin", "for":
				frames = append(frames, localFrame{names: frame.names, closing: "end", loopHeader: token.Literal == "for"})
				if token.Literal == "for" {
					for _, variable := range tokens[i+1:] {
						if variable.Literal == "in" || variable.Type == TokenNewline {
							break
						}
						if variable.Type == TokenIdentifier {
							frame.names[variable.Literal] = true
						}
					}
				}
			case "do":
				if frame.loopHeader {
					frame.loopHeader = false
				} else {
					frames = append(frames, localFrame{names: copyLocals(frame.names), closing: "end"})
				}
			case "end":
				if len(frames) > 1 && frame.closing == "end" {
					frames = frames[:len(frames)-1]
				}
			}
			continue
		}
		if token.Literal == "{" {
			// Hashes share bindings. A brace following a value is a block.
			names := frame.names
			if !expressionBegins(tokens, i) {
				names = copyLocals(names)
			}
			frames = append(frames, localFrame{names: names, closing: "}"})
			continue
		}
		if token.Literal == "}" && len(frames) > 1 && frame.closing == "}" {
			frames = frames[:len(frames)-1]
			continue
		}
		if token.Literal == "|" && (blockParams || i > 0 && (tokens[i-1].Literal == "do" || tokens[i-1].Literal == "{")) {
			blockParams = !blockParams
			continue
		}
		if token.Type != TokenIdentifier {
			continue
		}
		if blockParams {
			frame.names[token.Literal] = true
			continue
		}
		if i > 0 && (tokens[i-1].Literal == "." || tokens[i-1].Literal == "::" || tokens[i-1].Literal == "&.") {
			continue
		}
		if i > 0 && tokens[i-1].Literal == "=>" {
			frame.names[token.Literal] = true
		}
		if i+1 < len(tokens) {
			switch tokens[i+1].Literal {
			case "=", "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "||=", "&&=", "**=", "<<=", ">>=":
				frame.names[token.Literal] = true
			}
		}
	}
	p.localsCache = frames[len(frames)-1].names
	p.localsTokenCount = len(p.tokens)
	return p.localsCache
}

func copyLocals(source map[string]bool) map[string]bool {
	result := make(map[string]bool, len(source))
	for name, value := range source {
		result[name] = value
	}
	return result
}

func expressionBegins(tokens []Token, i int) bool {
	if i == 0 {
		return true
	}
	previous := tokens[i-1]
	if previous.Type == TokenNewline || previous.Type == TokenOperator {
		return true
	}
	if previous.Type == TokenPunctuation && strings.Contains("([{,", previous.Literal) {
		return true
	}
	return previous.Type == TokenKeyword && strings.Contains(" return next break yield then else elsif when rescue in and or not ", " "+previous.Literal+" ")
}

func collectParameters(tokens []Token, names map[string]bool) {
	if len(tokens) == 0 {
		return
	}
	// First token is the method name (or receiver in a singleton definition).
	start := 1
	if len(tokens) > 2 && (tokens[1].Literal == "." || tokens[1].Literal == "::") {
		start = 3
	}
	expectParameter, depth := true, 0
	for _, token := range tokens[start:] {
		if token.Type == TokenNewline {
			break
		}
		if token.Literal == "(" {
			depth++
			continue
		}
		if token.Literal == ")" {
			depth--
			if depth <= 0 {
				break
			}
			continue
		}
		if token.Literal == "," && depth <= 1 {
			expectParameter = true
			continue
		}
		if expectParameter && token.Type == TokenIdentifier {
			names[token.Literal] = true
			expectParameter = false
		}
	}
}
