package parser

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

type heredocOpening struct {
	start, end             int
	delimiter              string
	indented, interpolates bool
}

func heredocAt(source string, start int) (heredocOpening, bool) {
	h := heredocOpening{start: start, interpolates: true}
	i := start + 2
	if i < len(source) && (source[i] == '-' || source[i] == '~') {
		h.indented = true
		i++
	}
	if i >= len(source) {
		return h, false
	}
	if quote := source[i]; quote == '\'' || quote == '"' || quote == '`' {
		h.interpolates = quote != '\''
		begin := i + 1
		i = begin
		for i < len(source) && source[i] != quote && source[i] != '\n' && source[i] != '\r' {
			i++
		}
		if i == len(source) || source[i] != quote {
			return h, false
		}
		h.delimiter, h.end = source[begin:i], i+1
		return h, true
	}
	begin := i
	r, width := utf8.DecodeRuneInString(source[i:])
	if !unicode.IsLetter(r) && r != '_' {
		return h, false
	}
	i += width
	for i < len(source) {
		r, width = utf8.DecodeRuneInString(source[i:])
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
			break
		}
		i += width
	}
	h.delimiter, h.end = source[begin:i], i
	return h, true
}

// Heredoc bodies are physically on later lines, but their tokens occupy the
// opener's expression. Scan the body now and record where the main lexer must
// resume after the opener line. Sharing these offsets also handles multiple
// heredocs on one line and heredocs inside interpolation, without moving or
// rewriting source bytes (and therefore without changing declaration positions).
func (p *RubyParser) scanHeredoc(h heredocOpening) (bodyStart, end int, err error) {
	newline := strings.IndexByte(p.source[h.end:], '\n')
	line, column := sourcePosition(p.source, h.start)
	if newline < 0 {
		return 0, h.end, &IncompleteLiteralError{Kind: "heredoc", Line: line, Column: column}
	}
	firstBody := h.end + newline + 1
	bodyStart = firstBody
	if previousEnd, ok := p.heredocEnds[firstBody]; ok {
		bodyStart = previousEnd
	}
	for end = bodyStart; end < len(p.source); {
		if next, ok := p.heredocEnds[end]; ok {
			end = next
			continue
		}
		if end == bodyStart || p.source[end-1] == '\n' {
			lineEnd := strings.IndexByte(p.source[end:], '\n')
			if lineEnd < 0 {
				lineEnd = len(p.source)
			} else {
				lineEnd += end
			}
			text := strings.TrimSuffix(p.source[end:lineEnd], "\r")
			if h.indented {
				text = strings.TrimLeft(text, " \t")
			}
			if text == h.delimiter {
				end = lineEnd
				if end < len(p.source) {
					end++
				}
				p.heredocEnds[firstBody] = end
				return bodyStart, end, nil
			}
		}
		if h.interpolates {
			if p.source[end] == '\\' && end+1 < len(p.source) {
				end += 2
				continue
			}
			if p.source[end] == '#' && end+1 < len(p.source) {
				if p.source[end+1] == '{' {
					nested := p.interpolationParser()
					end, err = nested.tokenizeFrom(end+2, true)
					if err != nil {
						return bodyStart, end, err
					}
					continue
				}
				if p.source[end+1] == '$' {
					end = globalVariableEnd(p.source, end+1)
					continue
				}
			}
		}
		end++
		if next, ok := p.heredocEnds[end]; ok {
			end = next
		}
	}
	return bodyStart, end, &IncompleteLiteralError{Kind: "heredoc", Line: line, Column: column}
}

func lineMarkerAt(source string, start int, marker string) bool {
	if !strings.HasPrefix(source[start:], marker) {
		return false
	}
	end := start + len(marker)
	return end == len(source) || strings.ContainsRune(" \t\r\n\v\f", rune(source[end]))
}

func dataMarkerAt(source string, start int) bool {
	if !strings.HasPrefix(source[start:], "__END__") {
		return false
	}
	end := start + len("__END__")
	return end == len(source) || source[end] == '\n' || strings.HasPrefix(source[end:], "\r\n")
}

func (p *RubyParser) scanBlockComment(start int) (int, error) {
	for end := start; end < len(p.source); {
		next := strings.IndexByte(p.source[end:], '\n')
		if next < 0 {
			break
		}
		end += next + 1
		if lineMarkerAt(p.source, end, "=end") {
			next = strings.IndexByte(p.source[end:], '\n')
			if next < 0 {
				return len(p.source), nil
			}
			return end + next, nil // leave the newline as a statement separator
		}
	}
	line, column := sourcePosition(p.source, start)
	return len(p.source), &IncompleteLiteralError{Kind: "block comment", Line: line, Column: column}
}
