package store

import (
	"sync"
)

type Store struct {
	documents   map[string]*Document
	mutex       sync.RWMutex
	globalState interface{}
}

type Document struct {
	URI        string
	Version    int
	Source     string
	LanguageID string
}

// New creates a new store
func New(gs interface{}) *Store {
	return &Store{
		documents:   make(map[string]*Document),
		globalState: gs,
	}
}

// Get retrieves a document by URI
func (s *Store) Get(uri string) (*Document, bool) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	doc, exists := s.documents[uri]
	return doc, exists
}

// Set creates or updates a document in the store
func (s *Store) Set(uri string, source string, version int, languageID string) *Document {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	doc := &Document{
		URI:       uri,
		Version:   version,
		Source:    source,
		LanguageID: languageID,
	}
	
	s.documents[uri] = doc
	return doc
}

// Delete removes a document from the store
func (s *Store) Delete(uri string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	delete(s.documents, uri)
}

// Clear removes all documents from the store
func (s *Store) Clear() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	
	s.documents = make(map[string]*Document)
}

// Each iterates over all documents in the store
func (s *Store) Each(fn func(string, *Document)) {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	for uri, doc := range s.documents {
		fn(uri, doc)
	}
}

// Keys returns all URIs in the store
func (s *Store) Keys() []string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	
	keys := make([]string, 0, len(s.documents))
	for uri := range s.documents {
		keys = append(keys, uri)
	}
	return keys
}

