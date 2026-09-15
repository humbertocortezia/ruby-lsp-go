package indexer

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// SymbolType represents the kind of Ruby symbol
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
	SymbolLocalVariable
)

// SymbolEntry represents a single indexed symbol
type SymbolEntry struct {
	Name               string
	FullyQualifiedName string
	Type               SymbolType
	FilePath           string
	Line               int
	EndLine            int
	Character          int
	EndCharacter       int
	Parent             string // enclosing class/module
	Visibility         string // public, private, protected
	Detail             string // extra info (e.g., superclass, association type)
}

// Index is the main symbol index for the workspace
type Index struct {
	symbols       map[string][]SymbolEntry // name -> entries
	fileSymbols   map[string][]SymbolEntry // filePath -> entries
	mutex         sync.RWMutex
	workspaceRoot string
	logger        *log.Logger
	ready         bool
}

// Regex patterns for Ruby constructs
var (
	classPattern           = regexp.MustCompile(`^\s*class\s+([A-Z][\w:]*)\s*(?:<\s*([A-Z][\w:]*))?`)
	modulePattern          = regexp.MustCompile(`^\s*module\s+([A-Z][\w:]*)`)
	methodPattern          = regexp.MustCompile(`^\s*def\s+(self\.)?(\w+[!?=]?)`)
	constantPattern        = regexp.MustCompile(`^\s*([A-Z][A-Z0-9_]*)\s*=`)
	scopePattern           = regexp.MustCompile(`^\s*scope\s+:(\w+)`)
	associationPattern     = regexp.MustCompile(`^\s*(belongs_to|has_many|has_one|has_and_belongs_to_many)\s+:(\w+)`)
	attrPattern            = regexp.MustCompile(`^\s*(attr_accessor|attr_reader|attr_writer)\s+(.+)`)
	symbolExtractPattern   = regexp.MustCompile(`:(\w+)`)
	endPattern             = regexp.MustCompile(`^\s*end\b`)
	privatePattern         = regexp.MustCompile(`^\s*(private|protected|public)\s*$`)
	includePattern         = regexp.MustCompile(`^\s*(include|extend|prepend)\s+([A-Z][\w:]*)`)
	localAssignmentPattern = regexp.MustCompile(`\b([a-z_][a-zA-Z0-9_]*)\s*=`)
	methodCallPattern      = regexp.MustCompile(`([A-Z][a-zA-Z0-9_:]*)\s*\.\s*([a-z_][a-zA-Z0-9_]*[!?=]?)`)
)

// Directories to skip during indexing
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

// New creates a new Index
func New(workspaceRoot string, logger *log.Logger) *Index {
	return &Index{
		symbols:       make(map[string][]SymbolEntry),
		fileSymbols:   make(map[string][]SymbolEntry),
		workspaceRoot: workspaceRoot,
		logger:        logger,
		ready:         false,
	}
}

// IsReady returns whether the index has finished building
func (idx *Index) IsReady() bool {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()
	return idx.ready
}

