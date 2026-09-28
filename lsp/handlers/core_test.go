package handlers

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/humberto/ruby-lsp-go/indexer"
	"github.com/humberto/ruby-lsp-go/store"
	"github.com/humberto/ruby-lsp-go/workspace"
)

func TestDefinitionResolvesLocalVariableBeforeMethod(t *testing.T) {
	directory := t.TempDir()
	filePath := filepath.Join(directory, "demo.rb")
	source := `class Demo1
  def some_text
    "Goodbye World"
  end
end

class Demo2
  def print_val(val)
    puts val
  end

  def my_demo
    some_text = "Hello World"
    print_val(some_text)
  end
end
`
	writeTestFile(t, filePath, source)
	ctx, uri := testContext(t, directory, filePath, source)
	character := strings.Index("    print_val(some_text)", "some_text")
	result := Definition(ctx, definitionParams(uri, 13, character))

	locations := result.([]interface{})
	if len(locations) != 1 {
		t.Fatalf("expected one local definition, got %d: %+v", len(locations), locations)
	}
	start := locations[0].(map[string]interface{})["range"].(map[string]interface{})["start"].(map[string]interface{})
	if start["line"] != 12 || start["character"] != 4 {
		t.Fatalf("unexpected local definition location: %+v", start)
	}
}

func TestDefinitionResolvesNewToInitialize(t *testing.T) {
	directory := t.TempDir()
	filePath := filepath.Join(directory, "demo.rb")
	source := "class MyClass\n  def initialize\n  end\nend\n\nMyClass.new\n"
	writeTestFile(t, filePath, source)
	ctx, uri := testContext(t, directory, filePath, source)
	character := strings.Index("MyClass.new", "new")
	result := Definition(ctx, definitionParams(uri, 5, character))

	locations := result.([]interface{})
	if len(locations) != 1 {
		t.Fatalf("expected initialize definition, got %d: %+v", len(locations), locations)
	}
	start := locations[0].(map[string]interface{})["range"].(map[string]interface{})["start"].(map[string]interface{})
	if start["line"] != 1 {
		t.Fatalf("expected initialize on line 2, got %+v", start)
	}
}

func testContext(t *testing.T, directory, filePath, source string) (*Context, string) {
	t.Helper()
	logger := log.New(os.Stderr, "test: ", 0)
	state := &workspace.State{WorkspacePath: directory, EnabledFeatures: map[string]bool{}}
	docStore := store.New(state)
	idx := indexer.NewIndexStore(directory, logger)
	idx.IndexFile(filePath, source)
	uri := "file://" + filePath
	docStore.SetDocument(uri, source, 1, "ruby")
	return &Context{State: state, Store: docStore, Index: idx, Logger: logger}, uri
}

func writeTestFile(t *testing.T, filePath, source string) {
	t.Helper()
	if err := os.WriteFile(filePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
}

func definitionParams(uri string, line, character int) map[string]interface{} {
	return map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position": map[string]interface{}{
			"line":      float64(line),
			"character": float64(character),
		},
	}
}
