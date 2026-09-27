package handlers

import "testing"

// The fixture mirrors the user's reported file:
//   - 12 lines of leading `#` comments
//   - one `class ... end` block
//   - several `belongs_to` lines
// The previous heuristic counted `end` as a "close" but did not count
// `class` as an "open", so it reported a false positive on every model.
func TestCountUnbalancedBraces_NoFalsePositiveOnModel(t *testing.T) {
	src := `# frozen_string_literal: true

# Associações:
# e_ano_serie
# e_plano_curso_area
# g_unidade_gestora

# Atributos:
# text - referencias
# text - avaliacoes

# Adicione aqui quaisquer métodos ou validações abaixo
class EAnoSeriePlanoCursoArea < ApplicationRecord
  belongs_to :e_ano_serie
  belongs_to :e_plano_curso_area
  belongs_to :g_unidade_gestora
end
`
	if unbalanced, which := countUnbalancedBraces(src); unbalanced {
		t.Errorf("false positive on a valid Rails model: reported unbalanced %q", which)
	}
}

func TestCountUnbalancedBraces_DetectsMissingClose(t *testing.T) {
	// `bar(` opens a paren that is never closed: opensParens=1, closesParens=0
	// → the missing side is the open paren.
	src := `def foo
  bar(
end
`
	if unbalanced, which := countUnbalancedBraces(src); !unbalanced {
		t.Errorf("expected to detect unbalanced delimiter, got false")
	} else if which != "(" {
		t.Errorf("expected to flag %q, got %q", "(", which)
	}
}

func TestCountUnbalancedBraces_DetectsExtraClose(t *testing.T) {
	// `end)` adds a `)` that has no matching `(`: closesParens=1, opensParens=0
	// → the extra side is the close paren.
	src := `def foo
end)
`
	if unbalanced, which := countUnbalancedBraces(src); !unbalanced {
		t.Errorf("expected to detect unbalanced delimiter, got false")
	} else if which != ")" {
		t.Errorf("expected to flag %q, got %q", ")", which)
	}
}

func TestCountUnbalancedBraces_IgnoresComments(t *testing.T) {
	// A comment with mismatched-looking tokens must not trigger the
	// diagnostic. The previous implementation only stripped `# ... \n`,
	// so this still passed — but the regression test pins the behavior.
	src := `# def foo
# end
# (unbalanced
class A
end
`
	if unbalanced, which := countUnbalancedBraces(src); unbalanced {
		t.Errorf("comments should be ignored, got %q", which)
	}
}

func TestCountUnbalancedBraces_IgnoresStrings(t *testing.T) {
	src := `s = "{ not a block }"
class A
end
`
	if unbalanced, which := countUnbalancedBraces(src); unbalanced {
		t.Errorf("strings should be ignored, got %q", which)
	}
}

func TestCountUnbalancedBraces_HandlesBlocks(t *testing.T) {
	src := `[1, 2, 3].each do |i|
  puts i
end
`
	if unbalanced, which := countUnbalancedBraces(src); unbalanced {
		t.Errorf("do/end blocks should not trigger, got %q", which)
	}
}
