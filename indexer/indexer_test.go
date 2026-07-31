package indexer

import (
	"io"
	"log"
	"strings"
	"testing"
)

func testIndex() *Index { return New("/workspace", log.New(io.Discard, "", 0)) }

func lineAndColumn(source, text string) (int, int) {
	offset := strings.Index(source, text)
	if offset < 0 {
		return -1, -1
	}
	line := strings.Count(source[:offset], "\n")
	lineStart := strings.LastIndex(source[:offset], "\n") + 1
	return line, len([]rune(source[lineStart:offset]))
}

func TestDefinitionCandidatesResolvesNewToInitialize(t *testing.T) {
	source := "class MyClass\n  def initialize\n  end\nend\n\nMyClass.new\n"
	idx := testIndex()
	path := "/workspace/app/models/my_class.rb"
	idx.UpdateSource(path, source)
	line, column := lineAndColumn(source, "new")
	entries := idx.DefinitionCandidates(path, source, line, column)
	if len(entries) != 1 {
		t.Fatalf("expected one constructor definition, got %#v", entries)
	}
	if entries[0].Name != "initialize" || entries[0].Parent != "MyClass" {
		t.Fatalf("expected MyClass#initialize, got %#v", entries[0])
	}
}

func TestDefinitionCandidatesPrefersLocalVariableOverMethod(t *testing.T) {
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
	idx := testIndex()
	path := "/workspace/demo.rb"
	idx.UpdateSource(path, source)
	line, column := lineAndColumn(source, "some_text)\n")
	entries := idx.DefinitionCandidates(path, source, line, column)
	if len(entries) != 1 {
		t.Fatalf("expected one local definition, got %#v", entries)
	}
	if entries[0].Type != SymbolLocalVariable || entries[0].Line != 13 {
		t.Fatalf("expected local assignment on line 13, got %#v", entries[0])
	}
}

func TestERBSourceAndFoldingRanges(t *testing.T) {
	source := `<h1>Hello</h1>
<% class Greeting %>
  <% if user %>
    <%= user.name %>
  <% end %>
<% end %>
`
	idx := testIndex()
	entries := idx.ParseSource("/workspace/views/greeting.html.erb", source)
	if len(entries) != 1 || entries[0].Name != "Greeting" {
		t.Fatalf("expected the Ruby class inside ERB to be indexed, got %#v", entries)
	}
	ranges := FoldingRanges(source, "erb")
	if len(ranges) != 2 {
		t.Fatalf("expected class and if folds, got %#v", ranges)
	}
	if ranges[0].StartLine != 1 || ranges[0].EndLine != 4 {
		t.Fatalf("unexpected class fold: %#v", ranges[0])
	}
}

func TestERBFoldingIncludesHTMLTags(t *testing.T) {
	source := `<html>
<head>
  <style>
    body { color: red; }
  </style>
</head>
<body>
  <div>
    <%= yield %>
  </div>
</body>
</html>
`
	ranges := FoldingRanges(source, "erb")
	contains := func(start, end int) bool {
		for _, folding := range ranges {
			if folding.StartLine == start && folding.EndLine == end {
				return true
			}
		}
		return false
	}
	if !contains(0, 10) || !contains(1, 4) || !contains(6, 9) || !contains(7, 8) {
		t.Fatalf("expected HTML folds for html/head/body/div, got %#v", ranges)
	}
}

func TestIndexUpdateRemovesStaleEntries(t *testing.T) {
	idx := testIndex()
	path := "/workspace/a.rb"
	idx.UpdateSource(path, "class OldName\nend\n")
	if len(idx.Lookup("OldName")) != 1 {
		t.Fatal("expected initial symbol")
	}
	idx.UpdateSource(path, "# now empty\n")
	if len(idx.Lookup("OldName")) != 0 {
		t.Fatalf("stale symbol remained: %#v", idx.Lookup("OldName"))
	}
	if len(idx.GetFileSymbols(path)) != 0 {
		t.Fatal("stale file symbols remained")
	}
}
