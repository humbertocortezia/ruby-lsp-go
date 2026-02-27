// main.go - Main entry point for the Ruby Language Server in Go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/humberto/ruby-lsp-go/indexer"
	"github.com/humberto/ruby-lsp-go/lsp"
	"github.com/humberto/ruby-lsp-go/store"
)

func main() {
	logger := log.New(os.Stderr, "[RubyLSP-Go] ", log.LstdFlags)
	
	// Create the server
	globalState := &lsp.GlobalState{
		WorkspaceURI:       fmt.Sprintf("file://%s", os.Getenv("PWD")),
		Formatter:          "auto",
		TestLibrary:        "minitest",
		HasTypeChecker:     false,
		ClientCapabilities: make(map[string]interface{}),
		EnabledFeatures:    make(map[string]bool),
		Mutex:              sync.Mutex{},
	}
	
	storeInstance := store.New(globalState)
	
	server := &lsp.Server{
		GlobalState:       globalState,
		Store:             storeInstance,
		IncomingQueue:     make(chan lsp.Message, 100),
		OutgoingQueue:     make(chan lsp.Message, 100),
		CancelledRequests: make(map[int]bool),
		Logger:            logger,
	}

	// Start the outgoing message dispatcher
	go server.DispatchOutgoingMessages()

	// Read initialization message if provided
	reader := bufio.NewReader(os.Stdin)
	
	// Handle LSP communication over stdin/stdout
	scanner := NewMessageScanner(reader)
	
	for {
		msg, err := scanner.Scan()
		if err != nil {
			if err == io.EOF {
				break
			}
			logger.Printf("Error reading message: %v", err)
			continue
		}

		// Route messages based on method type
		switch msg.Method {
		case "initialize":
			// Extract rootUri for workspace indexing
			if paramMap, ok := msg.Params.(map[string]interface{}); ok {
				if rootURI, ok := paramMap["rootUri"].(string); ok {
					globalState.WorkspaceURI = rootURI
					globalState.WorkspacePath = uriToPath(rootURI)
				} else if rootPath, ok := paramMap["rootPath"].(string); ok {
					globalState.WorkspacePath = rootPath
					globalState.WorkspaceURI = "file://" + rootPath
				}
			}

			// Start workspace indexing in background
			if globalState.WorkspacePath != "" {
				idx := indexer.New(globalState.WorkspacePath, logger)
				server.Indexer = idx
				go idx.BuildIndex()
			}

			response := server.HandleInitialize(msg.Params)
			server.SendResponse(msg.ID, response)
		case "initialized":
			server.HandleInitialized()
		case "textDocument/didOpen":
			server.HandleDidOpen(msg.Params)
		case "textDocument/didClose":
			server.HandleDidClose(msg.Params)
		case "textDocument/didChange":
			server.HandleDidChange(msg.Params)
		case "textDocument/didSave":
			// Re-index the saved file
			if paramMap, ok := msg.Params.(map[string]interface{}); ok {
				if textDoc, ok := paramMap["textDocument"].(map[string]interface{}); ok {
					if uri, ok := textDoc["uri"].(string); ok {
						filePath := uriToPath(uri)
						if idx, ok := server.Indexer.(*indexer.Index); ok {
							go idx.UpdateFile(filePath)
						}
					}
				}
			}
		case "textDocument/completion":
			result := server.HandleCompletion(msg.Params)
			server.SendResponse(msg.ID, result)
		case "textDocument/hover":
			result := server.HandleHover(msg.Params)
			server.SendResponse(msg.ID, result)
		case "textDocument/definition":
			result := server.HandleDefinition(msg.Params)
			server.SendResponse(msg.ID, result)
		case "textDocument/documentSymbol":
			result := server.HandleDocumentSymbol(msg.Params)
			server.SendResponse(msg.ID, result)
		case "textDocument/formatting":
			result := server.HandleFormatting(msg.Params)
			server.SendResponse(msg.ID, result)
		case "workspace/symbol":
			result := server.HandleWorkspaceSymbol(msg.Params)
			server.SendResponse(msg.ID, result)
		case "shutdown":
			server.Shutdown()
			server.SendResponse(msg.ID, nil)
		case "exit":
			return
		case "$/cancelRequest":
			server.HandleCancelRequest(msg.Params)
		default:
			// Queue other messages for background processing
			server.IncomingQueue <- msg
		}
	}
}

// MessageScanner handles LSP protocol message scanning (Content-Length headers)
type MessageScanner struct {
	reader *bufio.Reader
}

func NewMessageScanner(reader *bufio.Reader) *MessageScanner {
	return &MessageScanner{reader: reader}
}

func (ms *MessageScanner) Scan() (lsp.Message, error) {
	var msg lsp.Message
	
	// Read Content-Length header
	header, err := ms.reader.ReadString('\n')
	if err != nil {
		return msg, err
	}

	var contentLength int
	if _, err := fmt.Sscanf(header, "Content-Length: %d\r", &contentLength); err != nil {
		return msg, fmt.Errorf("failed to parse Content-Length: %v", err)
	}

	// Skip empty line
	ms.reader.ReadString('\n')

	// Read the actual JSON content
	buf := make([]byte, contentLength)
	_, err = io.ReadFull(ms.reader, buf)
	if err != nil {
		return msg, err
	}

	// Parse the JSON
	var req map[string]interface{}
	if err := json.Unmarshal(buf, &req); err != nil {
		return msg, fmt.Errorf("failed to parse JSON: %v", err)
	}

	// Extract common fields
	if id, ok := req["id"]; ok {
		switch v := id.(type) {
		case float64:
			msg.ID = int(v)
		case string:
			// Handle string IDs if needed
			msg.ID = v
		}
	}
	
	if method, ok := req["method"]; ok {
		msg.Method = method.(string)
	}
	
	if params, ok := req["params"]; ok {
		msg.Params = params
	}

	return msg, nil
}

// SendJSON writes a message to stdout in LSP format
func SendJSON(w io.Writer, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	// Write the Content-Length header followed by the content
	fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(data))
	_, err = w.Write(data)
	return err
}

// uriToPath converts a file:// URI to a local filesystem path
func uriToPath(uri string) string {
	if strings.HasPrefix(uri, "file://") {
		parsed, err := url.Parse(uri)
		if err == nil {
			return parsed.Path
		}
		return strings.TrimPrefix(uri, "file://")
	}
	return uri
}
