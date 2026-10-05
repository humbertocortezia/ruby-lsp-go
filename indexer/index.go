package indexer

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/humberto/ruby-lsp-go/parser"
)

// IndexStore is the AST-based workspace symbol index.
type IndexStore struct {
	entries       map[string][]Entry
	uriToEntries  map[string][]Entry
	prefixTree    *PrefixTree
	ancestors     map[string][]string
	mixins        map[string][]MixinEntry
	mutex         sync.RWMutex
	workspaceRoot string
	logger        *log.Logger
	ready         bool
	// Set once at construction; tests can inject a parser before indexing starts.
	parseSource func(string) (*parser.ParseResult, error)
}

// NewIndexStore creates a new AST-based index.
func NewIndexStore(workspaceRoot string, logger *log.Logger) *IndexStore {
	return &IndexStore{
		entries:       make(map[string][]Entry),
		uriToEntries:  make(map[string][]Entry),
		prefixTree:    NewPrefixTree(),
		ancestors:     make(map[string][]string),
		mixins:        make(map[string][]MixinEntry),
		workspaceRoot: workspaceRoot,
		logger:        logger,
		parseSource:   parser.ParseSource,
	}
}

// IsReady returns whether initial indexing completed.
func (idx *IndexStore) IsReady() bool {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()
	return idx.ready
}

// BuildIndex walks workspace and indexes all Ruby files using AST.
func (idx *IndexStore) BuildIndex() {
	idx.logger.Printf("Starting AST-based workspace indexing: %s", idx.workspaceRoot)
	fileCount := 0
	symbolCount := 0

	err := filepath.Walk(idx.workspaceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".rb" && ext != ".erb" {
			return nil
		}
		entries := idx.IndexFile(path, "")
		if len(entries) > 0 {
			fileCount++
			symbolCount += len(entries)
		}
		return nil
	})
	if err != nil {
		idx.logger.Printf("Indexing error: %v", err)
	}

	idx.mutex.Lock()
	idx.ready = true
	idx.mutex.Unlock()
	idx.logger.Printf("AST indexing complete: %d files, %d symbols", fileCount, symbolCount)
}

// Rebuild clears the current symbol state and indexes the workspace again.
// It is used after external changes such as a Git branch switch.
func (idx *IndexStore) Rebuild() {
	idx.mutex.Lock()
	idx.entries = make(map[string][]Entry)
	idx.uriToEntries = make(map[string][]Entry)
	idx.prefixTree = NewPrefixTree()
	idx.ancestors = make(map[string][]string)
	idx.mixins = make(map[string][]MixinEntry)
	idx.ready = false
	idx.mutex.Unlock()
	idx.BuildIndex()
}

// IndexFile parses and replaces a file's entries. On failure the old entries
// are removed, so queries never use symbols from an outdated source version.
func (idx *IndexStore) IndexFile(filePath, source string) []Entry {
	entries, err := idx.indexFile(filePath, source)
	if err != nil {
		idx.logParseFailure(filePath, err)
	}
	return entries
}

func (idx *IndexStore) indexFile(filePath, source string) ([]Entry, error) {
	entries, err := idx.parseFileOnly(filePath, source)
	idx.mutex.Lock()
	defer idx.mutex.Unlock()
	idx.deleteFileLocked(filePath)
	if err != nil {
		return nil, err
	}
	idx.addEntriesLocked(filePath, entries)
	return entries, nil
}

func (idx *IndexStore) logParseFailure(filePath string, err error) {
	idx.logger.Printf("Failed to index %s: %v", filePath, err)
	var failure *parser.PanicError
	if errors.As(err, &failure) {
		idx.logger.Printf("Parsing stack for %s:\n%s", filePath, failure.Stack)
	}
}

