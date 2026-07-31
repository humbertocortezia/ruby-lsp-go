package indexer

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// SymbolType represents the kind of Ruby symbol.
type SymbolType int

const (
	SymbolClass SymbolType = iota
	SymbolModule
	SymbolMethod
	SymbolSingletonMethod
	SymbolConstant
	SymbolScope
	SymbolAssociation
	SymbolAttrAccessor
	// SymbolLocalVariable is indexed for precise go-to-definition, but is not
	// exposed as a document/workspace symbol.
	SymbolLocalVariable
)

// SymbolEntry is a declaration discovered in a Ruby or ERB document.
type SymbolEntry struct {
	Name               string
	FullyQualifiedName string
	Type               SymbolType
	FilePath           string
	Line               int // 1-based source line
	EndLine            int // 1-based source line
	Character          int // zero-based rune column
	EndCharacter       int
	Parent             string
	Visibility         string
	Detail             string
	ScopeStartLine     int // used by local variables
}

// FoldingRange is the small subset of an LSP folding range needed by the
// server. Lines are zero-based, as required by LSP.
type FoldingRange struct {
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Kind      string `json:"kind,omitempty"`
}

type Index struct {
	symbols       map[string][]SymbolEntry
	fileSymbols   map[string][]SymbolEntry
	mutex         sync.RWMutex
	workspaceRoot string
	logger        *log.Logger
	ready         bool
}

var (
	classPattern       = regexp.MustCompile(`^\s*class\s+([A-Z][\w:]*)\s*(?:<\s*([A-Z][\w:]*(?:::[A-Z][\w:]*)*))?`)
	modulePattern      = regexp.MustCompile(`^\s*module\s+([A-Z][\w:]*)`)
	methodPattern      = regexp.MustCompile(`^\s*def\s+(.+)$`)
	constantPattern    = regexp.MustCompile(`(^|[;\s])([A-Z][A-Z0-9_]*)\s*=`)
	scopePattern       = regexp.MustCompile(`^\s*scope\s+:(\w+)`)
	associationPattern = regexp.MustCompile(`^\s*(belongs_to|has_many|has_one|has_and_belongs_to_many)\s+:(\w+)`)
	attrPattern        = regexp.MustCompile(`^\s*(attr_accessor|attr_reader|attr_writer)\s+(.+)`)
	localAssignPattern = regexp.MustCompile(`(?:^|[;\s])([a-z_]\w*)\s*(?:\|\|=|&&=|[+\-*\/%]?=)`)
	constructorPattern = regexp.MustCompile(`([A-Z][A-Za-z0-9_:]*)\s*\.\s*new\b`)
	methodNamePattern  = regexp.MustCompile(`^(?:(self|[A-Z][\w:]*)\.)?([a-zA-Z_]\w*[!?=]?)(?:\s*\((.*)\)|\s+(.+))?$`)
	parameterPattern   = regexp.MustCompile(`(?:^|[,\s])(?:\*{0,2}|&)?([a-z_]\w*)`)
	htmlTagPattern     = regexp.MustCompile(`</?([A-Za-z][A-Za-z0-9:_-]*)([^>]*)>`)
	htmlCommentPattern = regexp.MustCompile(`(?s)<!--.*?-->`)
)

var skipDirs = map[string]bool{
	"vendor": true, "node_modules": true, ".git": true, "tmp": true,
	"log": true, ".bundle": true, "coverage": true, "public": true,
	"storage": true,
}

func New(workspaceRoot string, logger *log.Logger) *Index {
	if logger == nil {
		logger = log.New(os.Stderr, "[RubyLSP-Go] ", log.LstdFlags)
	}
	return &Index{
		symbols: make(map[string][]SymbolEntry), fileSymbols: make(map[string][]SymbolEntry),
		workspaceRoot: normalizePath(workspaceRoot), logger: logger,
	}
}

func (idx *Index) IsReady() bool { idx.mutex.RLock(); defer idx.mutex.RUnlock(); return idx.ready }

