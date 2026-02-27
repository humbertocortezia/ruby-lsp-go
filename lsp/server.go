package lsp

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/humberto/ruby-lsp-go/documents"
	"github.com/humberto/ruby-lsp-go/indexer"
	"github.com/humberto/ruby-lsp-go/store"
)

// HandleInitialize handles the LSP initialize request
func (s *Server) HandleInitialize(params interface{}) interface{} {
	s.Logger.(*log.Logger).Println("Processing initialize request")

	capabilities := map[string]interface{}{
		"capabilities": map[string]interface{}{
			"textDocumentSync": map[string]interface{}{
				"change":    2, // incremental
				"openClose": true,
				"save":      map[string]interface{}{"includeText": false},
			},
			"completionProvider": map[string]interface{}{
				"triggerCharacters": []string{".", ":", "@"},
			},
			"hoverProvider":              true,
			"definitionProvider":         true,
			"documentSymbolProvider":     true,
			"workspaceSymbolProvider":    true,
			"documentFormattingProvider": true,
			"documentHighlightProvider":  true,
			"codeActionProvider": map[string]interface{}{
				"codeActionKinds": []string{"quickfix", "refactor"},
			},
			"foldingRangeProvider": true,
			"renameProvider":      true,
			"referencesProvider":  true,
		},
		"serverInfo": map[string]string{
			"name":    "Ruby LSP Go",
			"version": "1.2.0",
		},
		"formatter":     "none",
		"degraded_mode": false,
	}

	return capabilities
}

// HandleInitialized handles the initialized notification
func (s *Server) HandleInitialized() {
	s.Logger.(*log.Logger).Println("Initialization complete")
	s.Logger.(*log.Logger).Println("Performing initial indexing...")
}

// HandleDidOpen handles textDocument/didOpen notification
func (s *Server) HandleDidOpen(params interface{}) {
	if paramMap, ok := params.(map[string]interface{}); ok {
		if textDoc, ok := paramMap["textDocument"].(map[string]interface{}); ok {
			uri, _ := textDoc["uri"].(string)
			text, _ := textDoc["text"].(string)
			version, _ := textDoc["version"].(float64)
			languageID, _ := textDoc["languageId"].(string)

			storeInst := s.Store.(*store.Store)
			storeInst.Set(uri, text, int(version), languageID)

			s.Logger.(*log.Logger).Printf("Opened document: %s", uri)
		}
	}
}

// HandleDidClose handles textDocument/didClose notification
func (s *Server) HandleDidClose(params interface{}) {
	if paramMap, ok := params.(map[string]interface{}); ok {
		if textDoc, ok := paramMap["textDocument"].(map[string]interface{}); ok {
			uri, _ := textDoc["uri"].(string)

			storeInst := s.Store.(*store.Store)
			storeInst.Delete(uri)

			s.Logger.(*log.Logger).Printf("Closed document: %s", uri)
		}
	}
}

// HandleDidChange handles textDocument/didChange notification
func (s *Server) HandleDidChange(params interface{}) {
	if paramMap, ok := params.(map[string]interface{}); ok {
		if textDoc, ok := paramMap["textDocument"].(map[string]interface{}); ok {
			uri, _ := textDoc["uri"].(string)

			if changes, ok := paramMap["contentChanges"].([]interface{}); ok {
				storeInst := s.Store.(*store.Store)

				edits := make([]documents.TextEdit, 0, len(changes))
				for _, change := range changes {
					if changeMap, ok := change.(map[string]interface{}); ok {
						var rangeObj *documents.Range

						if rangeInterface, exists := changeMap["range"]; exists {
							if rangeMap, isMap := rangeInterface.(map[string]interface{}); isMap {
								var start, end documents.Position

								if startMap, exists := rangeMap["start"].(map[string]interface{}); exists {
									startLine, _ := startMap["line"].(float64)
									startChar, _ := startMap["character"].(float64)
									start = documents.Position{
										Line:      int(startLine),
										Character: int(startChar),
									}
								}

								if endMap, exists := rangeMap["end"].(map[string]interface{}); exists {
									endLine, _ := endMap["line"].(float64)
									endChar, _ := endMap["character"].(float64)
									end = documents.Position{
										Line:      int(endLine),
										Character: int(endChar),
									}
								}

								rangeObj = &documents.Range{
									Start: start,
									End:   end,
								}
							}
						}

						newText, _ := changeMap["text"].(string)

						edit := documents.TextEdit{
							Range:   rangeObj,
							NewText: newText,
						}
						edits = append(edits, edit)
					}
				}

				if doc, exists := storeInst.Get(uri); exists {
					rubyDoc := documents.New(doc.URI, doc.Source, doc.Version, doc.LanguageID)
					rubyDoc.Update(edits)
					storeInst.Set(uri, rubyDoc.Source, rubyDoc.Version, rubyDoc.LanguageID)
				}

				s.Logger.(*log.Logger).Printf("Changed document: %s", uri)
			}
		}
	}
}

