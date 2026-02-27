package lsp

import (
	"sync"
)

type Message struct {
	ID     interface{} `json:"id,omitempty"`
	Method string      `json:"method,omitempty"`
	Params interface{} `json:"params,omitempty"`
}

type GlobalState struct {
	WorkspaceURI       string
	WorkspacePath      string
	Formatter          string
	TestLibrary        string
	HasTypeChecker     bool
	ClientCapabilities map[string]interface{}
	EnabledFeatures    map[string]bool
	Mutex              sync.Mutex
}

type Server struct {
	GlobalState       *GlobalState
	Store             interface{} // Will be defined in the store package
	Indexer           interface{} // Workspace indexer
	IncomingQueue     chan Message
	OutgoingQueue     chan Message
	CancelledRequests map[int]bool
	Logger            interface{} // Logger interface
}

