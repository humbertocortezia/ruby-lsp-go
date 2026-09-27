package lsp

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/humberto/ruby-lsp-go/documents"
	"github.com/humberto/ruby-lsp-go/indexer"
	"github.com/humberto/ruby-lsp-go/lsp/handlers"
	"github.com/humberto/ruby-lsp-go/store"
	"github.com/humberto/ruby-lsp-go/workspace"
)

const workerCount = 4

// Message represents an LSP JSON-RPC message.
type Message struct {
	ID     interface{} `json:"id,omitempty"`
	Method string      `json:"method,omitempty"`
	Params interface{} `json:"params,omitempty"`
}

// Server is the LSP server with worker pool for request processing.
type Server struct {
	State             *workspace.State
	Store             *store.Store
	Index             *indexer.IndexStore
	IncomingQueue     chan Message
	OutgoingQueue     chan Message
	CancelledRequests map[int]bool
	cancelMutex       sync.RWMutex
	Logger            *log.Logger
	shutdown          bool
	shutdownMutex     sync.Mutex
}

// HandlerContext returns the shared handler context.
func (s *Server) HandlerContext() *handlers.Context {
	return &handlers.Context{
		State:  s.State,
		Store:  s.Store,
		Index:  s.Index,
		Logger: s.Logger,
	}
}

// StartWorkers launches the worker pool consuming IncomingQueue.
func (s *Server) StartWorkers() {
	for i := 0; i < workerCount; i++ {
		go s.worker(i)
	}
}

func (s *Server) worker(id int) {
	s.Logger.Printf("Worker %d started", id)
	for msg := range s.IncomingQueue {
		if s.isCancelled(msg) {
			s.Logger.Printf("Worker %d: request %v cancelled", id, msg.ID)
			continue
		}
		s.processMessage(msg)
	}
	s.Logger.Printf("Worker %d stopped", id)
}

func (s *Server) isCancelled(msg Message) bool {
	if msg.ID == nil {
		return false
	}
	id, ok := msg.ID.(float64)
	if !ok {
		if intID, ok := msg.ID.(int); ok {
			id = float64(intID)
		} else {
			return false
		}
	}
	s.cancelMutex.RLock()
	defer s.cancelMutex.RUnlock()
	return s.CancelledRequests[int(id)]
}

