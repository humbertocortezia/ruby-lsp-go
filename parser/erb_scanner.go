package parser

import (
	"strings"
)

// ERBRegion represents a region in an ERB file.
type ERBRegion struct {
	Kind       string // "ruby" or "host"
	StartByte  int
	EndByte    int
	StartLine  int
	StartCol   int
	EndLine    int
	EndCol     int
}

// ERBScanner separates Ruby code from host language (HTML) in ERB files.
type ERBScanner struct {
	source      string
	rubyContent strings.Builder
	hostContent strings.Builder
	regions     []ERBRegion
	rubyMap     []int // maps ruby byte offset -> original byte offset
}

// NewERBScanner creates a scanner for ERB source.
func NewERBScanner(source string) *ERBScanner {
	s := &ERBScanner{source: source}
	s.scan()
	return s
}

func (s *ERBScanner) scan() {
	i := 0
	rubyOffset := 0
	for i < len(s.source) {
		if i+1 < len(s.source) && s.source[i] == '<' && s.source[i+1] == '%' {
			// Determine ERB tag type
			tagStart := i
			i += 2
			isOutput := false
			if i < len(s.source) && s.source[i] == '=' {
				isOutput = true
				i++
			}

			// Find closing %>
			closeIdx := strings.Index(s.source[i:], "%>")
			if closeIdx == -1 {
				break
			}
			rubyCode := s.source[i : i+closeIdx]
			i += closeIdx + 2

			regionStart := rubyOffset
			if !isOutput {
				s.rubyContent.WriteString(rubyCode)
				for range rubyCode {
					s.rubyMap = append(s.rubyMap, tagStart)
				}
				rubyOffset += len(rubyCode)
			} else {
				// Output tags are Ruby expressions
				s.rubyContent.WriteString(rubyCode)
				for range rubyCode {
					s.rubyMap = append(s.rubyMap, tagStart)
				}
				rubyOffset += len(rubyCode)
			}

			s.regions = append(s.regions, ERBRegion{
				Kind:      "ruby",
				StartByte: tagStart,
				EndByte:   i,
			})
			_ = regionStart
		} else {
			// Host language (HTML)
			hostStart := i
			for i < len(s.source) && !(i+1 < len(s.source) && s.source[i] == '<' && s.source[i+1] == '%') {
				s.hostContent.WriteByte(s.source[i])
				i++
			}
			if i > hostStart {
				s.regions = append(s.regions, ERBRegion{
					Kind:      "host",
					StartByte: hostStart,
					EndByte:   i,
				})
			}
		}
	}
}

// RubyContent returns concatenated Ruby code from ERB tags.
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
	rubyOffset := lineColToOffset(s.rubyContent.String(), rubyLine, rubyCol)
	if rubyOffset < len(s.rubyMap) {
		erbOffset := s.rubyMap[rubyOffset]
		return offsetToLineCol(s.source, erbOffset)
	}
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