// BuildIndex creates a fresh index. Ruby and ERB files are both indexed and
// paths are normalized so URI paths and filesystem walks address the same file.
func (idx *Index) BuildIndex() {
	idx.logger.Printf("Starting workspace indexing: %s", idx.workspaceRoot)
	idx.mutex.Lock()
	idx.symbols = make(map[string][]SymbolEntry)
	idx.fileSymbols = make(map[string][]SymbolEntry)
	idx.ready = false
	idx.mutex.Unlock()

	fileCount, symbolCount := 0, 0
	err := filepath.Walk(idx.workspaceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !isRubyFile(path) {
			return nil
		}
		entries := idx.ParseFile(path)
		idx.replaceEntries(path, entries)
		fileCount++
		symbolCount += len(entries)
		return nil
	})
	if err != nil {
		idx.logger.Printf("Error during indexing: %v", err)
	}
	idx.mutex.Lock()
	idx.ready = true
	idx.mutex.Unlock()
	idx.logger.Printf("Indexing complete: %d files, %d symbols", fileCount, symbolCount)
}

func (idx *Index) ParseFile(filePath string) []SymbolEntry {
	source, err := os.ReadFile(filePath)
	if err != nil {
		return nil
	}
	return idx.ParseSource(filePath, string(source))
}

// ParseSource is also used for open/unsaved documents, which must be visible
// to LSP requests before they are written to disk.
func (idx *Index) ParseSource(filePath, source string) []SymbolEntry {
	languageID := "ruby"
	if strings.EqualFold(filepath.Ext(filePath), ".erb") || strings.EqualFold(filepath.Ext(filePath), ".rhtml") {
		languageID = "erb"
		source = ERBToRubySource(source)
	}
	return parseRubySource(normalizePath(filePath), source, languageID)
}

// UpdateSource replaces all declarations for a file, including when the new
// source contains no declarations.
func (idx *Index) UpdateSource(filePath, source string) {
	entries := idx.ParseSource(filePath, source)
	idx.replaceEntries(filePath, entries)
}

func (idx *Index) UpdateFile(filePath string) {
	normalized := normalizePath(filePath)
	source, err := os.ReadFile(normalized)
	if err != nil {
		idx.replaceEntries(normalized, nil)
		idx.logger.Printf("Removed unavailable file from index: %s", normalized)
		return
	}
	idx.UpdateSource(normalized, string(source))
	idx.logger.Printf("Re-indexed file: %s", normalized)
}

func (idx *Index) replaceEntries(filePath string, entries []SymbolEntry) {
	filePath = normalizePath(filePath)
	idx.mutex.Lock()
	defer idx.mutex.Unlock()
	idx.removeEntriesLocked(filePath)
	if len(entries) == 0 {
		return
	}
	idx.fileSymbols[filePath] = append([]SymbolEntry(nil), entries...)
	for _, entry := range entries {
		if entry.Type == SymbolLocalVariable {
			continue
		}
		idx.symbols[entry.Name] = append(idx.symbols[entry.Name], entry)
		if entry.FullyQualifiedName != entry.Name {
			idx.symbols[entry.FullyQualifiedName] = append(idx.symbols[entry.FullyQualifiedName], entry)
		}
	}
}

func (idx *Index) removeEntriesLocked(filePath string) {
	old := idx.fileSymbols[filePath]
	for _, entry := range old {
		for _, key := range []string{entry.Name, entry.FullyQualifiedName} {
			if key == "" {
				continue
			}
			values := idx.symbols[key]
			filtered := values[:0]
			for _, value := range values {
				if value.FilePath != filePath {
					filtered = append(filtered, value)
				}
			}
			if len(filtered) == 0 {
				delete(idx.symbols, key)
			} else {
				idx.symbols[key] = filtered
			}
		}
	}
	delete(idx.fileSymbols, filePath)
}

func (idx *Index) Lookup(name string) []SymbolEntry {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()
	return append([]SymbolEntry(nil), idx.symbols[strings.TrimPrefix(name, "::")]...)
}