// HandleDefinition handles textDocument/definition request (Ctrl+Click)
func (s *Server) HandleDefinition(params interface{}) interface{} {
	s.Logger.(*log.Logger).Println("Processing definition request")

	idx, hasIndexer := s.Indexer.(*indexer.Index)
	if !hasIndexer || !idx.IsReady() {
		return []interface{}{}
	}

	uri, pos := extractTextDocumentPosition(params)
	if uri == "" {
		return []interface{}{}
	}

	// Get the document source to find the word at cursor
	storeInst := s.Store.(*store.Store)
	doc, exists := storeInst.Get(uri)
	if !exists {
		return []interface{}{}
	}

	word := indexer.GetWordAtPosition(doc.Source, pos.Line, pos.Character)
	if word == "" {
		return []interface{}{}
	}

	s.Logger.(*log.Logger).Printf("Definition lookup for: %s", word)

	// Remove leading colons (e.g., :user → user, then capitalize)
	cleanWord := strings.TrimPrefix(word, ":")

	// Try direct lookup first
	entries := idx.Lookup(cleanWord)

	// If nothing found, try capitalized version (Rails association → Model)
	if len(entries) == 0 && !isCapitalized(cleanWord) {
		capitalized := capitalize(cleanWord)
		entries = idx.Lookup(capitalized)
	}

	// Try Rails conventions
	if len(entries) == 0 {
		lookupWord := cleanWord
		if !isCapitalized(lookupWord) {
			lookupWord = capitalize(lookupWord)
		}
		entries = idx.LookupByConvention(lookupWord)
	}

	// Filter to only class/module definitions for Ctrl+Click (most common use case)
	var locations []interface{}
	for _, entry := range entries {
		// For class/module/constant lookups, prioritize non-method results
		loc := map[string]interface{}{
			"uri": pathToURI(entry.FilePath),
			"range": map[string]interface{}{
				"start": map[string]interface{}{
					"line":      entry.Line - 1, // LSP is 0-indexed
					"character": entry.Character,
				},
				"end": map[string]interface{}{
					"line":      entry.Line - 1,
					"character": entry.Character + len(entry.Name),
				},
			},
		}
		locations = append(locations, loc)
	}

	if len(locations) == 0 {
		s.Logger.(*log.Logger).Printf("No definition found for: %s", word)
	} else {
		s.Logger.(*log.Logger).Printf("Found %d definition(s) for: %s", len(locations), word)
	}

	return locations
}

