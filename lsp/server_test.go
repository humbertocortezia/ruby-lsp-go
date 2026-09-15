package lsp

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/humberto/ruby-lsp-go/indexer"
	"github.com/humberto/ruby-lsp-go/store"
)

func TestHandleDefinitionResolvesLocalVariableBeforeMethod(t *testing.T) {
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
	if err := os.WriteFile(filePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	server, uri := testServerForFile(t, directory, filePath, source)
	character := strings.Index("    print_val(some_text)", "some_text")
	result := server.HandleDefinition(definitionParams(uri, 13, character))

	locations := result.([]interface{})
	if len(locations) != 1 {
		t.Fatalf("expected one local definition, got %d: %+v", len(locations), locations)
	}
	location := locations[0].(map[string]interface{})
	rangeValue := location["range"].(map[string]interface{})
	start := rangeValue["start"].(map[string]interface{})
	if start["line"] != 12 || start["character"] != 4 {
		t.Fatalf("unexpected local definition location: %+v", start)
	}
}

func TestHandleDefinitionResolvesNewToInitialize(t *testing.T) {
	directory := t.TempDir()
	filePath := filepath.Join(directory, "demo.rb")
	source := "class MyClass\n  def initialize\n  end\nend\n\nMyClass.new\n"
	if err := os.WriteFile(filePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	server, uri := testServerForFile(t, directory, filePath, source)
	character := strings.Index("MyClass.new", "new")
	result := server.HandleDefinition(definitionParams(uri, 5, character))

	locations := result.([]interface{})
	if len(locations) != 1 {
		t.Fatalf("expected initialize definition, got %d: %+v", len(locations), locations)
	}
	location := locations[0].(map[string]interface{})
	rangeValue := location["range"].(map[string]interface{})
	start := rangeValue["start"].(map[string]interface{})
	if start["line"] != 1 {
		t.Fatalf("expected initialize on line 2, got %+v", start)
	}
}

func testServerForFile(t *testing.T, directory string, filePath string, source string) (*Server, string) {
	t.Helper()
	logger := log.New(os.Stderr, "test: ", 0)
	globalState := &GlobalState{WorkspacePath: directory}
	server := &Server{
		GlobalState: globalState,
		Store:       store.New(globalState),
		Indexer:     indexer.New(directory, logger),
		Logger:      logger,
	}
	server.Indexer.(*indexer.Index).BuildIndex()

	uri := pathToURI(filePath)
	server.Store.(*store.Store).Set(uri, source, 1, "ruby")
	return server, uri
}

func definitionParams(uri string, line int, character int) map[string]interface{} {
	return map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position": map[string]interface{}{
			"line":      float64(line),
			"character": float64(character),
		},
	}
}