func (idx *Index) PrefixSearch(prefix string) []SymbolEntry {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()
	lowerPrefix := strings.ToLower(prefix)
	var results []SymbolEntry
	for name, entries := range idx.symbols {
		if strings.HasPrefix(strings.ToLower(name), lowerPrefix) {
			results = append(results, entries...)
		}
	}
	return sortEntries(deduplicateEntries(results))
}

// ResolveLocalVariable only considers a declaration inside the method scope
// that contains the requested line. This prevents a method with the same name
// elsewhere in the workspace from winning a local-variable lookup.
func (idx *Index) ResolveLocalVariable(filePath, name string, line int) []SymbolEntry {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()
	filePath = normalizePath(filePath)
	var candidates []SymbolEntry
	for _, entry := range idx.fileSymbols[filePath] {
		if entry.Type != SymbolLocalVariable || entry.Name != name || line < entry.ScopeStartLine {
			continue
		}
		if entry.EndLine == 0 || line <= entry.EndLine {
			candidates = append(candidates, entry)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].ScopeStartLine != candidates[j].ScopeStartLine {
			return candidates[i].ScopeStartLine > candidates[j].ScopeStartLine
		}
		return candidates[i].Line < candidates[j].Line
	})
	return []SymbolEntry{candidates[0]}
}

// DefinitionCandidates applies the two pieces of context that a flat name
// index cannot infer: local-variable scope and Class.new -> initialize.
func (idx *Index) DefinitionCandidates(filePath, source string, line, character int) []SymbolEntry {
	word := GetWordAtPosition(source, line, character)
	if word == "" {
		return nil
	}
	if locals := idx.ResolveLocalVariable(filePath, word, line+1); len(locals) > 0 {
		return locals
	}
	if word == "new" {
		lines := strings.Split(source, "\n")
		if line >= 0 && line < len(lines) {
			if match := constructorPattern.FindStringSubmatch(lines[line]); match != nil {
				className := strings.TrimPrefix(match[1], "::")
				classEntries := idx.Lookup(className)
				var result []SymbolEntry
				for _, classEntry := range classEntries {
					for _, method := range idx.Lookup("initialize") {
						if method.Type == SymbolMethod && (method.Parent == classEntry.FullyQualifiedName || method.Parent == classEntry.Name) {
							result = append(result, method)
						}
					}
				}
				if len(result) > 0 {
					return deduplicateEntries(result)
				}
			}
		}
	}
	return idx.Lookup(strings.TrimPrefix(word, ":"))
}

func (idx *Index) LookupByConvention(word string) []SymbolEntry {
	if entries := idx.Lookup(word); len(entries) > 0 {
		return entries
	}
	snake := camelToSnake(strings.TrimPrefix(word, "::"))
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()
	var results []SymbolEntry
	for path, entries := range idx.fileSymbols {
		if !isRubyFile(path) || filepath.Base(path) != filepath.Base(snake)+".rb" {
			continue
		}
		for _, entry := range entries {
			if entry.Type == SymbolClass || entry.Type == SymbolModule {
				results = append(results, entry)
			}
		}
	}
	return results
}

func (idx *Index) GetFileSymbols(filePath string) []SymbolEntry {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()
	return append([]SymbolEntry(nil), idx.fileSymbols[normalizePath(filePath)]...)
}

// GetWordAtPosition uses rune columns (the default LSP encoding used by this
// server) and accepts a cursor immediately after a token.
func GetWordAtPosition(source string, line, character int) string {
	lines := strings.Split(source, "\n")
	if line < 0 || line >= len(lines) {
		return ""
	}
	runes := []rune(lines[line])
	if character < 0 || character > len(runes) || len(runes) == 0 {
		return ""
	}
	if character == len(runes) && character > 0 {
		character--
	}
	if !isWordChar(runes[character]) {
		return ""
	}
	start, end := character, character+1
	for start > 0 && isWordChar(runes[start-1]) {
		start--
	}
	for end < len(runes) && isWordChar(runes[end]) {
		end++
	}
	return string(runes[start:end])
}

