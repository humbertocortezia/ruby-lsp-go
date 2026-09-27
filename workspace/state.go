package workspace

import (
	"sync"
)

// State holds global workspace configuration shared across the LSP server.
type State struct {
	WorkspaceURI       string
	WorkspacePath      string
	Formatter          string
	Linters            []string
	TestLibrary        string
	HasTypeChecker     bool
	ClientCapabilities map[string]interface{}
	EnabledFeatures    map[string]bool
	IndexingConfig     map[string]interface{}
	Mutex              sync.RWMutex
}

// FeatureEnabled returns whether an LSP feature is enabled (default true).
func (s *State) FeatureEnabled(name string) bool {
	s.Mutex.RLock()
	defer s.Mutex.RUnlock()
	if s.EnabledFeatures == nil {
		return true
	}
	enabled, ok := s.EnabledFeatures[name]
	if !ok {
		return true
	}
	return enabled
}

// ApplyInitializationOptions merges client initialization options.
func (s *State) ApplyInitializationOptions(params map[string]interface{}) {
	s.Mutex.Lock()
	defer s.Mutex.Unlock()

	if initOpts, ok := params["initializationOptions"].(map[string]interface{}); ok {
		if formatter, ok := initOpts["formatter"].(string); ok {
			s.Formatter = formatter
		}
		if linters, ok := initOpts["linters"].([]interface{}); ok {
			s.Linters = make([]string, 0, len(linters))
			for _, l := range linters {
				if str, ok := l.(string); ok {
					s.Linters = append(s.Linters, str)
				}
			}
		}
		if features, ok := initOpts["enabledFeatures"].(map[string]interface{}); ok {
			if s.EnabledFeatures == nil {
				s.EnabledFeatures = make(map[string]bool)
			}
			for k, v := range features {
				if b, ok := v.(bool); ok {
					s.EnabledFeatures[k] = b
				}
			}
		}
		if indexing, ok := initOpts["indexing"].(map[string]interface{}); ok {
			s.IndexingConfig = indexing
		}
	}
}