// BuildIndex scans the workspace and indexes all Ruby files
func (idx *Index) BuildIndex() {
	idx.logger.Printf("Starting workspace indexing: %s", idx.workspaceRoot)

	fileCount := 0
	symbolCount := 0

	err := filepath.Walk(idx.workspaceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}

		// Skip ignored directories
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process .rb files
		if filepath.Ext(path) != ".rb" {
			return nil
		}

		entries := idx.ParseFile(path)
		if len(entries) > 0 {
			idx.mutex.Lock()
			idx.fileSymbols[path] = entries
			for _, entry := range entries {
				idx.symbols[entry.Name] = append(idx.symbols[entry.Name], entry)
				if entry.FullyQualifiedName != entry.Name {
					idx.symbols[entry.FullyQualifiedName] = append(idx.symbols[entry.FullyQualifiedName], entry)
				}
			}
			idx.mutex.Unlock()

			fileCount++
			symbolCount += len(entries)
		}

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

// ParseFile parses a single Ruby file and extracts symbol definitions
func (idx *Index) ParseFile(filePath string) []SymbolEntry {
	file, err := os.Open(filePath)
	if err != nil {
		return nil
	}
	defer file.Close()

	var entries []SymbolEntry
	scanner := bufio.NewScanner(file)

	// Stack to track nesting (class/module hierarchy)
	var nestingStack []string
	var indentStack []int
	currentVisibility := "public"
	lineNumber := 0

	for scanner.Scan() {
		line := scanner.Text()
		lineNumber++

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		indent := countIndent(line)

		// Track end keywords to pop nesting
		if endPattern.MatchString(line) {
			if len(indentStack) > 0 && indent <= indentStack[len(indentStack)-1] {
				nestingStack = nestingStack[:len(nestingStack)-1]
				indentStack = indentStack[:len(indentStack)-1]
				currentVisibility = "public"
			}
			continue
		}

		// Track visibility modifiers
		if matches := privatePattern.FindStringSubmatch(line); matches != nil {
			currentVisibility = matches[1]
			continue
		}

		parent := ""
		if len(nestingStack) > 0 {
			parent = strings.Join(nestingStack, "::")
		}

		// Class definition
		if matches := classPattern.FindStringSubmatch(line); matches != nil {
			className := matches[1]
			superclass := matches[2]

			fqn := className
			if parent != "" && !strings.Contains(className, "::") {
				fqn = parent + "::" + className
			}

			entries = append(entries, SymbolEntry{
				Name:               className,
				FullyQualifiedName: fqn,
				Type:               SymbolClass,
				FilePath:           filePath,
				Line:               lineNumber,
				Character:          strings.Index(line, "class") + 6,
				Parent:             parent,
				Visibility:         "public",
				Detail:             superclass,
			})

			nestingStack = append(nestingStack, classNameOnly(className))
			indentStack = append(indentStack, indent)
			currentVisibility = "public"
			continue
		}

		// Module definition
		if matches := modulePattern.FindStringSubmatch(line); matches != nil {
			moduleName := matches[1]

			fqn := moduleName
			if parent != "" && !strings.Contains(moduleName, "::") {
				fqn = parent + "::" + moduleName
			}

			entries = append(entries, SymbolEntry{
				Name:               moduleName,
				FullyQualifiedName: fqn,
				Type:               SymbolModule,
				FilePath:           filePath,
				Line:               lineNumber,
				Character:          strings.Index(line, "module") + 7,
				Parent:             parent,
				Visibility:         "public",
			})

			nestingStack = append(nestingStack, classNameOnly(moduleName))
			indentStack = append(indentStack, indent)
			currentVisibility = "public"
			continue
		}

		// Method definition
		if matches := methodPattern.FindStringSubmatch(line); matches != nil {
			isSingleton := matches[1] != ""
			methodName := matches[2]

			symType := SymbolMethod
			if isSingleton {
				symType = SymbolSingletonMethod
			}

			fqn := methodName
			if parent != "" {
				sep := "#"
				if isSingleton {
					sep = "."
				}
				fqn = parent + sep + methodName
			}

			entries = append(entries, SymbolEntry{
				Name:               methodName,
				FullyQualifiedName: fqn,
				Type:               symType,
				FilePath:           filePath,
				Line:               lineNumber,
				Character:          strings.Index(line, "def") + 4,
				Parent:             parent,
				Visibility:         currentVisibility,
			})
			continue
		}

		// Constant assignment
		if matches := constantPattern.FindStringSubmatch(line); matches != nil {
			constName := matches[1]

			fqn := constName
			if parent != "" {
				fqn = parent + "::" + constName
			}

			entries = append(entries, SymbolEntry{
				Name:               constName,
				FullyQualifiedName: fqn,
				Type:               SymbolConstant,
				FilePath:           filePath,
				Line:               lineNumber,
				Character:          strings.Index(line, constName),
				Parent:             parent,
				Visibility:         "public",
			})
			continue
		}

		// Scope definition
		if matches := scopePattern.FindStringSubmatch(line); matches != nil {
			scopeName := matches[1]

			entries = append(entries, SymbolEntry{
				Name:               scopeName,
				FullyQualifiedName: parent + "." + scopeName,
				Type:               SymbolScope,
				FilePath:           filePath,
				Line:               lineNumber,
				Character:          strings.Index(line, ":"+scopeName) + 1,
				Parent:             parent,
				Visibility:         "public",
				Detail:             "scope",
			})
			continue
		}

		// Associations (belongs_to, has_many, has_one)
		if matches := associationPattern.FindStringSubmatch(line); matches != nil {
			assocType := matches[1]
			assocName := matches[2]

			entries = append(entries, SymbolEntry{
				Name:               assocName,
				FullyQualifiedName: parent + "#" + assocName,
				Type:               SymbolAssociation,
				FilePath:           filePath,
				Line:               lineNumber,
				Character:          strings.Index(line, ":"+assocName) + 1,
				Parent:             parent,
				Visibility:         "public",
				Detail:             assocType,
			})
			continue
		}

		// Attr accessors
		if matches := attrPattern.FindStringSubmatch(line); matches != nil {
			attrType := matches[1]
			attrList := matches[2]

			for _, sym := range symbolExtractPattern.FindAllStringSubmatch(attrList, -1) {
				attrName := sym[1]
				entries = append(entries, SymbolEntry{
					Name:               attrName,
					FullyQualifiedName: parent + "#" + attrName,
					Type:               SymbolAttrAccessor,
					FilePath:           filePath,
					Line:               lineNumber,
					Character:          strings.Index(line, ":"+attrName) + 1,
					Parent:             parent,
					Visibility:         currentVisibility,
					Detail:             attrType,
				})
			}
			continue
		}
	}

	return entries
}

// Lookup finds symbols by exact name
func (idx *Index) Lookup(name string) []SymbolEntry {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()

	if entries, ok := idx.symbols[name]; ok {
		return entries
	}
	return nil
}

// LookupMethod finds a method defined on a specific receiver class.
// Looking up by both method name and parent avoids resolving calls such as
// MyClass.new to an unrelated method with the same name.
func (idx *Index) LookupMethod(receiver string, method string) []SymbolEntry {
	entries := idx.Lookup(method)
	results := make([]SymbolEntry, 0, len(entries))

	for _, entry := range entries {
		if entry.Parent == receiver && (entry.Type == SymbolMethod || entry.Type == SymbolSingletonMethod) {
			results = append(results, entry)
		}
	}

	return deduplicateEntries(results)
}

// FindLocalVariableDefinition resolves a local variable inside the method that
// contains the requested line. Local variables must be resolved before global
// symbols because Ruby permits a local variable and a method to share a name.
func FindLocalVariableDefinition(source string, line int, name string) (SymbolEntry, bool) {
	if name == "" || !isLocalVariableName(name) {
		return SymbolEntry{}, false
	}

	lines := strings.Split(source, "\n")
	if line < 0 || line >= len(lines) {
		return SymbolEntry{}, false
	}

	methodStart := -1
	methodIndent := 0
	for i := 0; i <= line; i++ {
		currentLine := lines[i]
		if matches := methodPattern.FindStringSubmatch(currentLine); matches != nil {
			methodStart = i
			methodIndent = countIndent(currentLine)
			continue
		}

		if methodStart >= 0 && endPattern.MatchString(currentLine) && countIndent(currentLine) <= methodIndent {
			methodStart = -1
		}
	}

	if methodStart < 0 {
		return SymbolEntry{}, false
	}

	// Parameters are local variables and are defined at the method declaration.
	for _, parameter := range methodParameters(lines[methodStart]) {
		if parameter.name == name {
			return SymbolEntry{
				Name:               name,
				FullyQualifiedName: name,
				Type:               SymbolLocalVariable,
				Line:               methodStart + 1,
				Character:          parameter.character,
				Parent:             methodName(lines[methodStart]),
			}, true
		}
	}

	// Ruby locals are scoped to the whole method, so search the complete method
	// body instead of only lines before the cursor.
	for i := methodStart + 1; i < len(lines); i++ {
		currentLine := lines[i]
		if i > methodStart && endPattern.MatchString(currentLine) && countIndent(currentLine) <= methodIndent {
			break
		}

		matches := localAssignmentPattern.FindAllStringSubmatchIndex(currentLine, -1)
		for _, match := range matches {
			if currentLine[match[2]:match[3]] != name {
				continue
			}

			return SymbolEntry{
				Name:               name,
				FullyQualifiedName: name,
				Type:               SymbolLocalVariable,
				Line:               i + 1,
				Character:          runeCount(currentLine[:match[2]]),
				Parent:             methodName(lines[methodStart]),
			}, true
		}
	}

	return SymbolEntry{}, false
}

// GetReceiverAtPosition returns the constant receiver of a method call when
// the cursor is on that call's method name (for example MyClass in
// MyClass.new). It intentionally only handles constant receivers; resolving a
// variable receiver requires type inference that this lightweight index does
// not provide yet.
func GetReceiverAtPosition(source string, line int, character int) (string, bool) {
	lines := strings.Split(source, "\n")
	if line < 0 || line >= len(lines) || character < 0 {
		return "", false
	}

	lineText := lines[line]
	characterByte := byteOffset(lineText, character)
	for _, match := range methodCallPattern.FindAllStringSubmatchIndex(lineText, -1) {
		methodStart := match[4]
		methodEnd := match[5]
		if characterByte >= methodStart && characterByte <= methodEnd {
			return lineText[match[2]:match[3]], true
		}
	}

	return "", false
}

// PrefixSearch finds symbols whose name starts with the given prefix
func (idx *Index) PrefixSearch(prefix string) []SymbolEntry {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()

	var results []SymbolEntry
	lowerPrefix := strings.ToLower(prefix)

	for name, entries := range idx.symbols {
		if strings.HasPrefix(strings.ToLower(name), lowerPrefix) {
			results = append(results, entries...)
		}
	}

	// Deduplicate by file+line
	return deduplicateEntries(results)
}

// LookupByConvention resolves a word to file paths using Rails conventions
func (idx *Index) LookupByConvention(word string) []SymbolEntry {
	// First try exact lookup
	if entries := idx.Lookup(word); len(entries) > 0 {
		return entries
	}

	// Convert CamelCase to snake_case for file lookup
	snakeName := camelToSnake(word)

	// Rails convention paths to try
	conventionPaths := []string{
		filepath.Join(idx.workspaceRoot, "app", "models", snakeName+".rb"),
		filepath.Join(idx.workspaceRoot, "app", "controllers", snakeName+".rb"),
		filepath.Join(idx.workspaceRoot, "app", "services", snakeName+".rb"),
		filepath.Join(idx.workspaceRoot, "app", "serializers", snakeName+".rb"),
		filepath.Join(idx.workspaceRoot, "app", "jobs", snakeName+".rb"),
		filepath.Join(idx.workspaceRoot, "app", "mailers", snakeName+".rb"),
		filepath.Join(idx.workspaceRoot, "app", "helpers", snakeName+".rb"),
		filepath.Join(idx.workspaceRoot, "app", "workers", snakeName+".rb"),
		filepath.Join(idx.workspaceRoot, "app", "policies", snakeName+".rb"),
		filepath.Join(idx.workspaceRoot, "app", "forms", snakeName+".rb"),
		filepath.Join(idx.workspaceRoot, "app", "decorators", snakeName+".rb"),
		filepath.Join(idx.workspaceRoot, "app", "validators", snakeName+".rb"),
		filepath.Join(idx.workspaceRoot, "app", "interactors", snakeName+".rb"),
		filepath.Join(idx.workspaceRoot, "app", "operations", snakeName+".rb"),
		filepath.Join(idx.workspaceRoot, "lib", snakeName+".rb"),
	}

	// Concerns (search both model and controller concerns)
	concernPaths := []string{
		filepath.Join(idx.workspaceRoot, "app", "models", "concerns", snakeName+".rb"),
		filepath.Join(idx.workspaceRoot, "app", "controllers", "concerns", snakeName+".rb"),
	}

	allPaths := append(conventionPaths, concernPaths...)

	var results []SymbolEntry
	for _, p := range allPaths {
		if _, err := os.Stat(p); err == nil {
			results = append(results, SymbolEntry{
				Name:               word,
				FullyQualifiedName: word,
				Type:               SymbolClass,
				FilePath:           p,
				Line:               1,
				Character:          0,
			})
		}
	}

	// Also try glob search for nested paths
	if len(results) == 0 {
		pattern := filepath.Join(idx.workspaceRoot, "app", "**", snakeName+".rb")
		if matches, err := filepath.Glob(pattern); err == nil {
			for _, m := range matches {
				results = append(results, SymbolEntry{
					Name:               word,
					FullyQualifiedName: word,
					Type:               SymbolClass,
					FilePath:           m,
					Line:               1,
					Character:          0,
				})
			}
		}
	}

	return results
}

// UpdateFile re-indexes a single file (incremental update)
func (idx *Index) UpdateFile(filePath string) {
	idx.mutex.Lock()

	// Remove old entries for this file
	if oldEntries, ok := idx.fileSymbols[filePath]; ok {
		for _, entry := range oldEntries {
			if entries, exists := idx.symbols[entry.Name]; exists {
				filtered := entries[:0]
				for _, e := range entries {
					if e.FilePath != filePath {
						filtered = append(filtered, e)
					}
				}
				if len(filtered) > 0 {
					idx.symbols[entry.Name] = filtered
				} else {
					delete(idx.symbols, entry.Name)
				}
			}
			if entry.FullyQualifiedName != entry.Name {
				if entries, exists := idx.symbols[entry.FullyQualifiedName]; exists {
					filtered := entries[:0]
					for _, e := range entries {
						if e.FilePath != filePath {
							filtered = append(filtered, e)
						}
					}
					if len(filtered) > 0 {
						idx.symbols[entry.FullyQualifiedName] = filtered
					} else {
						delete(idx.symbols, entry.FullyQualifiedName)
					}
				}
			}
		}
		delete(idx.fileSymbols, filePath)
	}

	idx.mutex.Unlock()

	// Re-parse the file
	newEntries := idx.ParseFile(filePath)
	if len(newEntries) > 0 {
		idx.mutex.Lock()
		idx.fileSymbols[filePath] = newEntries
		for _, entry := range newEntries {
			idx.symbols[entry.Name] = append(idx.symbols[entry.Name], entry)
			if entry.FullyQualifiedName != entry.Name {
				idx.symbols[entry.FullyQualifiedName] = append(idx.symbols[entry.FullyQualifiedName], entry)
			}
		}
		idx.mutex.Unlock()
	}

	idx.logger.Printf("Re-indexed file: %s (%d symbols)", filePath, len(newEntries))
}

// GetFileSymbols returns all symbols for a specific file
func (idx *Index) GetFileSymbols(filePath string) []SymbolEntry {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()

	if entries, ok := idx.fileSymbols[filePath]; ok {
		return entries
	}
	return nil
}

// GetWordAtPosition extracts the word/token at a given cursor position
func GetWordAtPosition(source string, line int, character int) string {
	lines := strings.Split(source, "\n")
	if line < 0 || line >= len(lines) {
		return ""
	}

	lineText := lines[line]
	runes := []rune(lineText)

	if character < 0 || character >= len(runes) {
		return ""
	}

	// Expand left
	start := character
	for start > 0 && isWordChar(runes[start-1]) {
		start--
	}

	// Expand right
	end := character
	for end < len(runes) && isWordChar(runes[end]) {
		end++
	}

	if start == end {
		return ""
	}

	return string(runes[start:end])
}

// SymbolKindToLSP converts our SymbolType to an LSP SymbolKind integer
func SymbolKindToLSP(t SymbolType) int {
	switch t {
	case SymbolClass:
		return 5 // Class
	case SymbolModule:
		return 2 // Module
	case SymbolMethod, SymbolSingletonMethod:
		return 6 // Method
	case SymbolConstant:
		return 14 // Constant
	case SymbolScope:
		return 6 // Method (scopes are callable)
	case SymbolAssociation:
		return 7 // Property
	case SymbolAttrAccessor:
		return 7 // Property
	case SymbolLocalVariable:
		return 13 // Variable
	default:
		return 1 // File
	}
}

// CompletionKindFromType returns the LSP CompletionItemKind
func CompletionKindFromType(t SymbolType) int {
	switch t {
	case SymbolClass:
		return 7 // Class
	case SymbolModule:
		return 9 // Module
	case SymbolMethod, SymbolSingletonMethod:
		return 2 // Method
	case SymbolConstant:
		return 21 // Constant
	case SymbolScope:
		return 2 // Method
	case SymbolAssociation:
		return 5 // Field
	case SymbolAttrAccessor:
		return 10 // Property
	case SymbolLocalVariable:
		return 6 // Variable
	default:
		return 1 // Text
	}
}

// SymbolTypeString returns a human-readable string for the symbol type
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

// --- Helper functions ---

func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == ':' || r == '!' || r == '?' || r == '='
}