// FoldingRanges recognizes Ruby block constructs using the same lexical
// masking as the indexer, so keywords in strings/comments/HTML do not create
// phantom folds.
func FoldingRanges(source, languageID string) []FoldingRange {
	var result []FoldingRange
	if strings.EqualFold(languageID, "erb") {
		// ERB has two folding languages: Ruby control blocks and the HTML host
		// document. Keep both sets so <head>/<body> fold as well as <% if %>.
		result = append(result, htmlFoldingRanges(source)...)
		source = ERBToRubySource(source)
	}
	type fold struct {
		start int
		kind  string
	}
	var stack []fold
	lines := strings.Split(source, "\n")
	for i, line := range lines {
		tokens := lexRubyLine(maskRubyLine(line))
		if len(tokens) == 0 {
			continue
		}
		first := tokens[0].text
		if first == "end" {
			if len(stack) > 0 {
				opening := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				if i > opening.start {
					result = append(result, FoldingRange{StartLine: opening.start, EndLine: i - 1, Kind: opening.kind})
				}
			}
			continue
		}
		if opensBlock(tokens, line) {
			stack = append(stack, fold{start: i, kind: foldKind(first)})
		}
	}
	for _, opening := range stack {
		if len(lines)-1 > opening.start {
			result = append(result, FoldingRange{StartLine: opening.start, EndLine: len(lines) - 1, Kind: opening.kind})
		}
	}
	return sortFoldingRanges(result)
}

func htmlFoldingRanges(source string) []FoldingRange {
	type htmlBlock struct {
		name  string
		start int
	}
	var stack []htmlBlock
	var result []FoldingRange

	// Comments are handled separately because they are not HTML tags.
	for _, match := range htmlCommentPattern.FindAllStringIndex(source, -1) {
		startLine := strings.Count(source[:match[0]], "\n")
		endLine := strings.Count(source[:match[1]], "\n")
		if endLine > startLine {
			result = append(result, FoldingRange{StartLine: startLine, EndLine: endLine, Kind: "comment"})
		}
	}

	voidTags := map[string]bool{
		"area": true, "base": true, "br": true, "col": true, "embed": true,
		"hr": true, "img": true, "input": true, "link": true, "meta": true,
		"param": true, "source": true, "track": true, "wbr": true,
	}
	for _, match := range htmlTagPattern.FindAllStringSubmatchIndex(source, -1) {
		full := source[match[0]:match[1]]
		name := source[match[2]:match[3]]
		startLine := strings.Count(source[:match[0]], "\n")
		if strings.HasPrefix(full, "</") {
			for i := len(stack) - 1; i >= 0; i-- {
				if !strings.EqualFold(stack[i].name, name) {
					continue
				}
				for j := len(stack) - 1; j >= i; j-- {
					opening := stack[j]
					// Keep the closing tag visible when the range is collapsed. This
					// mirrors Ruby folds, which keep the closing `end` visible.
					foldEnd := startLine - 1
					if foldEnd > opening.start {
						result = append(result, FoldingRange{StartLine: opening.start, EndLine: foldEnd})
					}
				}
				stack = stack[:i]
				break
			}
			continue
		}
		if voidTags[strings.ToLower(name)] || strings.HasSuffix(strings.TrimSpace(full), "/>") {
			continue
		}
		stack = append(stack, htmlBlock{name: name, start: startLine})
	}
	return result
}

func sortFoldingRanges(result []FoldingRange) []FoldingRange {
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].StartLine != result[j].StartLine {
			return result[i].StartLine < result[j].StartLine
		}
		if result[i].EndLine != result[j].EndLine {
			return result[i].EndLine < result[j].EndLine
		}
		return result[i].Kind < result[j].Kind
	})
	deduplicated := result[:0]
	seen := make(map[string]bool)
	for _, folding := range result {
		key := fmt.Sprintf("%d:%d:%s", folding.StartLine, folding.EndLine, folding.Kind)
		if !seen[key] {
			seen[key] = true
			deduplicated = append(deduplicated, folding)
		}
	}
	return deduplicated
}

