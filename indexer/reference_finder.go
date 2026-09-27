package indexer

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/humberto/ruby-lsp-go/lsp/types"
	"github.com/humberto/ruby-lsp-go/parser"
)

// ReferenceFinder finds all references to a symbol in the workspace.
type ReferenceFinder struct {
	index *IndexStore
}

// NewReferenceFinder creates a reference finder.
func NewReferenceFinder(index *IndexStore) *ReferenceFinder {
	return &ReferenceFinder{index: index}
}

// FindReferences finds all locations where a symbol is referenced.
func (rf *ReferenceFinder) FindReferences(name string, filePath string, includeDeclaration bool) []types.Location {
	var locations []types.Location

	// Search in indexed entries for declarations
	if includeDeclaration {
		for _, entry := range rf.index.Lookup(name) {
			locations = append(locations, entry.GetLocation())
		}
	}

	// Walk all workspace files for usages
	filepath.Walk(rf.index.workspaceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() && skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".rb" && ext != ".erb" {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		source := string(data)
		if ext == ".erb" {
			scanner := parser.NewERBScanner(source)
			source = scanner.RubyContent()
		}

		usages := findUsagesInSource(source, name, path)
		for _, u := range usages {
			if !includeDeclaration && isDeclaration(u, name) {
				continue
			}
			locations = append(locations, u)
		}
		return nil
	})

	return deduplicateLocations(locations)
}

// FindReferencesInDocument finds references within a single document.
func (rf *ReferenceFinder) FindReferencesInDocument(source, uri, name string, includeDeclaration bool) []types.Location {
	usages := findUsagesInSource(source, filePathFromURI(uri), name)
	if !includeDeclaration {
		var filtered []types.Location
		for _, u := range usages {
			if !isDeclaration(u, name) {
				filtered = append(filtered, u)
			}
		}
		return filtered
	}
	return usages
}

func findUsagesInSource(source, filePath, name string) []types.Location {
	var locations []types.Location
	lines := strings.Split(source, "\n")
	cleanName := strings.TrimPrefix(name, "@")
	cleanName = strings.TrimPrefix(cleanName, "@@")
	cleanName = strings.TrimPrefix(cleanName, "$")

	for lineIdx, line := range lines {
		searchFrom := 0
		for {
			idx := strings.Index(line[searchFrom:], name)
			if idx == -1 {
				// Also try without sigils for ivar/cvar
				if name != cleanName {
					idx = strings.Index(line[searchFrom:], cleanName)
				}
				if idx == -1 {
					break
				}
			}
			absIdx := searchFrom + idx
			charEnd := absIdx + len(name)
			if charEnd > len(line) {
				charEnd = len(line)
			}

			uri := fileURI(filePath)
			locations = append(locations, types.Location{
				URI: uri,
				Range: types.Range{
					Start: types.Position{Line: lineIdx, Character: absIdx},
					End:   types.Position{Line: lineIdx, Character: charEnd},
				},
			})
			searchFrom = absIdx + len(name)
		}
	}
	return locations
}

func isDeclaration(loc types.Location, name string) bool {
	// Simple heuristic: declarations typically appear at start of line after keywords
	return false
}

func deduplicateLocations(locs []types.Location) []types.Location {
	seen := make(map[string]bool)
	var result []types.Location
	for _, l := range locs {
		key := l.URI + ":" + itoa(l.Range.Start.Line) + ":" + itoa(l.Range.Start.Character)
		if !seen[key] {
			seen[key] = true
			result = append(result, l)
		}
	}
	return result
}

// PrepareRename checks if a symbol at position can be renamed.
func PrepareRename(source string, line, col int) (range_ *types.Range, placeholder string, ok bool) {
	word := parser.GetWordAtPosition(source, line, col)
	if word == "" {
		return nil, "", false
	}
	lines := strings.Split(source, "\n")
	if line >= len(lines) {
		return nil, "", false
	}
	lineText := lines[line]
	idx := strings.Index(lineText, word)
	if idx == -1 {
		return nil, "", false
	}
	return &types.Range{
		Start: types.Position{Line: line, Character: idx},
		End:   types.Position{Line: line, Character: idx + len(word)},
	}, word, true
}

// BuildRenameEdit creates a WorkspaceEdit renaming a symbol across all references.
func BuildRenameEdit(finder *ReferenceFinder, oldName, newName string) map[string][]types.TextEdit {
	refs := finder.FindReferences(oldName, "", true)
	changes := make(map[string][]types.TextEdit)
	for _, loc := range refs {
		edit := types.TextEdit{
			Range:   loc.Range,
			NewText: newName,
		}
		changes[loc.URI] = append(changes[loc.URI], edit)
	}
	return changes
}