type parameterLocation struct {
	name      string
	character int
}

func methodName(line string) string {
	matches := methodPattern.FindStringSubmatch(line)
	if len(matches) < 3 {
		return ""
	}
	return matches[2]
}

func methodParameters(line string) []parameterLocation {
	matches := methodPattern.FindStringSubmatchIndex(line)
	if len(matches) < 6 {
		return nil
	}

	restStart := matches[5]
	rest := strings.TrimSpace(line[restStart:])
	if strings.HasPrefix(rest, "(") {
		if closing := strings.LastIndex(rest, ")"); closing >= 0 {
			rest = rest[1:closing]
		} else {
			rest = rest[1:]
		}
	}

	parameterPattern := regexp.MustCompile(`(?:^|[,|])\s*(?:\*{0,2})\s*([a-z_][a-zA-Z0-9_]*)`)
	locations := parameterPattern.FindAllStringSubmatchIndex(rest, -1)
	parameters := make([]parameterLocation, 0, len(locations))
	for _, location := range locations {
		parameters = append(parameters, parameterLocation{
			name:      rest[location[2]:location[3]],
			character: runeCount(line[:restStart]) + runeCount(rest[:location[2]]),
		})
	}

	return parameters
}

func isLocalVariableName(name string) bool {
	if name == "" {
		return false
	}

	first := rune(name[0])
	return first == '_' || (first >= 'a' && first <= 'z')
}