type rubyToken struct {
	text       string
	start, end int
}
type parseBlock struct {
	kind, fqn         string
	entry             int
	start             int
	namespace, method bool
}

func parseRubySource(filePath, source, languageID string) []SymbolEntry {
	lines := strings.Split(source, "\n")
	entries := make([]SymbolEntry, 0)
	var stack []parseBlock
	visibility := "public"
	for lineIndex, original := range lines {
		lineNumber := lineIndex + 1
		masked := maskRubyLine(original)
		trimmed := strings.TrimSpace(masked)
		if trimmed == "" {
			continue
		}
		tokens := lexRubyLine(masked)
		if len(tokens) == 0 {
			continue
		}
		first := tokens[0].text

		if first == "end" {
			closeParseBlock(&stack, &entries, lineNumber)
			continue
		}
		if first == "private" || first == "protected" || first == "public" {
			visibility = first
			continue
		}

		namespace := currentNamespace(stack)
		method := currentMethod(stack)
		if matches := classPattern.FindStringSubmatch(masked); matches != nil {
			name, superclass := matches[1], matches[2]
			fqn := qualify(namespace, name)
			entry := SymbolEntry{Name: name, FullyQualifiedName: fqn, Type: SymbolClass, FilePath: filePath, Line: lineNumber, EndLine: lineNumber, Character: nameColumn(original, "class", name), EndCharacter: nameColumn(original, "class", name) + runeLen(name), Parent: namespace, Visibility: "public", Detail: superclass}
			entries = append(entries, entry)
			stack = append(stack, parseBlock{kind: "class", fqn: fqn, entry: len(entries) - 1, start: lineNumber, namespace: true})
			if inlineEnd(masked) {
				closeParseBlock(&stack, &entries, lineNumber)
			}
			visibility = "public"
			continue
		}
		if matches := modulePattern.FindStringSubmatch(masked); matches != nil {
			name := matches[1]
			fqn := qualify(namespace, name)
			col := nameColumn(original, "module", name)
			entries = append(entries, SymbolEntry{Name: name, FullyQualifiedName: fqn, Type: SymbolModule, FilePath: filePath, Line: lineNumber, EndLine: lineNumber, Character: col, EndCharacter: col + runeLen(name), Parent: namespace, Visibility: "public"})
			stack = append(stack, parseBlock{kind: "module", fqn: fqn, entry: len(entries) - 1, start: lineNumber, namespace: true})
			if inlineEnd(masked) {
				closeParseBlock(&stack, &entries, lineNumber)
			}
			visibility = "public"
			continue
		}
		if matches := methodPattern.FindStringSubmatch(masked); matches != nil {
			receiver, methodName, params, singleton, ok := parseMethod(matches[1])
			if ok {
				if receiver != "" && receiver != "self" {
					namespace = receiver
				}
				fqn := methodName
				if namespace != "" {
					if singleton {
						fqn = namespace + "." + methodName
					} else {
						fqn = namespace + "#" + methodName
					}
				}
				kind := SymbolMethod
				if singleton {
					kind = SymbolSingletonMethod
				}
				col := nameColumn(original, "def", methodName)
				entries = append(entries, SymbolEntry{Name: methodName, FullyQualifiedName: fqn, Type: kind, FilePath: filePath, Line: lineNumber, EndLine: lineNumber, Character: col, EndCharacter: col + runeLen(methodName), Parent: namespace, Visibility: visibility})
				methodIndex := len(entries) - 1
				stack = append(stack, parseBlock{kind: "def", fqn: fqn, entry: methodIndex, start: lineNumber, method: true})
				addLocalParameters(&entries, filePath, lineNumber, params, fqn, lineNumber, original)
				if inlineEnd(masked) {
					closeParseBlock(&stack, &entries, lineNumber)
				}
				continue
			}
		}

		if matches := constantPattern.FindStringSubmatch(masked); matches != nil {
			name := matches[2]
			col := runeColumn(original, name)
			fqn := name
			if namespace != "" {
				fqn = namespace + "::" + name
			}
			entries = append(entries, SymbolEntry{Name: name, FullyQualifiedName: fqn, Type: SymbolConstant, FilePath: filePath, Line: lineNumber, EndLine: lineNumber, Character: col, EndCharacter: col + runeLen(name), Parent: namespace, Visibility: visibility})
			continue
		}
		if matches := scopePattern.FindStringSubmatch(masked); matches != nil {
			name := matches[1]
			col := runeColumn(original, ":"+name) + 1
			entries = append(entries, SymbolEntry{Name: name, FullyQualifiedName: namespace + "." + name, Type: SymbolScope, FilePath: filePath, Line: lineNumber, EndLine: lineNumber, Character: col, EndCharacter: col + runeLen(name), Parent: namespace, Visibility: "public", Detail: "scope"})
			continue
		}
		if matches := associationPattern.FindStringSubmatch(masked); matches != nil {
			name := matches[2]
			col := runeColumn(original, ":"+name) + 1
			entries = append(entries, SymbolEntry{Name: name, FullyQualifiedName: namespace + "#" + name, Type: SymbolAssociation, FilePath: filePath, Line: lineNumber, EndLine: lineNumber, Character: col, EndCharacter: col + runeLen(name), Parent: namespace, Visibility: "public", Detail: matches[1]})
			continue
		}
		if matches := attrPattern.FindStringSubmatch(masked); matches != nil {
			for _, symbol := range regexp.MustCompile(`:([a-zA-Z_]\w*)`).FindAllStringSubmatch(matches[2], -1) {
				name := symbol[1]
				col := runeColumn(original, ":"+name) + 1
				entries = append(entries, SymbolEntry{Name: name, FullyQualifiedName: namespace + "#" + name, Type: SymbolAttrAccessor, FilePath: filePath, Line: lineNumber, EndLine: lineNumber, Character: col, EndCharacter: col + runeLen(name), Parent: namespace, Visibility: visibility, Detail: matches[1]})
			}
			continue
		}
		if method != nil {
			for _, match := range localAssignPattern.FindAllStringSubmatch(masked, -1) {
				name := match[1]
				if hasLocalInScope(entries, name, method.start, lineNumber) {
					continue
				}
				col := runeColumn(original, name)
				entries = append(entries, SymbolEntry{Name: name, FullyQualifiedName: name, Type: SymbolLocalVariable, FilePath: filePath, Line: lineNumber, EndLine: 0, Character: col, EndCharacter: col + runeLen(name), Parent: method.fqn, ScopeStartLine: method.start, Visibility: "local"})
			}
		}
		if opensBlock(tokens, masked) {
			stack = append(stack, parseBlock{kind: first, start: lineNumber})
		}
	}
	for len(stack) > 0 {
		closeParseBlock(&stack, &entries, len(lines))
	}
	return entries
}