// HandleHover handles textDocument/hover request
func (s *Server) HandleHover(params interface{}) interface{} {
	s.Logger.(*log.Logger).Println("Processing hover request")

	idx, hasIndexer := s.Indexer.(*indexer.Index)
	if !hasIndexer || !idx.IsReady() {
		return map[string]interface{}{"contents": ""}
	}

	uri, pos := extractTextDocumentPosition(params)
	if uri == "" {
		return map[string]interface{}{"contents": ""}
	}

	storeInst := s.Store.(*store.Store)
	doc, exists := storeInst.Get(uri)
	if !exists {
		return map[string]interface{}{"contents": ""}
	}

	word := indexer.GetWordAtPosition(doc.Source, pos.Line, pos.Character)
	if word == "" {
		return map[string]interface{}{"contents": ""}
	}

	cleanWord := strings.TrimPrefix(word, ":")

	// Try lookup
	entries := idx.Lookup(cleanWord)
	if len(entries) == 0 && !isCapitalized(cleanWord) {
		entries = idx.Lookup(capitalize(cleanWord))
	}
	if len(entries) == 0 {
		lookupWord := cleanWord
		if !isCapitalized(lookupWord) {
			lookupWord = capitalize(lookupWord)
		}
		entries = idx.LookupByConvention(lookupWord)
	}

	if len(entries) == 0 {
		return map[string]interface{}{"contents": ""}
	}

	// Build hover markdown
	var mdParts []string
	for _, entry := range entries {
		typeStr := indexer.SymbolTypeString(entry.Type)
		relPath := entry.FilePath
		if s.GlobalState.WorkspacePath != "" {
			if rel, err := filepath.Rel(s.GlobalState.WorkspacePath, entry.FilePath); err == nil {
				relPath = rel
			}
		}

		header := fmt.Sprintf("```ruby\n%s %s\n```", typeStr, entry.FullyQualifiedName)
		detail := fmt.Sprintf("**Defined in:** `%s:%d`", relPath, entry.Line)

		extra := ""
		if entry.Detail != "" {
			switch entry.Type {
			case indexer.SymbolClass:
				extra = fmt.Sprintf("\n\n**Inherits from:** `%s`", entry.Detail)
			case indexer.SymbolAssociation:
				extra = fmt.Sprintf("\n\n**Association type:** `%s`", entry.Detail)
			case indexer.SymbolAttrAccessor:
				extra = fmt.Sprintf("\n\n**Accessor type:** `%s`", entry.Detail)
			case indexer.SymbolScope:
				extra = "\n\n**Type:** ActiveRecord scope"
			}
		}

		mdParts = append(mdParts, header+"\n\n"+detail+extra)
	}

	return map[string]interface{}{
		"contents": map[string]interface{}{
			"kind":  "markdown",
			"value": strings.Join(mdParts, "\n\n---\n\n"),
		},
	}
}

// HandleCompletion handles textDocument/completion request
func (s *Server) HandleCompletion(params interface{}) interface{} {
	s.Logger.(*log.Logger).Println("Processing completion request")

	idx, hasIndexer := s.Indexer.(*indexer.Index)
	if !hasIndexer || !idx.IsReady() {
		return map[string]interface{}{
			"isIncomplete": false,
			"items":        []interface{}{},
		}
	}

	uri, pos := extractTextDocumentPosition(params)
	if uri == "" {
		return map[string]interface{}{
			"isIncomplete": false,
			"items":        []interface{}{},
		}
	}

	storeInst := s.Store.(*store.Store)
	doc, exists := storeInst.Get(uri)
	if !exists {
		return map[string]interface{}{
			"isIncomplete": false,
			"items":        []interface{}{},
		}
	}

	word := indexer.GetWordAtPosition(doc.Source, pos.Line, pos.Character)
	if word == "" || len(word) < 2 {
		return map[string]interface{}{
			"isIncomplete": false,
			"items":        []interface{}{},
		}
	}

	entries := idx.PrefixSearch(word)

	var items []interface{}
	seen := make(map[string]bool)

	for _, entry := range entries {
		label := entry.Name
		if seen[label] {
			continue
		}
		seen[label] = true

		kind := indexer.CompletionKindFromType(entry.Type)
		detail := indexer.SymbolTypeString(entry.Type)
		if entry.Parent != "" {
			detail += " in " + entry.Parent
		}

		item := map[string]interface{}{
			"label":  label,
			"kind":   kind,
			"detail": detail,
		}
		items = append(items, item)

		// Cap at 50 results for performance
		if len(items) >= 50 {
			break
		}
	}

	return map[string]interface{}{
		"isIncomplete": len(items) >= 50,
		"items":        items,
	}
}

