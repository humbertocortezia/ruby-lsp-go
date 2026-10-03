package indexer

import (
	"strings"

	"github.com/humberto/ruby-lsp-go/parser"
)

// entryLine returns the 0-based line where the symbol's *name* starts.
// When the parser records a NamePosition, that is the position of the
// identifier itself (e.g. the `Foo` in `class Foo < Bar`). When the
// parser does not record a NamePosition, we fall back to
// node.Range.Start, which points at the keyword (`class`) — this is the
// legacy behavior and is preserved for forward compatibility with
// older AST producers.
func entryLine(node *parser.Node) int {
	if node.NamePosition.Line > 0 {
		return node.NamePosition.Line
	}
	return node.Range.Start.Line
}

// entryChar returns the 0-based column where the symbol's *name* starts.
func entryChar(node *parser.Node) int {
	if node.NamePosition.Character > 0 || node.NamePosition.Line > 0 {
		return node.NamePosition.Character
	}
	return node.Range.Start.Character
}

// entryEndLine returns the 0-based line where the symbol's *name* ends.
// For now this is the same as entryLine because we only ever position
// the name on a single line.
func entryEndLine(node *parser.Node) int {
	return entryLine(node)
}

// entryEndChar returns the 0-based column right after the symbol's name.
// We deliberately do NOT use node.Range.End because for blocks (class,
// module, def) that is the position of the closing `end`, not the name.
func entryEndChar(node *parser.Node) int {
	start := entryChar(node)
	if start == node.Range.Start.Character && node.Name != "" {
		// Legacy fallback: NamePosition was not set. Approximate the
		// end of the name by walking forward from the keyword column.
		// This is wrong for `class EAnoSeriePlanoCursoArea < AR` because
		// the keyword is `class`, not the name, but at least it does
		// not return a position past the end of the line.
		return start + len(node.Name)
	}
	return start + len(node.Name)
}

// DeclarationVisitor walks an AST and collects indexable entries.
type DeclarationVisitor struct {
	FilePath   string
	Entries    []Entry
	nesting    []string
	visibility string
}

// NewDeclarationVisitor creates a visitor for a file.
func NewDeclarationVisitor(filePath string) *DeclarationVisitor {
	return &DeclarationVisitor{
		FilePath:   filePath,
		visibility: "public",
	}
}

// Visit walks the AST root and returns collected entries.
func (v *DeclarationVisitor) Visit(root *parser.Node) []Entry {
	if root == nil {
		return nil
	}
	v.walk(root)
	return v.Entries
}

func (v *DeclarationVisitor) walk(node *parser.Node) {
	if node == nil {
		return
	}

	switch node.Type {
	case parser.NodeClass:
		v.visitClass(node)
		for _, child := range node.Children {
			v.walk(child)
		}
		v.nesting = v.nesting[:len(v.nesting)-1]
		return
	case parser.NodeModule:
		v.visitModule(node)
		for _, child := range node.Children {
			v.walk(child)
		}
		v.nesting = v.nesting[:len(v.nesting)-1]
		return
	case parser.NodeMethod:
		v.visitMethod(node, false)
	case parser.NodeSingletonMethod:
		v.visitMethod(node, true)
	case parser.NodeConstant:
		v.visitConstant(node)
	case parser.NodeInstanceVariable:
		v.visitInstanceVariable(node)
	case parser.NodeClassVariable:
		v.visitClassVariable(node)
	case parser.NodeGlobalVariable:
		v.visitGlobalVariable(node)
	case parser.NodeAttrAccessor:
		v.visitAttrAccessor(node)
	case parser.NodeAlias:
		v.visitAlias(node)
	case parser.NodeCall:
		v.visitCall(node)
	}

	for _, child := range node.Children {
		v.walk(child)
	}
}

func (v *DeclarationVisitor) currentOwner() string {
	return strings.Join(v.nesting, "::")
}

func (v *DeclarationVisitor) visitClass(node *parser.Node) {
	name := node.Name
	owner := v.currentOwner()
	fqn := name
	if owner != "" && !strings.Contains(name, "::") {
		fqn = owner + "::" + name
	}

	entry := ClassEntry{
		BaseEntry: BaseEntry{
			Name:               shortName(name),
			FullyQualifiedName: fqn,
			FilePath:           v.FilePath,
			Line:               entryLine(node),
			EndLine:            entryEndLine(node),
			Character:          entryChar(node),
			EndCharacter:       entryEndChar(node),
			Owner:              owner,
			Visibility:         "public",
			Detail:             node.Detail,
		},
		Superclass: node.Detail,
	}
	v.Entries = append(v.Entries, entry)
	v.nesting = append(v.nesting, shortName(name))
}

