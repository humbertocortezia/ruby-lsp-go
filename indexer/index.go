package indexer

import (
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
	entries      map[string][]Entry
	uriToEntries map[string][]Entry
	prefixTree   *PrefixTree
	ancestors    map[string][]string
	mixins       map[string][]MixinEntry
	mutex        sync.RWMutex
	workspaceRoot string
	logger       *log.Logger
	ready        bool
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

// IndexFile parses and indexes a single file.
func (idx *IndexStore) IndexFile(filePath, source string) []Entry {
	entries := idx.parseFileOnly(filePath, source)
	if len(entries) > 0 {
		idx.addEntries(filePath, entries)
	}
	return entries
}

func (idx *IndexStore) addEntries(filePath string, entries []Entry) {
	idx.mutex.Lock()
	defer idx.mutex.Unlock()

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
	filtered := entries[:0]
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
	idx.DeleteFile(filePath)
	idx.IndexFile(filePath, "")
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
	entries := idx.parseFileOnly(filePath, "")
	result := make([]SymbolEntry, len(entries))
	for i, e := range entries {
		result[i] = EntryToSymbolEntry(e)
	}
	return result
}

// parseFileOnly parses without adding to the index store.
func (idx *IndexStore) parseFileOnly(filePath, source string) []Entry {
	if source == "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil
		}
		source = string(data)
	}

	if strings.HasSuffix(filePath, ".erb") {
		scanner := parser.NewERBScanner(source)
		source = scanner.RubyContent()
	}

	parseResult, err := parser.ParseSource(source)
	if err != nil || parseResult.AST == nil {
		return nil
	}

	visitor := NewDeclarationVisitor(filePath)
	return visitor.Visit(parseResult.AST)
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
