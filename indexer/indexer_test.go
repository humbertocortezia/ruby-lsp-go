package indexer

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindLocalVariableDefinitionPrefersMethodLocal(t *testing.T) {
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

	entry, ok := FindLocalVariableDefinition(source, 13, "some_text")
	if !ok {
		t.Fatal("expected local variable definition")
	}
	if entry.Type != SymbolLocalVariable || entry.Line != 13 || entry.Parent != "my_demo" {
		t.Fatalf("unexpected local variable entry: %+v", entry)
	}
}

func TestGetReceiverAtPosition(t *testing.T) {
	source := "MyClass.new()\n"
	character := strings.Index(source, "new")

	receiver, ok := GetReceiverAtPosition(source, 0, character)
	if !ok || receiver != "MyClass" {
		t.Fatalf("expected MyClass receiver, got %q (ok=%v)", receiver, ok)
	}
}

func TestLookupMethodByReceiver(t *testing.T) {
	directory := t.TempDir()
	filePath := filepath.Join(directory, "demo.rb")
	source := "class MyClass\n  def initialize\n  end\nend\n"
	if err := os.WriteFile(filePath, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}

	idx := New(directory, log.Default())
	entries := idx.ParseFile(filePath)
	idx.mutex.Lock()
	for _, entry := range entries {
		idx.symbols[entry.Name] = append(idx.symbols[entry.Name], entry)
	}
	idx.mutex.Unlock()

	methods := idx.LookupMethod("MyClass", "initialize")
	if len(methods) != 1 || methods[0].FilePath != filePath {
		t.Fatalf("unexpected method lookup: %+v", methods)
	}
}