// processMessage routes a message to the appropriate handler.
func (s *Server) processMessage(msg Message) {
	ctx := s.HandlerContext()

	switch msg.Method {
	case "textDocument/completion":
		s.SendResponse(msg.ID, handlers.Completion(ctx, msg.Params))
	case "textDocument/hover":
		s.SendResponse(msg.ID, handlers.Hover(ctx, msg.Params))
	case "textDocument/definition":
		s.SendResponse(msg.ID, handlers.Definition(ctx, msg.Params))
	case "textDocument/documentSymbol":
		s.SendResponse(msg.ID, handlers.DocumentSymbol(ctx, msg.Params))
	case "textDocument/formatting":
		s.SendResponse(msg.ID, handlers.Formatting(ctx, msg.Params))
	case "textDocument/rangeFormatting":
		s.SendResponse(msg.ID, handlers.RangeFormatting(ctx, msg.Params))
	case "textDocument/onTypeFormatting":
		s.SendResponse(msg.ID, handlers.OnTypeFormatting(ctx, msg.Params))
	case "textDocument/foldingRange":
		s.SendResponse(msg.ID, handlers.FoldingRange(ctx, msg.Params))
	case "textDocument/selectionRange":
		s.SendResponse(msg.ID, handlers.SelectionRange(ctx, msg.Params))
	case "textDocument/documentHighlight":
		s.SendResponse(msg.ID, handlers.DocumentHighlight(ctx, msg.Params))
	case "textDocument/signatureHelp":
		s.SendResponse(msg.ID, handlers.SignatureHelp(ctx, msg.Params))
	case "textDocument/codeAction":
		s.SendResponse(msg.ID, handlers.CodeAction(ctx, msg.Params))
	case "textDocument/codeAction/resolve":
		s.SendResponse(msg.ID, handlers.CodeActionResolve(ctx, msg.Params))
	case "textDocument/rename":
		s.SendResponse(msg.ID, handlers.Rename(ctx, msg.Params))
	case "textDocument/prepareRename":
		s.SendResponse(msg.ID, handlers.PrepareRename(ctx, msg.Params))
	case "textDocument/references":
		s.SendResponse(msg.ID, handlers.References(ctx, msg.Params))
	case "textDocument/semanticTokens/full":
		s.SendResponse(msg.ID, handlers.SemanticTokensFull(ctx, msg.Params))
	case "textDocument/semanticTokens/range":
		s.SendResponse(msg.ID, handlers.SemanticTokensRange(ctx, msg.Params))
	case "textDocument/inlayHint":
		s.SendResponse(msg.ID, handlers.InlayHints(ctx, msg.Params))
	case "textDocument/codeLens":
		s.SendResponse(msg.ID, handlers.CodeLens(ctx, msg.Params))
	case "codeLens/resolve":
		s.SendResponse(msg.ID, handlers.CodeLensResolve(ctx, msg.Params))
	case "textDocument/documentLink":
		s.SendResponse(msg.ID, handlers.DocumentLink(ctx, msg.Params))
	case "textDocument/diagnostic":
		s.SendResponse(msg.ID, handlers.Diagnostics(ctx, msg.Params))
	case "textDocument/prepareTypeHierarchy":
		s.SendResponse(msg.ID, handlers.PrepareTypeHierarchy(ctx, msg.Params))
	case "typeHierarchy/supertypes":
		s.SendResponse(msg.ID, handlers.TypeHierarchySupertypes(ctx, msg.Params))
	case "workspace/symbol":
		s.SendResponse(msg.ID, handlers.WorkspaceSymbol(ctx, msg.Params))
	default:
		s.Logger.Printf("Unhandled method: %s", msg.Method)
	}
}

// HandleInitialize handles the LSP initialize request.
func (s *Server) HandleInitialize(params interface{}) interface{} {
	s.Logger.Println("Processing initialize request")

	if paramMap, ok := params.(map[string]interface{}); ok {
		s.State.ApplyInitializationOptions(paramMap)

		if rootURI, ok := paramMap["rootUri"].(string); ok {
			s.State.WorkspaceURI = rootURI
			s.State.WorkspacePath = uriToPath(rootURI)
		} else if rootPath, ok := paramMap["rootPath"].(string); ok {
			s.State.WorkspacePath = rootPath
			s.State.WorkspaceURI = "file://" + rootPath
		} else if folders, ok := paramMap["workspaceFolders"].([]interface{}); ok && len(folders) > 0 {
			if folder, ok := folders[0].(map[string]interface{}); ok {
				if folderURI, ok := folder["uri"].(string); ok {
					s.State.WorkspaceURI = folderURI
					s.State.WorkspacePath = uriToPath(folderURI)
				}
			}
		}
	}

	if s.State.WorkspacePath != "" && s.Index == nil {
		s.Index = indexer.NewIndexStore(s.State.WorkspacePath, s.Logger)
		go s.Index.BuildIndex()
	}

	return map[string]interface{}{
		"capabilities": buildCapabilities(),
		"serverInfo": map[string]string{
			"name":    "Ruby LSP Go",
			"version": "2.0.0",
		},
	}
}