func closeParseBlock(stack *[]parseBlock, entries *[]SymbolEntry, line int) {
	if len(*stack) == 0 {
		return
	}
	block := (*stack)[len(*stack)-1]
	*stack = (*stack)[:len(*stack)-1]
	if block.entry >= 0 && block.entry < len(*entries) {
		(*entries)[block.entry].EndLine = line
	}
	if block.method {
		for i := range *entries {
			if (*entries)[i].Type == SymbolLocalVariable && (*entries)[i].Parent == block.fqn && (*entries)[i].ScopeStartLine == block.start && (*entries)[i].EndLine == 0 {
				(*entries)[i].EndLine = line
			}
		}
	}
}

func currentNamespace(stack []parseBlock) string {
	result := ""
	for _, block := range stack {
		if block.namespace {
			result = block.fqn
		}
	}
	return result
}
func currentMethod(stack []parseBlock) *parseBlock {
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].method {
			return &stack[i]
		}
	}
	return nil
}
func qualify(parent, name string) string {
	name = strings.TrimPrefix(name, "::")
	if strings.Contains(name, "::") || parent == "" {
		return name
	}
	return parent + "::" + name
}

func parseMethod(raw string) (receiver, name, params string, singleton, ok bool) {
	raw = strings.TrimSpace(raw)
	if semicolon := strings.Index(raw, ";"); semicolon >= 0 {
		raw = strings.TrimSpace(raw[:semicolon])
	}
	match := methodNamePattern.FindStringSubmatch(raw)
	if match == nil {
		return
	}
	receiver, name, params = match[1], match[2], match[3]
	if params == "" {
		params = match[4]
	}
	singleton = receiver != ""
	ok = name != ""
	return
}

