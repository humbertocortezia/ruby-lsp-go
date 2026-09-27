package store

import (
	"sync"

	"github.com/humberto/ruby-lsp-go/documents"
	"github.com/humberto/ruby-lsp-go/workspace"
)

// Store manages open documents and per-document caches.
type Store struct {
	documents      map[string]documents.Document
	combinedCaches map[string]*documents.CombinedCache
	mutex          sync.RWMutex
	state          *workspace.State
}

// New creates a new document store.
func New(state *workspace.State) *Store {
	return &Store{
		documents:      make(map[string]documents.Document),
		combinedCaches: make(map[string]*documents.CombinedCache),
		state:          state,
	}
}

// GetDocument retrieves a document by URI.
func (s *Store) GetDocument(uri string) (documents.Document, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	doc, exists := s.documents[uri]
	return doc, exists
}

// SetDocument creates or updates a document.
func (s *Store) SetDocument(uri, source string, version int, languageID string) documents.Document {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	doc := documents.NewDocument(uri, source, version, languageID)
	s.documents[uri] = doc

	if cache, ok := s.combinedCaches[uri]; ok {
		cache.Invalidate()
	} else {
		s.combinedCaches[uri] = documents.NewCombinedCache()
	}

	return doc
}

// DeleteDocument removes a document.
func (s *Store) DeleteDocument(uri string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.documents, uri)
	delete(s.combinedCaches, uri)
}

// ApplyEdits applies incremental edits to a document.
func (s *Store) ApplyEdits(uri string, edits []documents.TextEdit, version int) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	doc, exists := s.documents[uri]
	if !exists {
		return
	}

	doc.ApplyEdits(edits)

	if cache, ok := s.combinedCaches[uri]; ok {
		cache.Invalidate()
	}

	_ = version
}

// GetCombinedCache returns the combined AST cache for a document.
func (s *Store) GetCombinedCache(uri string) *documents.CombinedCache {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if cache, ok := s.combinedCaches[uri]; ok {
		return cache
	}
	cache := documents.NewCombinedCache()
	s.combinedCaches[uri] = cache
	return cache
}

// Each iterates over all open documents.
func (s *Store) Each(fn func(string, documents.Document)) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	for uri, doc := range s.documents {
		fn(uri, doc)
	}
}

// Keys returns all open document URIs.
func (s *Store) Keys() []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	keys := make([]string, 0, len(s.documents))
	for uri := range s.documents {
		keys = append(keys, uri)
	}
	return keys
}

// Legacy Document struct for backward compatibility.
type Document struct {
	URI        string
	Version    int
	Source     string
	LanguageID string
}

// Get retrieves legacy document (backward compat).
func (s *Store) Get(uri string) (*Document, bool) {
	doc, ok := s.GetDocument(uri)
	if !ok {
		return nil, false
	}
	return &Document{
		URI:        doc.URI(),
		Version:    doc.Version(),
		Source:     doc.Source(),
		LanguageID: doc.LanguageID(),
	}, true
}

// Set creates legacy document (backward compat).
func (s *Store) Set(uri, source string, version int, languageID string) *Document {
	s.SetDocument(uri, source, version, languageID)
	return &Document{URI: uri, Version: version, Source: source, LanguageID: languageID}
}

// Delete removes document (backward compat).
func (s *Store) Delete(uri string) {
	s.DeleteDocument(uri)
}

// Clear removes all documents.
func (s *Store) Clear() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.documents = make(map[string]documents.Document)
	s.combinedCaches = make(map[string]*documents.CombinedCache)
}
