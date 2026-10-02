package indexer

import (
	"bytes"
	"log"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndexRubyAndERBWithOpaqueContent(t *testing.T) {
	for _, tc := range []struct{ filename, source string }{
		{"literals.rb", "=begin\nIt's read/write { class Fake\n=end\nclass Real\n  def banner\n    <<~TEXT.chomp\nIt's read/write } class Fake\n    TEXT\n  end\n  def separator?(c)\n    c == ?'\n  end\nend\n"},
		{"example.init.erb", "<%# It's read/write { class Fake %>\n#!/bin/sh\necho \"can't start <%= @home %>/bin\"\n<%% class Fake; end %>\n<% class Real %>\n<% def banner; end %>\n<% end %>\n"},
	} {
		t.Run(tc.filename, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, tc.filename)
			source := tc.source
			writeSource(t, path, source)
			var logs bytes.Buffer
			idx := NewIndexStore(dir, log.New(&logs, "", 0))
			assertIndex := func() {
				for _, symbol := range []string{"Real", "Real#banner"} {
					entries := idx.LookupLegacy(symbol)
					if len(entries) != 1 {
						t.Fatalf("missing %s: %v; logs=%s", symbol, entries, &logs)
					}
					entry := entries[0]
					line := strings.Split(source, "\n")[entry.Line-1]
					if entry.Character != strings.Index(line, entry.Name) {
						t.Fatalf("wrong position for %s: %+v", symbol, entry)
					}
				}
				if len(idx.LookupLegacy("Fake")) != 0 || strings.Contains(logs.String(), "Failed to index") {
					t.Fatalf("opaque content indexed or rejected: %s", &logs)
				}
			}
			idx.BuildIndex()
			assertIndex()
			source = "\n\n" + source
			writeSource(t, path, source)
			idx.UpdateFile(path)
			assertIndex()
			idx.Rebuild()
			assertIndex()
			writeSource(t, path, "<% class Broken")
			if strings.HasSuffix(path, ".erb") {
				idx.UpdateFile(path)
				if len(idx.LookupLegacy("Real")) != 0 || !strings.Contains(logs.String(), "unterminated ERB tag") {
					t.Fatalf("failed ERB extraction was hidden: %s", &logs)
				}
			}
		})
	}
}
