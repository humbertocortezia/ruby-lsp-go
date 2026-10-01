package indexer

import (
	"bytes"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/humberto/ruby-lsp-go/parser"
)

func writeSource(t *testing.T, path, source string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
}

func completeIndexWork(t *testing.T, work func()) {
	t.Helper()
	done := make(chan struct{})
	go func() { work(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("indexing did not complete")
	}
}

func TestIndexingContainsFileFailures(t *testing.T) {
	for _, mode := range []string{"incomplete", "panic"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			bad := filepath.Join(dir, "a_bad.rb")
			good := filepath.Join(dir, "z_good.rb")
			const secret = "private file content"
			writeSource(t, bad, "class Unpublished\nend\n\""+secret)
			writeSource(t, good, "class Survives\nend\n")
			var logs bytes.Buffer
			idx := NewIndexStore(dir, log.New(&logs, "", 0))
			if mode == "panic" {
				// Per-instance injection is configured before the worker starts;
				// there is no global hook or production environment switch.
				idx.parseSource = func(source string) (*parser.ParseResult, error) {
					if strings.Contains(source, secret) {
						panic(source)
					}
					return parser.ParseSource(source)
				}
			}
			for _, work := range []func(){idx.BuildIndex, idx.Rebuild} {
				logs.Reset()
				completeIndexWork(t, work)
				if !idx.IsReady() || len(idx.Lookup("Survives")) != 1 {
					t.Fatal("workspace did not finish indexing valid file")
				}
				if len(idx.Lookup("Unpublished")) != 0 || len(idx.GetFileEntries(bad)) != 0 {
					t.Fatal("published partial entries")
				}
				if !strings.Contains(logs.String(), bad) || !strings.Contains(logs.String(), "Failed to index") {
					t.Fatalf("missing file failure: %s", &logs)
				}
				if strings.Contains(logs.String(), secret) {
					t.Fatal("source leaked into logs")
				}
				if mode == "panic" && !strings.Contains(logs.String(), "Parsing stack") {
					t.Fatal("missing panic stack")
				}
			}
			if entries := idx.ParseFile(bad); len(entries) != 0 {
				t.Fatal("ParseFile published failed entries")
			}
		})
	}
}

func TestUpdateFailureRemovesOldEntriesAndRecovers(t *testing.T) {
	for _, mode := range []string{"incomplete", "panic", "read-error"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "changing.rb")
			var logs bytes.Buffer
			idx := NewIndexStore(dir, log.New(&logs, "", 0))
			if mode == "panic" {
				idx.parseSource = func(source string) (*parser.ParseResult, error) {
					if source == `"unfinished` {
						panic("injected failure")
					}
					return parser.ParseSource(source)
				}
			}
			writeSource(t, path, "class Before\nend\n")
			idx.BuildIndex()
			if len(idx.Lookup("Before")) != 1 {
				t.Fatal("missing initial entry")
			}
			writeSource(t, path, `"unfinished`)
			if mode == "read-error" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			logs.Reset()
			completeIndexWork(t, func() { idx.UpdateFile(path) })
			if len(idx.Lookup("Before")) != 0 || len(idx.GetFileEntries(path)) != 0 || len(idx.PrefixSearch("Before")) != 0 {
				t.Fatal("failed update left stale entries")
			}
			if !strings.Contains(logs.String(), "Failed to index") || strings.Contains(logs.String(), "Re-indexed:") {
				t.Fatalf("failed update logged as success: %s", &logs)
			}
			writeSource(t, path, "class After\nend\n")
			for i := 0; i < 2; i++ {
				completeIndexWork(t, func() { idx.UpdateFile(path) })
			}
			if len(idx.Lookup("After")) != 1 || len(idx.GetFileEntries(path)) != 1 || len(idx.PrefixSearch("After")) != 1 {
				t.Fatal("recovery lost or duplicated symbols")
			}
		})
	}
}

func TestParseFilePanicReturnsErrorWithoutEntries(t *testing.T) {
	idx := NewIndexStore(t.TempDir(), log.Default())
	idx.parseSource = func(string) (*parser.ParseResult, error) { panic("injected") }
	entries, err := idx.parseFileOnly("test.rb", "nonempty")
	var failure *parser.PanicError
	if entries != nil || !errors.As(err, &failure) {
		t.Fatalf("expected nil entries and panic error, got %v, %v", entries, err)
	}
}

func TestIndexFileReplacementPreservesQuerySnapshots(t *testing.T) {
	dir := t.TempDir()
	first, second := filepath.Join(dir, "first.rb"), filepath.Join(dir, "second.rb")
	idx := NewIndexStore(dir, log.Default())
	idx.IndexFile(first, "class Shared\nend\n")
	idx.IndexFile(second, "class Shared\nend\n")
	snapshot := idx.Lookup("Shared")
	if len(snapshot) != 2 {
		t.Fatalf("expected both declarations, got %v", snapshot)
	}
	firstURI := snapshot[0].GetLocation().URI
	idx.IndexFile(first, "class Replacement\nend\n")
	idx.IndexFile(first, "class Replacement\nend\n")
	if snapshot[0].GetLocation().URI != firstURI {
		t.Fatal("update mutated a previous query's snapshot")
	}
	if len(idx.Lookup("Shared")) != 1 || len(idx.Lookup("Replacement")) != 1 {
		t.Fatal("replacement duplicated or lost entries")
	}
}