func buildCapabilities() map[string]interface{} {
	return map[string]interface{}{
		"textDocumentSync": map[string]interface{}{
			"change":    2,
			"openClose": true,
			"save":      map[string]interface{}{"includeText": false},
		},
		"completionProvider": map[string]interface{}{
			"triggerCharacters": []string{".", ":", "@", "#", "$"},
		},
		"hoverProvider":                    true,
		"definitionProvider":               true,
		"documentSymbolProvider":           true,
		"workspaceSymbolProvider":          true,
		"documentFormattingProvider":       true,
		"documentRangeFormattingProvider":    true,
		"documentOnTypeFormattingProvider": map[string]interface{}{
			"firstTriggerCharacter": "\n",
			"moreTriggerCharacter":  []string{"d"},
		},
		"documentHighlightProvider": true,
		"codeActionProvider": map[string]interface{}{
			"resolveProvider": true,
			"codeActionKinds": []string{"quickfix", "refactor", "source.fixAll", "refactor.extract.variable", "refactor.rewrite"},
		},
		"foldingRangeProvider":   true,
		"selectionRangeProvider": true,
		"renameProvider": map[string]interface{}{
			"prepareProvider": true,
		},
		"referencesProvider": true,
		"signatureHelpProvider": map[string]interface{}{
			"triggerCharacters": []string{"(", ","},
		},
		"semanticTokensProvider": map[string]interface{}{
			"legend": map[string]interface{}{
				"tokenTypes": []string{
					"namespace", "type", "class", "enum", "interface", "struct",
					"typeParameter", "parameter", "variable", "property", "enumMember",
					"event", "function", "method", "macro", "keyword", "modifier",
					"comment", "string", "number", "regexp", "operator", "decorator",
				},
				"tokenModifiers": []string{
					"declaration", "definition", "readonly", "static", "deprecated",
					"abstract", "async", "modification", "documentation", "defaultLibrary",
				},
			},
			"full":  map[string]interface{}{"delta": false},
			"range": true,
		},
		"inlayHintProvider": map[string]interface{}{
			"resolveProvider": false,
		},
		"codeLensProvider": map[string]interface{}{
			"resolveProvider": true,
		},
		"documentLinkProvider": map[string]interface{}{
			"resolveProvider": false,
		},
		"diagnosticProvider": map[string]interface{}{
			"interFileDependencies": false,
			"workspaceDiagnostics":  false,
		},
		"typeHierarchyProvider": true,
	}
}

// HandleInitialized handles initialized notification.
func (s *Server) HandleInitialized() {
	s.Logger.Println("Initialization complete")
}

// HandleDidOpen handles textDocument/didOpen.
func (s *Server) HandleDidOpen(params interface{}) {
	if paramMap, ok := params.(map[string]interface{}); ok {
		if textDoc, ok := paramMap["textDocument"].(map[string]interface{}); ok {
			uri, _ := textDoc["uri"].(string)
			text, _ := textDoc["text"].(string)
			version, _ := textDoc["version"].(float64)
			languageID, _ := textDoc["languageId"].(string)
			s.Store.SetDocument(uri, text, int(version), languageID)
			s.Logger.Printf("Opened document: %s", uri)
		}
	}
}

// HandleDidClose handles textDocument/didClose.
func (s *Server) HandleDidClose(params interface{}) {
	if paramMap, ok := params.(map[string]interface{}); ok {
		if textDoc, ok := paramMap["textDocument"].(map[string]interface{}); ok {
			uri, _ := textDoc["uri"].(string)
			s.Store.DeleteDocument(uri)
			s.Logger.Printf("Closed document: %s", uri)
		}
	}
}

// HandleDidChange handles textDocument/didChange.
func (s *Server) HandleDidChange(params interface{}) {
	if paramMap, ok := params.(map[string]interface{}); ok {
		if textDoc, ok := paramMap["textDocument"].(map[string]interface{}); ok {
			uri, _ := textDoc["uri"].(string)
			version, _ := textDoc["version"].(float64)

			if changes, ok := paramMap["contentChanges"].([]interface{}); ok {
				edits := make([]documents.TextEdit, 0, len(changes))
				for _, change := range changes {
					if changeMap, ok := change.(map[string]interface{}); ok {
						edits = append(edits, parseTextEdit(changeMap))
					}
				}
				s.Store.ApplyEdits(uri, edits, int(version))
				s.Logger.Printf("Changed document: %s", uri)
			}
		}
	}
}

