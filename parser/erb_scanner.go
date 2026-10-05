package parser

import (
	"strings"
)

// ERBRegion represents a region in an ERB file.
type ERBRegion struct {
	Kind      string // "ruby" or "host"
	StartByte int
	EndByte   int
	StartLine int
	StartCol  int
	EndLine   int
	EndCol    int
}

// ERBScanner separates Ruby code from host language (HTML) in ERB files.
type ERBScanner struct {
	source      string
	rubyContent strings.Builder
	hostContent strings.Builder
	regions     []ERBRegion
	err         error
}

// NewERBScanner creates a scanner for ERB source.
func NewERBScanner(source string) *ERBScanner {
	s := &ERBScanner{source: source}
	s.scan()
	return s
}

func (s *ERBScanner) scan() {
	// Keep every byte offset and newline stable. Only Ruby tag contents are
	// copied; host text, ERB comments and escaped tags become whitespace.
	ruby := []byte(s.source)
	for i, ch := range ruby {
		if ch != '\n' && ch != '\r' {
			ruby[i] = ' '
		}
	}
	hostStart := 0
	for i := 0; i < len(s.source); {
		if !strings.HasPrefix(s.source[i:], "<%") {
			i++
			continue
		}
		if strings.HasPrefix(s.source[i:], "<%%") {
			i += 3
			continue
		}
		tagStart := i
		codeStart := i + 2
		comment := false
		if codeStart < len(s.source) {
			switch s.source[codeStart] {
			case '#':
				comment = true
				codeStart++
			case '=', '-':
				codeStart++
			}
		}
		closeOffset := strings.Index(s.source[codeStart:], "%>")
		if closeOffset < 0 {
			line, col := sourcePosition(s.source, tagStart)
			s.err = &IncompleteLiteralError{Kind: "ERB tag", Line: line, Column: col}
			break
		}
		codeEnd := codeStart + closeOffset
		i = codeEnd + 2
		if codeEnd > codeStart && s.source[codeEnd-1] == '-' {
			codeEnd--
		}
		if comment {
			continue
		}
		if tagStart > hostStart {
			s.addRegion("host", hostStart, tagStart)
			s.hostContent.WriteString(s.source[hostStart:tagStart])
		}
		s.addRegion("ruby", tagStart, i)
		copy(ruby[codeStart:codeEnd], s.source[codeStart:codeEnd])
		// Adjacent tags are separate statements, not one concatenated identifier.
		ruby[i-1] = ';'
		hostStart = i
	}
	if hostStart < len(s.source) {
		s.addRegion("host", hostStart, len(s.source))
		s.hostContent.WriteString(s.source[hostStart:])
	}
	s.rubyContent.Write(ruby)
}

func (s *ERBScanner) addRegion(kind string, start, end int) {
	startLine, startCol := sourcePosition(s.source, start)
	endLine, endCol := sourcePosition(s.source, end)
	s.regions = append(s.regions, ERBRegion{Kind: kind, StartByte: start, EndByte: end, StartLine: startLine, StartCol: startCol, EndLine: endLine, EndCol: endCol})
}

// Err reports malformed template delimiters. Callers must not publish a partial
// Ruby AST when extraction fails.
func (s *ERBScanner) Err() error { return s.err }

// RubyContent returns Ruby code padded to the template's original byte positions.
func (s *ERBScanner) RubyContent() string {
	return s.rubyContent.String()
}

// HostContent returns the host language content.
func (s *ERBScanner) HostContent() string {
	return s.hostContent.String()
}

// Regions returns all scanned regions.
func (s *ERBScanner) Regions() []ERBRegion {
	return s.regions
}

// InsideHostLanguage reports whether the byte offset is in host language.
func (s *ERBScanner) InsideHostLanguage(byteOffset int) bool {
	for _, r := range s.regions {
		if r.Kind == "host" && byteOffset >= r.StartByte && byteOffset < r.EndByte {
			return true
		}
	}
	return false
}

// InsideHostLanguageAtLineCol checks if LSP position is in host language.
func (s *ERBScanner) InsideHostLanguageAtLineCol(line, col int) bool {
	offset := lineColToOffset(s.source, line, col)
	return s.InsideHostLanguage(offset)
}

// MapRubyToERB maps a position in ruby content back to ERB source position.
func (s *ERBScanner) MapRubyToERB(rubyLine, rubyCol int) (line, col int) {
	return rubyLine, rubyCol
}

func lineColToOffset(source string, line, col int) int {
	lines := strings.Split(source, "\n")
	offset := 0
	for i := 0; i < line && i < len(lines); i++ {
		offset += len(lines[i]) + 1
	}
	if line < len(lines) {
		lineLen := len([]rune(lines[line]))
		if col > lineLen {
			col = lineLen
		}
		runes := 0
		for _, r := range lines[line] {
			if runes >= col {
				break
			}
			offset += len(string(r))
			runes++
		}
	}
	return offset
}

func offsetToLineCol(source string, offset int) (line, col int) {
	if offset < 0 {
		return 0, 0
	}
	if offset > len(source) {
		offset = len(source)
	}
	line = 0
	col = 0
	for i := 0; i < offset && i < len(source); i++ {
		if source[i] == '\n' {
			line++
			col = 0
		} else {
			col++
		}
	}
	return line, col
}

// DelegateRequestError is the LSP error code for ERB delegation.
const DelegateRequestError = -32000
