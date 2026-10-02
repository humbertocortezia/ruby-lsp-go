package parser

import (
	"strings"
	"unicode"
)

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

func (p *RubyParser) methodNameExpected() bool {
	previous, i := p.previousToken(len(p.tokens))
	if previous.Type == TokenKeyword && previous.Literal == "def" {
		return true
	}
	if previous.Literal != "." {
		return false
	}
	_, i = p.previousToken(i) // receiver
	previous, _ = p.previousToken(i)
	return previous.Type == TokenKeyword && previous.Literal == "def"
}

// regexExpected distinguishes a regexp from division using the expression
// context, ignoring horizontal whitespace. After an operator or an opening
// delimiter a regexp can begin with whitespace/newlines. After a value, '/' is
// division; a spaced command argument such as `match /pattern/` is a regexp.
func (p *RubyParser) regexExpected(offset int) bool {
	var previous *Token
	spaced := false
	for i := len(p.tokens) - 1; i >= 0; i-- {
		token := &p.tokens[i]
		if token.Type == TokenWhitespace {
			spaced = true
			continue
		}
		previous = token
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
		return spaced && offset+1 < len(p.source) && !unicode.IsSpace(rune(p.source[offset+1])) && p.source[offset+1] != '='
	default:
		return false
	}
}