func (idx *IndexStore) addEntriesLocked(filePath string, entries []Entry) {
	idx.uriToEntries[filePath] = entries
	for _, e := range entries {
		idx.entries[e.GetName()] = append(idx.entries[e.GetName()], e)
		fqn := e.GetFullyQualifiedName()
		if fqn != e.GetName() {
			idx.entries[fqn] = append(idx.entries[fqn], e)
		}
		idx.prefixTree.Insert(e.GetName(), e)
		if fqn != e.GetName() {
			idx.prefixTree.Insert(fqn, e)
		}

		switch typed := e.(type) {
		case ClassEntry:
			if typed.Superclass != "" {
				idx.ancestors[fqn] = append(idx.ancestors[fqn], typed.Superclass)
			}
			idx.mixins[fqn] = typed.Mixins
		case ModuleEntry:
			idx.mixins[fqn] = typed.Mixins
		}
	}
}

// DeleteFile removes all entries for a file.
func (idx *IndexStore) DeleteFile(filePath string) {
	idx.mutex.Lock()
	defer idx.mutex.Unlock()
	idx.deleteFileLocked(filePath)
}

func (idx *IndexStore) deleteFileLocked(filePath string) {
	oldEntries, ok := idx.uriToEntries[filePath]
	if !ok {
		return
	}

	for _, e := range oldEntries {
		idx.removeFromMap(idx.entries, e.GetName(), filePath)
		fqn := e.GetFullyQualifiedName()
		if fqn != e.GetName() {
			idx.removeFromMap(idx.entries, fqn, filePath)
		}
		delete(idx.ancestors, fqn)
		delete(idx.mixins, fqn)
	}
	delete(idx.uriToEntries, filePath)
	idx.prefixTree.Delete(filePath)
}

func (idx *IndexStore) removeFromMap(m map[string][]Entry, key, filePath string) {
	entries, ok := m[key]
	if !ok {
		return
	}
	// Lookup/AllEntries readers may still hold the previous slice after their
	// read lock is released. Do not overwrite that snapshot while replacing it.
	filtered := make([]Entry, 0, len(entries))
	for _, e := range entries {
		if e.GetLocation().URI != fileURI(filePath) && filePathFromURI(e.GetLocation().URI) != filePath {
			filtered = append(filtered, e)
		}
	}
	if len(filtered) > 0 {
		m[key] = filtered
	} else {
		delete(m, key)
	}
}

// UpdateFile re-indexes a file incrementally.
func (idx *IndexStore) UpdateFile(filePath string) {
	if _, err := idx.indexFile(filePath, ""); err != nil {
		idx.logParseFailure(filePath, err)
		return
	}
	idx.logger.Printf("Re-indexed: %s", filePath)
}

// Lookup finds entries by exact name.
func (idx *IndexStore) Lookup(name string) []Entry {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()
	return idx.entries[name]
}

// PrefixSearch finds entries by prefix using prefix tree.
func (idx *IndexStore) PrefixSearch(prefix string) []Entry {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()
	return idx.prefixTree.Search(prefix)
}

// GetFileEntries returns entries for a specific file.
func (idx *IndexStore) GetFileEntries(filePath string) []Entry {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()
	return idx.uriToEntries[filePath]
}

// GetFileSymbols returns legacy SymbolEntry slice for a file.
func (idx *IndexStore) GetFileSymbols(filePath string) []SymbolEntry {
	entries := idx.GetFileEntries(filePath)
	result := make([]SymbolEntry, len(entries))
	for i, e := range entries {
		result[i] = EntryToSymbolEntry(e)
	}
	return result
}

// LookupLegacy wraps Lookup as SymbolEntry for backward compat.
func (idx *IndexStore) LookupLegacy(name string) []SymbolEntry {
	entries := idx.Lookup(name)
	result := make([]SymbolEntry, len(entries))
	for i, e := range entries {
		result[i] = EntryToSymbolEntry(e)
	}
	return result
}

// LookupMethod returns methods declared on a receiver, using the receiver's
// fully-qualified name when available and falling back to its short name.
func (idx *IndexStore) LookupMethod(receiver, method string) []SymbolEntry {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()

	var result []SymbolEntry
	seen := make(map[string]bool)
	for _, entries := range idx.entries {
		for _, raw := range entries {
			entry, ok := raw.(MethodEntry)
			if !ok || entry.Name != method {
				continue
			}
			owner := entry.Owner
			if owner != receiver && shortName(owner) != shortName(receiver) {
				continue
			}
			converted := EntryToSymbolEntry(entry)
			key := converted.FilePath + ":" + converted.FullyQualifiedName
			if !seen[key] {
				seen[key] = true
				result = append(result, converted)
			}
		}
	}
	return result
}