func (v *DeclarationVisitor) visitModule(node *parser.Node) {
	name := node.Name
	owner := v.currentOwner()
	fqn := name
	if owner != "" && !strings.Contains(name, "::") {
		fqn = owner + "::" + name
	}

	entry := ModuleEntry{
		BaseEntry: BaseEntry{
			Name:               shortName(name),
			FullyQualifiedName: fqn,
			FilePath:           v.FilePath,
			Line:               entryLine(node),
			EndLine:            entryEndLine(node),
			Character:          entryChar(node),
			EndCharacter:       entryEndChar(node),
			Owner:              owner,
			Visibility:         "public",
		},
	}
	v.Entries = append(v.Entries, entry)
	v.nesting = append(v.nesting, shortName(name))
}

func (v *DeclarationVisitor) visitMethod(node *parser.Node, singleton bool) {
	name := node.Name
	owner := v.currentOwner()
	sep := "#"
	if singleton {
		sep = "."
		// Unlike def self.name, an explicit receiver owns this declaration,
		// not the lexical class/module enclosing it.
		if node.Receiver != "" && node.Receiver != "self" && node.Receiver != "(self)" {
			owner = node.Receiver
		}
	}
	fqn := name
	if owner != "" {
		fqn = owner + sep + name
	}

	entry := MethodEntry{
		BaseEntry: BaseEntry{
			Name:               name,
			FullyQualifiedName: fqn,
			FilePath:           v.FilePath,
			Line:               entryLine(node),
			EndLine:            entryEndLine(node),
			Character:          entryChar(node),
			EndCharacter:       entryEndChar(node),
			Owner:              owner,
			Visibility:         v.visibility,
		},
		IsSingleton: singleton,
	}
	v.Entries = append(v.Entries, entry)
}

func (v *DeclarationVisitor) visitConstant(node *parser.Node) {
	name := node.Name
	owner := v.currentOwner()
	fqn := name
	if owner != "" {
		fqn = owner + "::" + name
	}
	entry := ConstantEntry{
		BaseEntry: BaseEntry{
			Name:               name,
			FullyQualifiedName: fqn,
			FilePath:           v.FilePath,
			Line:               entryLine(node),
			EndLine:            entryEndLine(node),
			Character:          entryChar(node),
			EndCharacter:       entryEndChar(node),
			Owner:              owner,
			Visibility:         "public",
		},
	}
	v.Entries = append(v.Entries, entry)
}

func (v *DeclarationVisitor) visitInstanceVariable(node *parser.Node) {
	owner := v.currentOwner()
	fqn := node.Name
	if owner != "" {
		fqn = owner + "#" + node.Name
	}
	entry := InstanceVariableEntry{
		BaseEntry: BaseEntry{
			Name:               node.Name,
			FullyQualifiedName: fqn,
			FilePath:           v.FilePath,
			Line:               entryLine(node),
			EndLine:            entryEndLine(node),
			Character:          entryChar(node),
			EndCharacter:       entryEndChar(node),
			Owner:              owner,
			Visibility:         v.visibility,
		},
	}
	v.Entries = append(v.Entries, entry)
}

func (v *DeclarationVisitor) visitClassVariable(node *parser.Node) {
	owner := v.currentOwner()
	entry := ClassVariableEntry{
		BaseEntry: BaseEntry{
			Name:               node.Name,
			FullyQualifiedName: owner + node.Name,
			FilePath:           v.FilePath,
			Line:               node.Range.Start.Line,
			EndLine:            node.Range.End.Line,
			Character:          node.Range.Start.Character,
			EndCharacter:       node.Range.End.Character,
			Owner:              owner,
			Visibility:         v.visibility,
		},
	}
	v.Entries = append(v.Entries, entry)
}

func (v *DeclarationVisitor) visitGlobalVariable(node *parser.Node) {
	entry := GlobalVariableEntry{
		BaseEntry: BaseEntry{
			Name:               node.Name,
			FullyQualifiedName: node.Name,
			FilePath:           v.FilePath,
			Line:               node.Range.Start.Line,
			EndLine:            node.Range.End.Line,
			Character:          node.Range.Start.Character,
			EndCharacter:       node.Range.End.Character,
			Visibility:         "public",
		},
	}
	v.Entries = append(v.Entries, entry)
}

func (v *DeclarationVisitor) visitAttrAccessor(node *parser.Node) {
	owner := v.currentOwner()
	kind := node.Detail
	for _, child := range node.Children {
		if child.Type == parser.NodeSymbol {
			name := strings.TrimPrefix(child.Name, ":")
			entry := AccessorEntry{
				BaseEntry: BaseEntry{
					Name:               name,
					FullyQualifiedName: owner + "#" + name,
					FilePath:           v.FilePath,
					Line:               node.Range.Start.Line,
					EndLine:            node.Range.End.Line,
					Character:          child.Range.Start.Character,
					EndCharacter:       child.Range.End.Character,
					Owner:              owner,
					Visibility:         v.visibility,
					Detail:             kind,
				},
				AccessorKind: kind,
			}
			v.Entries = append(v.Entries, entry)
		}
	}
}