// HandleDocumentSymbol handles textDocument/documentSymbol request
func (s *Server) HandleDocumentSymbol(params interface{}) interface{} {
	s.Logger.(*log.Logger).Println("Processing document symbol request")

	uri := extractTextDocumentURI(params)
	if uri == "" {
		return []interface{}{}
	}

	filePath := uriToFilePath(uri)
	idx, hasIndexer := s.Indexer.(*indexer.Index)

	var entries []indexer.SymbolEntry
	if hasIndexer {
		entries = idx.GetFileSymbols(filePath)
	}

	// If indexer doesn't have it, parse from store
	if len(entries) == 0 {
		storeInst := s.Store.(*store.Store)
		if doc, exists := storeInst.Get(uri); exists {
			rubyDoc := documents.New(doc.URI, doc.Source, doc.Version, doc.LanguageID)
			ast, err := rubyDoc.Parse()
			if err != nil {
				return []interface{}{}
			}

			var symbols []interface{}
			extractSymbolsFromAST(ast, &symbols)
			return symbols
		}
		return []interface{}{}
	}

	var symbols []interface{}
	for _, entry := range entries {
		kind := indexer.SymbolKindToLSP(entry.Type)
		symbol := map[string]interface{}{
			"name": entry.Name,
			"kind": kind,
			"range": map[string]interface{}{
				"start": map[string]interface{}{
					"line":      entry.Line - 1,
					"character": 0,
				},
				"end": map[string]interface{}{
					"line":      entry.Line - 1,
					"character": entry.Character + len(entry.Name),
				},
			},
			"selectionRange": map[string]interface{}{
				"start": map[string]interface{}{
					"line":      entry.Line - 1,
					"character": entry.Character,
				},
				"end": map[string]interface{}{
					"line":      entry.Line - 1,
					"character": entry.Character + len(entry.Name),
				},
			},
		}

		if entry.Detail != "" {
			symbol["detail"] = entry.Detail
		}

		symbols = append(symbols, symbol)
	}

	return symbols
}

// HandleWorkspaceSymbol handles workspace/symbol request (Ctrl+T)
func (s *Server) HandleWorkspaceSymbol(params interface{}) interface{} {
	s.Logger.(*log.Logger).Println("Processing workspace symbol request")

	idx, hasIndexer := s.Indexer.(*indexer.Index)
	if !hasIndexer || !idx.IsReady() {
		return []interface{}{}
	}

	query := ""
	if paramMap, ok := params.(map[string]interface{}); ok {
		if q, ok := paramMap["query"].(string); ok {
			query = q
		}
	}

	if query == "" || len(query) < 2 {
		return []interface{}{}
	}

	entries := idx.PrefixSearch(query)

	var symbols []interface{}
	for _, entry := range entries {
		kind := indexer.SymbolKindToLSP(entry.Type)

		relPath := entry.FilePath
		if s.GlobalState.WorkspacePath != "" {
			if rel, err := filepath.Rel(s.GlobalState.WorkspacePath, entry.FilePath); err == nil {
				relPath = rel
			}
		}

		symbol := map[string]interface{}{
			"name": entry.FullyQualifiedName,
			"kind": kind,
			"location": map[string]interface{}{
				"uri": pathToURI(entry.FilePath),
				"range": map[string]interface{}{
					"start": map[string]interface{}{
						"line":      entry.Line - 1,
						"character": entry.Character,
					},
					"end": map[string]interface{}{
						"line":      entry.Line - 1,
						"character": entry.Character + len(entry.Name),
					},
				},
			},
			"containerName": relPath,
		}
		symbols = append(symbols, symbol)

		if len(symbols) >= 50 {
			break
		}
	}

	return symbols
}

// HandleFormatting handles textDocument/formatting request
func (s *Server) HandleFormatting(params interface{}) interface{} {
	s.Logger.(*log.Logger).Println("Processing formatting request")
	return []interface{}{}
}

// SendResponse sends a response back to the client
func (s *Server) SendResponse(id interface{}, result interface{}) {
	response := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		s.Logger.(*log.Logger).Printf("Error marshaling response: %v", err)
		return
	}

	fmt.Printf("Content-Length: %d\r\n\r\n%s", len(jsonBytes), jsonBytes)
}

