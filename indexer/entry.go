package indexer

import (
	"github.com/humberto/ruby-lsp-go/lsp/types"
)

// SymbolEntry aliases types.SymbolEntry so existing code can keep using
// indexer.SymbolEntry as a short name.
type SymbolEntry = types.SymbolEntry
type SymbolType = types.SymbolType

const (
	SymbolClass            = types.IndexerSymbolClass
	SymbolModule           = types.IndexerSymbolModule
	SymbolMethod           = types.IndexerSymbolMethod
	SymbolSingletonMethod  = types.IndexerSymbolSingletonMethod
	SymbolAttrAccessor     = types.IndexerSymbolAttrAccessor
	SymbolConstant         = types.IndexerSymbolConstant
	SymbolInstanceVariable = types.IndexerSymbolInstanceVariable
	SymbolClassVariable    = types.IndexerSymbolClassVariable
	SymbolGlobalVariable   = types.IndexerSymbolGlobalVariable
	SymbolScope            = types.IndexerSymbolScope
	SymbolAssociation      = types.IndexerSymbolAssociation
)

// EntryType classifies indexed Ruby entities.
type EntryType int

const (
	EntryClass EntryType = iota
	EntryModule
	EntryMethod
	EntrySingletonMethod
	EntryConstant
	EntryConstantAlias
	EntryAccessor
	EntryMethodAlias
	EntryGlobalVariable
	EntryClassVariable
	EntryInstanceVariable
	EntryScope
	EntryAssociation
)

// Entry is the interface for all indexed symbols.
type Entry interface {
	GetName() string
	GetFullyQualifiedName() string
	GetType() EntryType
	GetLocation() types.Location
	GetOwner() string
	GetVisibility() string
	GetDetail() string
	GetParameters() []string
}

// BaseEntry holds common fields for all entry types.
type BaseEntry struct {
	Name               string
	FullyQualifiedName string
	FilePath           string
	Line               int
	EndLine            int
	Character          int
	EndCharacter       int
	Owner              string
	Visibility         string
	Detail             string
	Parameters         []string
}

func (b BaseEntry) GetName() string               { return b.Name }
func (b BaseEntry) GetFullyQualifiedName() string { return b.FullyQualifiedName }
func (b BaseEntry) GetOwner() string              { return b.Owner }
func (b BaseEntry) GetVisibility() string         { return b.Visibility }
func (b BaseEntry) GetDetail() string             { return b.Detail }
func (b BaseEntry) GetParameters() []string       { return b.Parameters }

func (b BaseEntry) GetLocation() types.Location {
	uri := "file://" + b.FilePath
	if len(b.FilePath) > 0 && b.FilePath[0] != '/' {
		uri = "file:///" + b.FilePath
	}
	return types.Location{
		URI: uri,
		Range: types.Range{
			Start: types.Position{Line: b.Line, Character: b.Character},
			End:   types.Position{Line: b.EndLine, Character: b.EndCharacter},
		},
	}
}

// ClassEntry represents a Ruby class.
type ClassEntry struct {
	BaseEntry
	Superclass string
	Mixins     []MixinEntry
}

func (e ClassEntry) GetType() EntryType { return EntryClass }

// ModuleEntry represents a Ruby module.
type ModuleEntry struct {
	BaseEntry
	Mixins []MixinEntry
}

func (e ModuleEntry) GetType() EntryType { return EntryModule }

// MethodEntry represents an instance or class method.
type MethodEntry struct {
	BaseEntry
	IsSingleton bool
}

func (e MethodEntry) GetType() EntryType {
	if e.IsSingleton {
		return EntrySingletonMethod
	}
	return EntryMethod
}

// ConstantEntry represents a constant assignment.
type ConstantEntry struct {
	BaseEntry
}

func (e ConstantEntry) GetType() EntryType { return EntryConstant }

// ConstantAliasEntry represents a constant alias.
type ConstantAliasEntry struct {
	BaseEntry
	Target string
}

func (e ConstantAliasEntry) GetType() EntryType { return EntryConstantAlias }

// AccessorEntry represents attr_reader/writer/accessor.
type AccessorEntry struct {
	BaseEntry
	AccessorKind string // reader, writer, accessor
}

func (e AccessorEntry) GetType() EntryType { return EntryAccessor }

// InstanceVariableEntry represents @ivar.
type InstanceVariableEntry struct {
	BaseEntry
}

func (e InstanceVariableEntry) GetType() EntryType { return EntryInstanceVariable }

// ClassVariableEntry represents @@cvar.
type ClassVariableEntry struct {
	BaseEntry
}

func (e ClassVariableEntry) GetType() EntryType { return EntryClassVariable }

// GlobalVariableEntry represents $gvar.
type GlobalVariableEntry struct {
	BaseEntry
}

func (e GlobalVariableEntry) GetType() EntryType { return EntryGlobalVariable }

// MixinEntry tracks include/prepend/extend.
type MixinEntry struct {
	Name string
	Kind string // include, prepend, extend
}

// EntryToSymbolEntry converts a new Entry to legacy SymbolEntry.
func EntryToSymbolEntry(e Entry) SymbolEntry {
	loc := e.GetLocation()
	entryType := entryTypeToSymbolType(e.GetType())
	return SymbolEntry{
		Name:               e.GetName(),
		FullyQualifiedName: e.GetFullyQualifiedName(),
		Type:               entryType,
		FilePath:           filePathFromURI(loc.URI),
		Line:               loc.Range.Start.Line + 1,
		EndLine:            loc.Range.End.Line + 1,
		Character:          loc.Range.Start.Character,
		EndCharacter:       loc.Range.End.Character,
		Parent:             e.GetOwner(),
		Visibility:         e.GetVisibility(),
		Detail:             e.GetDetail(),
		Parameters:         e.GetParameters(),
	}
}

func entryTypeToSymbolType(t EntryType) SymbolType {
	switch t {
	case EntryClass:
		return SymbolClass
	case EntryModule:
		return SymbolModule
	case EntryMethod:
		return SymbolMethod
	case EntrySingletonMethod:
		return SymbolSingletonMethod
	case EntryConstant, EntryConstantAlias:
		return SymbolConstant
	case EntryAccessor:
		return SymbolAttrAccessor
	case EntryInstanceVariable:
		return SymbolInstanceVariable
	case EntryClassVariable:
		return SymbolClassVariable
	case EntryGlobalVariable:
		return SymbolGlobalVariable
	case EntryScope:
		return SymbolScope
	case EntryAssociation:
		return SymbolAssociation
	default:
		return SymbolMethod
	}
}

func filePathFromURI(uri string) string {
	if len(uri) > 7 && uri[:7] == "file://" {
		path := uri[7:]
		if len(path) > 2 && path[0] == '/' && path[2] == ':' {
			return path[1:]
		}
		return path
	}
	return uri
}
