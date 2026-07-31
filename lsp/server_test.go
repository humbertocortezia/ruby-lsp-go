package lsp

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/humberto/ruby-lsp-go/indexer"
	"github.com/humberto/ruby-lsp-go/store"
)

func serverForTest(t *testing.T, source string, extension string) (*Server, string, string) {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "app", "models", "demo"+extension)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	idx := indexer.New(root, log.New(io.Discard, "", 0))
	idx.BuildIndex()
	uri := pathToURI(path)
	gs := &GlobalState{WorkspacePath: root, WorkspaceURI: pathToURI(root)}
	st := store.New(gs)
	st.Set(uri, source, 1, map[bool]string{true: "erb", false: "ruby"}[extension == ".erb"])
	return &Server{GlobalState: gs, Store: st, Indexer: idx, Logger: log.New(io.Discard, "", 0), CancelledRequests: map[int]bool{}}, uri, path
}

func lspPosition(uri string, line, character int) map[string]interface{} {
	return map[string]interface{}{"textDocument": map[string]interface{}{"uri": uri}, "position": map[string]interface{}{"line": float64(line), "character": float64(character)}}
}

func TestHandleDefinitionResolvesConstructor(t *testing.T) {
	source := "class Demo\n  def initialize\n  end\nend\n\nDemo.new\n"
	server, uri, _ := serverForTest(t, source, ".rb")
	line := 5
	character := 5
	result, ok := server.HandleDefinition(lspPosition(uri, line, character)).([]interface{})
	if !ok || len(result) != 1 {
		t.Fatalf("unexpected definition result: %#v", result)
	}
	location := result[0].(map[string]interface{})
	if location["uri"] != uri {
		t.Fatalf("expected %s, got %#v", uri, location["uri"])
	}
	if location["range"].(map[string]interface{})["start"].(map[string]interface{})["line"] != 1 {
		t.Fatalf("expected initialize on line 1, got %#v", location)
	}
}

func TestHandleDefinitionDoesNotUseOutOfScopeMethod(t *testing.T) {
	source := `class Demo1
  def some_text
  end
end

class Demo2
  def my_demo
    some_text = "local"
    puts some_text
  end
end
`
	server, uri, _ := serverForTest(t, source, ".rb")
	line := 8
	character := 16
	result, ok := server.HandleDefinition(lspPosition(uri, line, character)).([]interface{})
	if !ok || len(result) != 1 {
		t.Fatalf("unexpected definition result: %#v", result)
	}
	start := result[0].(map[string]interface{})["range"].(map[string]interface{})["start"].(map[string]interface{})
	if start["line"] != 7 {
		t.Fatalf("expected local assignment on line 7, got %#v", start)
	}
}

func TestHandleFoldingRangeAndOnTypeFormattingForERB(t *testing.T) {
	source := "<% if user %>\n  <%= user.name %>\n<% end %>\n"
	server, uri, _ := serverForTest(t, source, ".erb")
	folds, ok := server.HandleFoldingRange(map[string]interface{}{"textDocument": map[string]interface{}{"uri": uri}}).([]interface{})
	if !ok || len(folds) != 1 {
		t.Fatalf("unexpected ERB folding response: %#v", folds)
	}
	server.Store.(*store.Store).Set(uri, "<% if user %>\n", 1, "erb")
	edits, ok := server.HandleOnTypeFormatting(map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": float64(1), "character": float64(0)},
		"ch":           "\n",
	}).([]interface{})
	if !ok || len(edits) != 1 {
		t.Fatalf("unexpected on-type response: %#v", edits)
	}
}
