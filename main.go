// main.go - Entry point for Ruby LSP Go
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

	"github.com/humberto/ruby-lsp-go/lsp"
	"github.com/humberto/ruby-lsp-go/store"
	"github.com/humberto/ruby-lsp-go/workspace"
)

func main() {
	logger := log.New(os.Stderr, "[RubyLSP-Go] ", log.LstdFlags)

	state := &workspace.State{
		WorkspaceURI:       fmt.Sprintf("file://%s", os.Getenv("PWD")),
		Formatter:          "auto",
		TestLibrary:        "minitest",
		ClientCapabilities: make(map[string]interface{}),
		EnabledFeatures:    make(map[string]bool),
	}

	storeInstance := store.New(state)

	server := &lsp.Server{
		State:             state,
		Store:             storeInstance,
		IncomingQueue:     make(chan lsp.Message, 256),
		OutgoingQueue:     make(chan lsp.Message, 256),
		CancelledRequests: make(map[int]bool),
		Logger:            logger,
	}

	// Start worker pool and outgoing dispatcher
	server.StartWorkers()
	go server.DispatchOutgoingMessages()

	reader := bufio.NewReader(os.Stdin)
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

		switch msg.Method {
		// Lifecycle — handled on main thread
		case "initialize":
			response := server.HandleInitialize(msg.Params)
			server.SendResponse(msg.ID, response)
		case "initialized":
			server.HandleInitialized()
		case "shutdown":
			server.Shutdown()
			server.SendResponse(msg.ID, nil)
		case "exit":
			return
		case "$/cancelRequest":
			server.HandleCancelRequest(msg.Params)

		// Document notifications — handled on main thread (fast)
		case "textDocument/didOpen":
			server.HandleDidOpen(msg.Params)
		case "textDocument/didClose":
			server.HandleDidClose(msg.Params)
		case "textDocument/didChange":
			server.HandleDidChange(msg.Params)
		case "textDocument/didSave":
			server.HandleDidSave(msg.Params)
		case "workspace/didChangeWatchedFiles":
			server.HandleWatchedFiles(msg.Params)

		// All requests — dispatched to worker pool
		default:
			if msg.ID != nil {
				server.Enqueue(msg)
			}
		}
	}
}

// MessageScanner reads LSP Content-Length framed messages.
type MessageScanner struct {
	reader *bufio.Reader
}

func NewMessageScanner(reader *bufio.Reader) *MessageScanner {
	return &MessageScanner{reader: reader}
}

func (ms *MessageScanner) Scan() (lsp.Message, error) {
	var msg lsp.Message

	contentLength := -1
	for {
		header, err := ms.reader.ReadString('\n')
		if err != nil {
			return msg, err
		}
		trimmed := strings.TrimSpace(header)
		if trimmed == "" {
			break
		}
		if strings.HasPrefix(strings.ToLower(trimmed), "content-length:") {
			if _, err := fmt.Sscanf(trimmed, "Content-Length: %d", &contentLength); err != nil {
				return msg, fmt.Errorf("failed to parse Content-Length: %v", err)
			}
		}
	}
	if contentLength < 0 || contentLength > 64*1024*1024 {
		return msg, fmt.Errorf("invalid Content-Length: %d", contentLength)
	}

	buf := make([]byte, contentLength)
	if _, err := io.ReadFull(ms.reader, buf); err != nil {
		return msg, err
	}

	var req map[string]interface{}
	if err := json.Unmarshal(buf, &req); err != nil {
		return msg, fmt.Errorf("failed to parse JSON: %v", err)
	}

	if id, ok := req["id"]; ok {
		msg.ID = id
	}
	if method, ok := req["method"]; ok {
		msg.Method, _ = method.(string)
	}
	if params, ok := req["params"]; ok {
		msg.Params = params
	}

	return msg, nil
}

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
