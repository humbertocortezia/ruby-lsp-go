package indexer

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIssue9IndexingPreservesDeclarationsAndPositions(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "parser", "testdata", "issue9.rb"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	dir := t.TempDir()
	file := filepath.Join(dir, "issue9.rb")
	writeSource(t, file, source)
	var logs bytes.Buffer
	idx := NewIndexStore(dir, log.New(&logs, "", 0))
	assertIndex := func() {
		t.Helper()
		for _, symbol := range []string{"BeforeIssue9", "to_s", "AfterIssue9", "AfterIssue9#indexed_after_literals"} {
			entries := idx.LookupLegacy(symbol)
			if len(entries) != 1 {
				t.Fatalf("expected exactly one %s declaration, got %v; logs:\n%s", symbol, entries, &logs)
			}
			entry := entries[0]
			lines := strings.Split(source, "\n")
			if entry.FilePath != file || entry.Line <= 0 || entry.Line > len(lines) {
				t.Fatalf("incorrect source location for %s: %+v", symbol, entry)
			}
			declaration := lines[entry.Line-1]
			if !strings.Contains(declaration, entry.Name) || strings.Index(declaration, entry.Name) != entry.Character {
				t.Fatalf("%s points to the wrong declaration: %+v, line=%q", symbol, entry, declaration)
			}
		}
		if strings.Contains(logs.String(), "Failed to index") || strings.Contains(logs.String(), "Parsing stack") {
			t.Fatalf("valid Ruby was reported as a failure:\n%s", &logs)
		}
	}
	idx.BuildIndex()
	assertIndex()

	// Changing the file externally must replace the positions, not leave the
	// old declarations or silently omit the file as an alleged parse failure.
	source = "# external edit\n\n" + source
	writeSource(t, file, source)
	idx.UpdateFile(file)
	assertIndex()
	idx.Rebuild()
	assertIndex()
}
