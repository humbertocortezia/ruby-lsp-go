package parser

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Compile only: no fixture or corpus Ruby code is executed. Ruby is a test
// oracle, not a runtime dependency of the language server.
func TestBoundaryCasesAcceptedByRuby(t *testing.T) {
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("Ruby is required for syntax-oracle validation")
	}
	for _, source := range boundaryCases() {
		cmd := exec.Command("ruby", "-c")
		cmd.Stdin = strings.NewReader(source)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("invalid regression fixture %q: %v\n%s", source, err, output)
		}
	}
}

// Opt-in acceptance check against an installed standard library or real
// workspace: RUBY_LSP_GO_CORPUS=/path/to/ruby/files go test ./parser -run TestRubyCorpus -v.
// It detects false rejections of valid Ruby, complementing the crash-only fuzz
// invariant. Curated declaration/position tests remain necessary for AST quality.
func TestRubyCorpus(t *testing.T) {
	root := os.Getenv("RUBY_LSP_GO_CORPUS")
	if root == "" {
		t.Skip("set RUBY_LSP_GO_CORPUS to validate a Ruby corpus")
	}
	var files []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && filepath.Ext(path) == ".rb" {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("corpus contains no Ruby files")
	}
	input, err := json.Marshal(files)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ruby", "-rjson", "-e", `files = JSON.parse(STDIN.read); valid = []; invalid = []; files.each { |path| begin; RubyVM::InstructionSequence.compile_file(path); valid << path; rescue SyntaxError => error; invalid << [path, error.message]; end }; STDOUT.write(JSON.generate({valid: valid, invalid: invalid}))`)
	cmd.Stdin = strings.NewReader(string(input))
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Ruby corpus syntax check failed: %v", err)
	}
	var corpus struct {
		Valid   []string
		Invalid [][]string
	}
	if err := json.Unmarshal(output, &corpus); err != nil {
		t.Fatal(err)
	}
	t.Logf("Ruby confirmed %d valid files; %d rejected by Ruby itself", len(corpus.Valid), len(corpus.Invalid))
	if len(corpus.Valid) == 0 {
		t.Fatal("Ruby accepted no corpus files")
	}
	for _, path := range corpus.Valid {
		t.Run(strings.TrimPrefix(path, root), func(t *testing.T) {
			source, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			result, err := ParseSource(string(source))
			if err != nil || result == nil {
				t.Fatalf("valid Ruby rejected (%s): %v", path, err)
			}
		})
	}
}