func addLocalParameters(entries *[]SymbolEntry, filePath string, line int, params, method string, scope int, original string) {
	if params == "" {
		return
	}
	for _, match := range parameterPattern.FindAllStringSubmatch(params, -1) {
		name := match[1]
		if name == "" {
			continue
		}
		col := runeColumn(original, name)
		*entries = append(*entries, SymbolEntry{Name: name, FullyQualifiedName: name, Type: SymbolLocalVariable, FilePath: filePath, Line: line, Character: col, EndCharacter: col + runeLen(name), Parent: method, ScopeStartLine: scope, Visibility: "local"})
	}
}

func hasLocalInScope(entries []SymbolEntry, name string, start, line int) bool {
	for _, entry := range entries {
		if entry.Type == SymbolLocalVariable && entry.Name == name && entry.ScopeStartLine == start && entry.Line <= line {
			return true
		}
	}
	return false
}

func opensBlock(tokens []rubyToken, line string) bool {
	if len(tokens) == 0 {
		return false
	}
	first := tokens[0].text
	if strings.Contains(line, ";end") || strings.Contains(line, "; end") {
		return false
	}
	switch first {
	case "class", "module", "def", "if", "unless", "case", "begin", "while", "until", "for":
		return true
	}
	for _, token := range tokens {
		if token.text == "do" {
			return true
		}
	}
	return false
}

func inlineEnd(line string) bool {
	parts := strings.Split(line, ";")
	return len(parts) > 1 && strings.TrimSpace(parts[len(parts)-1]) == "end"
}

func foldKind(keyword string) string {
	if keyword == "def" || keyword == "class" || keyword == "module" {
		return "region"
	}
	return ""
}

func maskRubyLine(line string) string {
	runes := []rune(line)
	out := append([]rune(nil), runes...)
	var quote rune
	escaped := false
	for i, r := range runes {
		if quote != 0 {
			if escaped {
				out[i] = ' '
				escaped = false
				continue
			}
			if r == '\\' {
				out[i] = ' '
				escaped = true
				continue
			}
			if r == quote {
				quote = 0
			} else if r != '\t' {
				out[i] = ' '
			}
			continue
		}
		if r == '\'' || r == '"' || r == '`' {
			quote = r
			out[i] = ' '
			continue
		}
		if r == '#' {
			for j := i; j < len(out); j++ {
				if out[j] != '\t' {
					out[j] = ' '
				}
			}
			break
		}
	}
	return string(out)
}

func lexRubyLine(line string) []rubyToken {
	runes := []rune(line)
	var tokens []rubyToken
	for i := 0; i < len(runes); {
		if unicode.IsLetter(runes[i]) || runes[i] == '_' {
			start := i
			i++
			for i < len(runes) && (unicode.IsLetter(runes[i]) || unicode.IsDigit(runes[i]) || runes[i] == '_' || runes[i] == '!' || runes[i] == '?' || runes[i] == '=') {
				i++
			}
			tokens = append(tokens, rubyToken{text: string(runes[start:i]), start: start, end: i})
			continue
		}
		if unicode.IsDigit(runes[i]) {
			i++
			continue
		}
		i++
	}
	return tokens
}

