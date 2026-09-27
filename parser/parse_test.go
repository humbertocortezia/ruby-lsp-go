package parser

import "testing"

func TestParseSimpleClass(t *testing.T) {
	src := `class Foo
  def bar
    42
  end
end`
	ast, err := Parse(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ast == nil {
		t.Fatal("expected non-nil AST")
	}
	if ast.Type != NodeProgram {
		t.Errorf("expected root node, got %v", ast.Type)
	}
	if len(ast.Children) == 0 {
		t.Fatal("expected at least one child")
	}
	if ast.Children[0].Type != NodeClass {
		t.Errorf("expected first child to be class, got %v", ast.Children[0].Type)
	}
	if ast.Children[0].Name != "Foo" {
		t.Errorf("expected class name 'Foo', got %q", ast.Children[0].Name)
	}
}

func TestParseWithAssociations(t *testing.T) {
	src := `class GTipoUnidadeGestora < ApplicationRecord
  belongs_to :g_unidade_gestora
  has_many :p_planos
  scope :ativos, -> { where(ativo: true) }
end`
	ast, err := Parse(src)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ast == nil {
		t.Fatal("expected non-nil AST")
	}
	if len(ast.Children) == 0 {
		t.Fatal("expected at least one child")
	}
	classNode := ast.Children[0]
	if classNode.Type != NodeClass {
		t.Fatalf("expected class node, got %v", classNode.Type)
	}
	if classNode.Name != "GTipoUnidadeGestora" {
		t.Errorf("expected name 'GTipoUnidadeGestora', got %q", classNode.Name)
	}
}

func TestGetWordAtPosition(t *testing.T) {
	src := "class FooBar\n  def baz\n  end\nend"
	tests := []struct {
		name      string
		line, col int
		want      string
	}{
		{"on class name", 0, 7, "FooBar"},
		{"on method name", 1, 7, "baz"},
		{"on end", 2, 3, "end"},
		{"past end of line", 0, 100, "FooBar"},
		{"on trailing newline", 3, 0, "end"},
		{"before identifier", 0, 5, "class"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetWordAtPosition(src, tt.line, tt.col)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseSourceWrapper(t *testing.T) {
	res, err := ParseSource("class A; end")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.AST == nil {
		t.Fatal("expected non-nil result with AST")
	}
}