func runeCount(value string) int {
	return utf8.RuneCountInString(value)
}

func byteOffset(value string, character int) int {
	if character <= 0 {
		return 0
	}

	for byteIndex, runeIndex := 0, 0; byteIndex < len(value); {
		_, size := utf8.DecodeRuneInString(value[byteIndex:])
		if runeIndex == character {
			return byteIndex
		}
		byteIndex += size
		runeIndex++
	}

	return len(value)
}

func countIndent(line string) int {
	count := 0
	for _, ch := range line {
		if ch == ' ' {
			count++
		} else if ch == '\t' {
			count += 2
		} else {
			break
		}
	}
	return count
}

func classNameOnly(name string) string {
	parts := strings.Split(name, "::")
	return parts[len(parts)-1]
}

func camelToSnake(s string) string {
	// Handle :: namespace separator
	s = strings.ReplaceAll(s, "::", "/")

	var result strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 && runes[i-1] != '/' {
				// Don't add underscore if previous char was also uppercase and next is lowercase (e.g., HTMLParser -> html_parser)
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

func deduplicateEntries(entries []SymbolEntry) []SymbolEntry {
	seen := make(map[string]bool)
	var result []SymbolEntry

	for _, e := range entries {
		key := fmt.Sprintf("%s:%d:%s", e.FilePath, e.Line, e.Name)
		if !seen[key] {
			seen[key] = true
			result = append(result, e)
		}
	}

	return result
}
