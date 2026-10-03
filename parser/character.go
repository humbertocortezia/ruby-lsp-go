package parser

import (
	"strings"
	"unicode/utf8"
)

func (p *RubyParser) scanCharacter(start int) (int, error) {
	end, ok := characterEnd(p.source, start+1)
	if !ok {
		line, column := sourcePosition(p.source, start)
		return end, &IncompleteLiteralError{Kind: "character", Line: line, Column: column}
	}
	return end, nil
}

// Character escapes can contain punctuation that is meaningful to Ruby's
// statement lexer (notably quotes and braces). Consume the complete escape,
// including chained control/meta modifiers, as one literal token.
func characterEnd(source string, start int) (int, bool) {
	if start >= len(source) || source[start] == '\n' || source[start] == '\r' {
		return start, false
	}
	if source[start] != '\\' {
		_, width := utf8.DecodeRuneInString(source[start:])
		return start + width, true
	}
	i := start + 1
	if i == len(source) {
		return i, false
	}
	switch source[i] {
	case 'C', 'M':
		if i+1 >= len(source) || source[i+1] != '-' {
			return i + 1, true
		}
		return characterEnd(source, i+2)
	case 'c':
		return characterEnd(source, i+1)
	case 'u':
		i++
		if i < len(source) && source[i] == '{' {
			close := strings.IndexByte(source[i+1:], '}')
			if close < 0 || strings.ContainsAny(source[i+1:i+1+close], "\r\n") {
				return i, false
			}
			return i + close + 2, true
		}
		return hexEscapeEnd(source, i, 4)
	case 'x':
		return hexEscapeEnd(source, i+1, 2)
	default:
		if source[i] >= '0' && source[i] <= '7' {
			for end := i; end < len(source) && end < i+3; end++ {
				if source[end] < '0' || source[end] > '7' {
					return end, true
				}
				if end+1 == len(source) || end == i+2 {
					return end + 1, true
				}
			}
		}
		_, width := utf8.DecodeRuneInString(source[i:])
		return i + width, source[i] != '\n' && source[i] != '\r'
	}
}

func hexEscapeEnd(source string, start, count int) (int, bool) {
	end := start
	for end < len(source) && end < start+count && strings.ContainsRune("0123456789abcdefABCDEF", rune(source[end])) {
		end++
	}
	return end, end > start
}