func (v *DeclarationVisitor) visitAlias(node *parser.Node) {
	owner := v.currentOwner()
	entry := MethodEntry{
		BaseEntry: BaseEntry{
			Name:               node.Name,
			FullyQualifiedName: owner + "#" + node.Name,
			FilePath:           v.FilePath,
			Line:               node.Range.Start.Line,
			EndLine:            node.Range.End.Line,
			Character:          node.Range.Start.Character,
			EndCharacter:       node.Range.End.Character,
			Owner:              owner,
			Visibility:         v.visibility,
			Detail:             "alias",
		},
	}
	v.Entries = append(v.Entries, entry)
}

func (v *DeclarationVisitor) visitCall(node *parser.Node) {
	name := node.Name
	switch name {
	case "private", "protected", "public":
		v.visibility = name
	case "attr_reader", "attr_writer", "attr_accessor":
		v.visitAttrCall(node, name)
	case "scope":
		v.visitScope(node)
	case "belongs_to", "has_many", "has_one", "has_and_belongs_to_many":
		v.visitAssociation(node, name)
	case "validates":
		v.visitValidates(node)
	case "before_action", "after_action", "around_action":
		v.visitCallback(node, name)
	}
}

func (v *DeclarationVisitor) visitAttrCall(node *parser.Node, kind string) {
	owner := v.currentOwner()
	for _, child := range node.Children {
		if child.Type == parser.NodeSymbol {
			name := strings.TrimPrefix(child.Name, ":")
			entry := AccessorEntry{
				BaseEntry: BaseEntry{
					Name:               name,
					FullyQualifiedName: owner + "#" + name,
					FilePath:           v.FilePath,
					Line:               node.Range.Start.Line,
					EndLine:            node.Range.End.Line,
					Character:          child.Range.Start.Character,
					EndCharacter:       child.Range.End.Character,
					Owner:              owner,
					Visibility:         v.visibility,
					Detail:             kind,
				},
				AccessorKind: strings.TrimPrefix(kind, "attr_"),
			}
			v.Entries = append(v.Entries, entry)
		}
	}
}

func (v *DeclarationVisitor) visitScope(node *parser.Node) {
	owner := v.currentOwner()
	for _, child := range node.Children {
		if child.Type == parser.NodeSymbol {
			name := strings.TrimPrefix(child.Name, ":")
			entry := MethodEntry{
				BaseEntry: BaseEntry{
					Name:               name,
					FullyQualifiedName: owner + "." + name,
					FilePath:           v.FilePath,
					Line:               node.Range.Start.Line,
					EndLine:            node.Range.End.Line,
					Character:          child.Range.Start.Character,
					EndCharacter:       child.Range.End.Character,
					Owner:              owner,
					Visibility:         "public",
					Detail:             "scope",
				},
			}
			v.Entries = append(v.Entries, entry)
		}
	}
}

func (v *DeclarationVisitor) visitAssociation(node *parser.Node, kind string) {
	owner := v.currentOwner()
	for _, child := range node.Children {
		if child.Type == parser.NodeSymbol {
			name := strings.TrimPrefix(child.Name, ":")
			entry := MethodEntry{
				BaseEntry: BaseEntry{
					Name:               name,
					FullyQualifiedName: owner + "#" + name,
					FilePath:           v.FilePath,
					Line:               node.Range.Start.Line,
					EndLine:            node.Range.End.Line,
					Character:          child.Range.Start.Character,
					EndCharacter:       child.Range.End.Character,
					Owner:              owner,
					Visibility:         "public",
					Detail:             kind,
				},
			}
			v.Entries = append(v.Entries, entry)
		}
	}
}

func (v *DeclarationVisitor) visitValidates(node *parser.Node) {
	owner := v.currentOwner()
	for _, child := range node.Children {
		if child.Type == parser.NodeSymbol {
			name := strings.TrimPrefix(child.Name, ":")
			entry := MethodEntry{
				BaseEntry: BaseEntry{
					Name:               name,
					FullyQualifiedName: owner + "#" + name,
					FilePath:           v.FilePath,
					Line:               node.Range.Start.Line,
					EndLine:            node.Range.End.Line,
					Character:          child.Range.Start.Character,
					EndCharacter:       child.Range.End.Character,
					Owner:              owner,
					Visibility:         "public",
					Detail:             "validates",
				},
			}
			v.Entries = append(v.Entries, entry)
		}
	}
}

func (v *DeclarationVisitor) visitCallback(node *parser.Node, kind string) {
	owner := v.currentOwner()
	for _, child := range node.Children {
		if child.Type == parser.NodeSymbol {
			name := strings.TrimPrefix(child.Name, ":")
			entry := MethodEntry{
				BaseEntry: BaseEntry{
					Name:               name,
					FullyQualifiedName: owner + "#" + name,
					FilePath:           v.FilePath,
					Line:               node.Range.Start.Line,
					EndLine:            node.Range.End.Line,
					Character:          child.Range.Start.Character,
					EndCharacter:       child.Range.End.Character,
					Owner:              owner,
					Visibility:         "public",
					Detail:             kind,
				},
			}
			v.Entries = append(v.Entries, entry)
		}
	}
}

func shortName(name string) string {
	parts := strings.Split(name, "::")
	return parts[len(parts)-1]
}