// DispatchOutgoingMessages dispatches messages from the outgoing queue
func (s *Server) DispatchOutgoingMessages() {
	s.Logger.(*log.Logger).Println("Starting message dispatcher...")
}

// Shutdown handles server shutdown
func (s *Server) Shutdown() {
	s.Logger.(*log.Logger).Println("Shutting down Ruby LSP Go server")
	close(s.IncomingQueue)
	close(s.OutgoingQueue)
}

// HandleCancelRequest handles cancellation of requests
func (s *Server) HandleCancelRequest(params interface{}) {
	s.Logger.(*log.Logger).Println("Handling cancel request")
	if paramMap, ok := params.(map[string]interface{}); ok {
		if idParam, exists := paramMap["id"]; exists {
			var id int
			switch v := idParam.(type) {
			case float64:
				id = int(v)
			case string:
				if intVal, err := strconv.Atoi(v); err == nil {
					id = intVal
				}
			}
			s.CancelledRequests[id] = true
		}
	}
}

// --- Helper functions ---

// extractTextDocumentPosition extracts URI and Position from LSP params
func extractTextDocumentPosition(params interface{}) (string, documents.Position) {
	var uri string
	var pos documents.Position

	if paramMap, ok := params.(map[string]interface{}); ok {
		if textDoc, ok := paramMap["textDocument"].(map[string]interface{}); ok {
			uri, _ = textDoc["uri"].(string)
		}
		if posParam, ok := paramMap["position"].(map[string]interface{}); ok {
			if line, ok := posParam["line"].(float64); ok {
				pos.Line = int(line)
			}
			if char, ok := posParam["character"].(float64); ok {
				pos.Character = int(char)
			}
		}
	}

	return uri, pos
}

// extractTextDocumentURI extracts just the URI from params
func extractTextDocumentURI(params interface{}) string {
	if paramMap, ok := params.(map[string]interface{}); ok {
		if textDoc, ok := paramMap["textDocument"].(map[string]interface{}); ok {
			uri, _ := textDoc["uri"].(string)
			return uri
		}
	}
	return ""
}

// uriToFilePath converts a file:// URI to a filesystem path
func uriToFilePath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		parsed, err := url.Parse(uri)
		if err == nil {
			return parsed.Path
		}
		return strings.TrimPrefix(uri, "file://")
	}
	return uri
}

// pathToURI converts a filesystem path to a file:// URI
func pathToURI(path string) string {
	if strings.HasPrefix(path, "/") {
		return "file://" + path
	}
	return "file:///" + path
}

// isCapitalized checks if a string starts with an uppercase letter
func isCapitalized(s string) bool {
	if len(s) == 0 {
		return false
	}
	return s[0] >= 'A' && s[0] <= 'Z'
}

// capitalize converts the first character to uppercase (simple CamelCase for one word)
func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}

	// Handle snake_case → CamelCase
	parts := strings.Split(s, "_")
	var result strings.Builder
	for _, part := range parts {
		if len(part) > 0 {
			result.WriteString(strings.ToUpper(part[:1]) + part[1:])
		}
	}
	return result.String()
}

// extractSymbolsFromAST extracts symbols from the AST for document symbols (fallback)
func extractSymbolsFromAST(node *documents.Node, symbols *[]interface{}) {
	if node.Type == "class" || node.Type == "method" || node.Type == "module" {
		kind := getSymbolKind(node.Type)
		symbol := map[string]interface{}{
			"name": node.Name,
			"kind": kind,
			"range": map[string]interface{}{
				"start": node.Location.Start,
				"end":   node.Location.End,
			},
			"selectionRange": map[string]interface{}{
				"start": node.Location.Start,
				"end":   node.Location.End,
			},
		}
		*symbols = append(*symbols, symbol)
	}

	for _, child := range node.Children {
		extractSymbolsFromAST(child, symbols)
	}
}

// getSymbolKind maps node types to LSP symbol kinds
func getSymbolKind(nodeType string) int {
	switch nodeType {
	case "class":
		return 5
	case "method":
		return 6
	case "module":
		return 2
	default:
		return 1
	}
}
