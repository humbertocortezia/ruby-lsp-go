package indexer

import "testing"

func TestCamelToSnake(t *testing.T) {
	cases := []struct{ in, want string }{
		{"GTipoUnidadeGestora", "g_tipo_unidade_gestora"},
		{"User", "user"},
		{"HTTPClient", "http_client"},
		{"HTMLParser", "html_parser"},
		{"Foo::Bar", "foo/bar"},
		{"A", "a"},
		{"", ""},
		{"UserGroup", "user_group"},
	}
	for _, c := range cases {
		got := camelToSnake(c.in)
		if got != c.want {
			t.Errorf("camelToSnake(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestCapitalize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"foo", "Foo"},
		{"g_unidade", "G_unidade"},
		{"Foo", "Foo"},
		{"", ""},
		{"x", "X"},
		{"1foo", "1foo"},
	}
	for _, c := range cases {
		got := capitalize(c.in)
		if got != c.want {
			t.Errorf("capitalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsCapitalized(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"Foo", true},
		{"foo", false},
		{"", false},
		{"1foo", false},
		{"G", true},
	}
	for _, c := range cases {
		got := isCapitalized(c.in)
		if got != c.want {
			t.Errorf("isCapitalized(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestSymbolKindToLSP(t *testing.T) {
	if got := SymbolKindToLSP(SymbolClass); got == 0 {
		t.Errorf("SymbolClass should map to a non-zero LSP SymbolKind")
	}
	if got := SymbolKindToLSP(SymbolMethod); got == 0 {
		t.Errorf("SymbolMethod should map to a non-zero LSP SymbolKind")
	}
}

func TestCompletionKindFromType(t *testing.T) {
	if got := CompletionKindFromType(SymbolClass); got == 0 {
		t.Errorf("SymbolClass should map to a non-zero CompletionItemKind")
	}
}

func TestSymbolTypeString(t *testing.T) {
	if SymbolTypeString(SymbolClass) != "class" {
		t.Errorf("expected 'class' for SymbolClass, got %q", SymbolTypeString(SymbolClass))
	}
	if SymbolTypeString(SymbolMethod) != "method" {
		t.Errorf("expected 'method' for SymbolMethod, got %q", SymbolTypeString(SymbolMethod))
	}
}