func ERBToRubySource(source string) string {
	runes := []rune(source)
	out := make([]rune, len(runes))
	inside := false
	comment := false
	for i := 0; i < len(runes); i++ {
		if !inside && runes[i] == '<' && i+1 < len(runes) && runes[i+1] == '%' {
			inside = true
			comment = i+2 < len(runes) && runes[i+2] == '#'
			out[i] = ' '
			out[i+1] = ' '
			i++
			if comment {
				out[i+1] = ' '
				i++
			}
			continue
		}
		if inside && runes[i] == '%' && i+1 < len(runes) && runes[i+1] == '>' {
			inside = false
			out[i] = ' '
			out[i+1] = ' '
			i++
			continue
		}
		if runes[i] == '\n' || runes[i] == '\r' {
			out[i] = runes[i]
			continue
		}
		if inside && !comment {
			out[i] = runes[i]
		} else {
			out[i] = ' '
		}
	}
	return string(out)
}

func nameColumn(line, keyword, name string) int {
	start := runeColumn(line, keyword)
	if start < 0 {
		return 0
	}
	rest := []rune(line)[start+runeLen(keyword):]
	offset := 0
	for offset < len(rest) && (rest[offset] == ' ' || rest[offset] == '\t') {
		offset++
	}
	return start + runeLen(keyword) + offset
}
func runeColumn(line, value string) int {
	lineRunes, valueRunes := []rune(line), []rune(value)
	if len(valueRunes) == 0 {
		return 0
	}
	for i := 0; i+len(valueRunes) <= len(lineRunes); i++ {
		match := true
		for j := range valueRunes {
			if lineRunes[i+j] != valueRunes[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}
func runeLen(value string) int { return len([]rune(value)) }
func normalizePath(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	return filepath.Clean(path)
}
func isRubyFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".rb" || ext == ".erb" || ext == ".rhtml"
}
func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == ':' || r == '!' || r == '?' || r == '='
}
func camelToSnake(s string) string {
	s = strings.ReplaceAll(s, "::", "/")
	var result strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 && runes[i-1] != '/' && !unicode.IsUpper(runes[i-1]) {
				result.WriteRune('_')
			}
			result.WriteRune(unicode.ToLower(r))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}
func deduplicateEntries(entries []SymbolEntry) []SymbolEntry {
	seen := map[string]bool{}
	result := make([]SymbolEntry, 0, len(entries))
	for _, e := range entries {
		key := fmt.Sprintf("%s:%d:%s", e.FilePath, e.Line, e.Name)
		if !seen[key] {
			seen[key] = true
			result = append(result, e)
		}
	}
	return result
}
func sortEntries(entries []SymbolEntry) []SymbolEntry {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].FilePath != entries[j].FilePath {
			return entries[i].FilePath < entries[j].FilePath
		}
		if entries[i].Line != entries[j].Line {
			return entries[i].Line < entries[j].Line
		}
		return entries[i].Character < entries[j].Character
	})
	return entries
}

func SymbolKindToLSP(t SymbolType) int {
	switch t {
	case SymbolClass:
		return 5
	case SymbolModule:
		return 2
	case SymbolMethod, SymbolSingletonMethod:
		return 6
	case SymbolConstant:
		return 14
	case SymbolScope:
		return 6
	case SymbolAssociation, SymbolAttrAccessor:
		return 7
	case SymbolLocalVariable:
		return 13
	default:
		return 1
	}
}
func CompletionKindFromType(t SymbolType) int {
	switch t {
	case SymbolClass:
		return 7
	case SymbolModule:
		return 9
	case SymbolMethod, SymbolSingletonMethod:
		return 2
	case SymbolConstant:
		return 21
	case SymbolScope:
		return 2
	case SymbolAssociation:
		return 5
	case SymbolAttrAccessor:
		return 10
	case SymbolLocalVariable:
		return 6
	default:
		return 1
	}
}
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
	case SymbolLocalVariable:
		return "local variable"
	default:
		return "symbol"
	}
}
