package indexer

import (
	"bytes"
	"log"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndexBacktickMethods(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "shell.rb")
	source := "class Shell\n  def `(cmd)\n    super\n  end\n  def self.`(cmd)\n    super\n  end\nend\nobj&.`(\"echo hi\")\nalias_method :old_backtick, :`\nclass AfterCommand\n  def indexed\n    `echo #{\"hi\"}`\n  end\nend\ndef OtherShell::`(cmd)\n  super\nend\n"
	writeSource(t, path, source)
	var logs bytes.Buffer
	idx := NewIndexStore(dir, log.New(&logs, "", 0))
	assertIndex := func() {
		t.Helper()
		for _, key := range []string{"Shell#`", "Shell.`", "OtherShell.`", "AfterCommand", "AfterCommand#indexed"} {
			entries := idx.LookupLegacy(key)
			if len(entries) != 1 {
				t.Fatalf("missing %s: %v; logs=%s", key, entries, &logs)
			}
			entry := entries[0]
			line := strings.Split(source, "\n")[entry.Line-1]
			if entry.Character != strings.Index(line, entry.Name) {
				t.Fatalf("wrong declaration position for %s: %+v", key, entry)
			}
		}
		if strings.Contains(logs.String(), "Failed to index") {
			t.Fatalf("valid methods rejected: %s", &logs)
		}
	}
	idx.BuildIndex()
	assertIndex()
	source = "# external edit\n\n" + source
	writeSource(t, path, source)
	idx.UpdateFile(path)
	assertIndex()
	idx.Rebuild()
	assertIndex()

	writeSource(t, path, source+"result = `unterminated")
	idx.UpdateFile(path)
	if len(idx.LookupLegacy("Shell#`")) != 0 || !strings.Contains(logs.String(), "unterminated string") {
		t.Fatalf("genuine lexical failure hidden or stale symbols retained: %s", &logs)
	}
}