// HandleDidSave handles textDocument/didSave.
func (s *Server) HandleDidSave(params interface{}) {
	if paramMap, ok := params.(map[string]interface{}); ok {
		if textDoc, ok := paramMap["textDocument"].(map[string]interface{}); ok {
			uri, _ := textDoc["uri"].(string)
			filePath := uriToPath(uri)
			if s.Index != nil {
				go s.Index.UpdateFile(filePath)
			}
		}
	}
}

// HandleCancelRequest handles $/cancelRequest.
func (s *Server) HandleCancelRequest(params interface{}) {
	if paramMap, ok := params.(map[string]interface{}); ok {
		if idParam, exists := paramMap["id"]; exists {
			var id int
			switch v := idParam.(type) {
			case float64:
				id = int(v)
			case int:
				id = v
			}
			s.cancelMutex.Lock()
			s.CancelledRequests[id] = true
			s.cancelMutex.Unlock()
		}
	}
}

// SendResponse sends a JSON-RPC response to stdout.
func (s *Server) SendResponse(id interface{}, result interface{}) {
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		s.Logger.Printf("Error marshaling response: %v", err)
		return
	}

	fmt.Printf("Content-Length: %d\r\n\r\n%s", len(jsonBytes), jsonBytes)
}

// SendNotification sends a JSON-RPC notification to stdout.
func (s *Server) SendNotification(method string, params interface{}) {
	notification := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	jsonBytes, _ := json.Marshal(notification)
	fmt.Printf("Content-Length: %d\r\n\r\n%s", len(jsonBytes), jsonBytes)
}

// Shutdown handles server shutdown.
func (s *Server) Shutdown() {
	s.shutdownMutex.Lock()
	defer s.shutdownMutex.Unlock()
	if s.shutdown {
		return
	}
	s.shutdown = true
	close(s.IncomingQueue)
	s.Logger.Println("Shutting down Ruby LSP Go server")
}

// Enqueue adds a message to the worker queue.
func (s *Server) Enqueue(msg Message) {
	select {
	case s.IncomingQueue <- msg:
	default:
		s.Logger.Printf("Incoming queue full, processing synchronously: %s", msg.Method)
		s.processMessage(msg)
	}
}

// DispatchOutgoingMessages processes outgoing notifications.
func (s *Server) DispatchOutgoingMessages() {
	for msg := range s.OutgoingQueue {
		if msg.Method != "" {
			s.SendNotification(msg.Method, msg.Params)
		}
	}
}

func parseTextEdit(changeMap map[string]interface{}) documents.TextEdit {
	var edit documents.TextEdit
	if rangeInterface, exists := changeMap["range"]; exists {
		if rangeMap, isMap := rangeInterface.(map[string]interface{}); isMap {
			edit.Range = parseRange(rangeMap)
		}
	}
	edit.NewText, _ = changeMap["text"].(string)
	return edit
}

func parseRange(rangeMap map[string]interface{}) *documents.Range {
	r := &documents.Range{}
	if startMap, ok := rangeMap["start"].(map[string]interface{}); ok {
		line, _ := startMap["line"].(float64)
		char, _ := startMap["character"].(float64)
		r.Start = documents.Position{Line: int(line), Character: int(char)}
	}
	if endMap, ok := rangeMap["end"].(map[string]interface{}); ok {
		line, _ := endMap["line"].(float64)
		char, _ := endMap["character"].(float64)
		r.End = documents.Position{Line: int(line), Character: int(char)}
	}
	return r
}

func uriToPath(uri string) string {
	if len(uri) > 7 && uri[:7] == "file://" {
		return uri[7:]
	}
	return uri
}