// PrefixSearchLegacy wraps PrefixSearch as SymbolEntry.
func (idx *IndexStore) PrefixSearchLegacy(prefix string) []SymbolEntry {
	entries := idx.PrefixSearch(prefix)
	result := make([]SymbolEntry, len(entries))
	for i, e := range entries {
		result[i] = EntryToSymbolEntry(e)
	}
	return result
}

// LinearizedAncestors returns ancestor chain for a class/module FQN.
func (idx *IndexStore) LinearizedAncestors(fqn string) []string {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()

	visited := make(map[string]bool)
	var result []string
	var walk func(string)
	walk = func(name string) {
		if visited[name] {
			return
		}
		visited[name] = true
		if ancestors, ok := idx.ancestors[name]; ok {
			for _, a := range ancestors {
				result = append(result, a)
				walk(a)
			}
		}
		if mixins, ok := idx.mixins[name]; ok {
			for _, m := range mixins {
				result = append(result, m.Name)
				walk(m.Name)
			}
		}
	}
	walk(fqn)
	return result
}

// LookupByConvention resolves CamelCase names to Rails file paths.
func (idx *IndexStore) LookupByConvention(word string) []SymbolEntry {
	if entries := idx.LookupLegacy(word); len(entries) > 0 {
		return entries
	}

	snakeName := camelToSnake(word)
	conventionPaths := []string{
		filepath.Join(idx.workspaceRoot, "app", "models", snakeName+".rb"),
		filepath.Join(idx.workspaceRoot, "app", "controllers", snakeName+"_controller.rb"),
		filepath.Join(idx.workspaceRoot, "app", "services", snakeName+".rb"),
		filepath.Join(idx.workspaceRoot, "lib", snakeName+".rb"),
	}

	var results []SymbolEntry
	for _, p := range conventionPaths {
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
	return results
}

// AllEntries returns all indexed entries (for reference finding).
func (idx *IndexStore) AllEntries() map[string][]Entry {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()
	copy := make(map[string][]Entry, len(idx.entries))
	for k, v := range idx.entries {
		copy[k] = v
	}
	return copy
}

// WorkspaceRoot returns the workspace root path.
func (idx *IndexStore) WorkspaceRoot() string {
	return idx.workspaceRoot
}

// ParseFile parses a file and returns legacy SymbolEntry without modifying the index.
func (idx *IndexStore) ParseFile(filePath string) []SymbolEntry {
	entries, err := idx.parseFileOnly(filePath, "")
	if err != nil {
		idx.logParseFailure(filePath, err)
		return nil
	}
	result := make([]SymbolEntry, len(entries))
	for i, e := range entries {
		result[i] = EntryToSymbolEntry(e)
	}
	return result
}

// parseFileOnly parses without adding to the index store.
func (idx *IndexStore) parseFileOnly(filePath, source string) (entries []Entry, err error) {
	// This boundary includes ERB extraction and AST visitation, before publishing
	// any entries. Recovery occurs in the worker that is parsing this file.
	defer parser.Recover(&err)
	if source == "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
		source = string(data)
	}

	if strings.EqualFold(filepath.Ext(filePath), ".erb") {
		scanner := parser.NewERBScanner(source)
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		source = scanner.RubyContent()
	}

	parseResult, err := idx.parseSource(source)
	if err != nil {
		return nil, err
	}
	if parseResult == nil || parseResult.AST == nil {
		return nil, fmt.Errorf("parser returned no AST")
	}

	visitor := NewDeclarationVisitor(filePath)
	return visitor.Visit(parseResult.AST), nil
}

// GetAST returns nil (AST is parsed on demand in documents package).
func (idx *IndexStore) GetAST(_ string) interface{} {
	return nil
}

// Format debug info.
func (idx *IndexStore) String() string {
	idx.mutex.RLock()
	defer idx.mutex.RUnlock()
	return fmt.Sprintf("IndexStore{entries=%d, files=%d, ready=%v}", len(idx.entries), len(idx.uriToEntries), idx.ready)
}
