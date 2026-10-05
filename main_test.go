package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Run the real main in a subprocess of the test executable. This also keeps the
// server instrumented when the parent suite runs with -race.
func TestLSPSubprocess(t *testing.T) {
	if os.Getenv("RUBY_LSP_GO_TEST_SERVER") == "1" {
		main()
		os.Exit(0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestLSPSubprocess$")
	// Diagnostics use the Go fallback, independent of local RuboCop installs.
	cmd.Env = append(os.Environ(), "RUBY_LSP_GO_TEST_SERVER=1", "PATH="+t.TempDir())
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })

	responses := make(chan map[string]interface{}, 32)
	readErrors := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(out)
		for {
			length := 0
			for {
				header, err := reader.ReadString('\n')
				if err != nil {
					readErrors <- err
					return
				}
				if strings.TrimSpace(header) == "" {
					break
				}
				if strings.HasPrefix(header, "Content-Length:") {
					if _, err := fmt.Sscanf(header, "Content-Length: %d", &length); err != nil {
						readErrors <- err
						return
					}
				}
			}
			if length <= 0 || length > 1024*1024 {
				readErrors <- fmt.Errorf("invalid response length: %d", length)
				return
			}
			body := make([]byte, length)
			if _, err := io.ReadFull(reader, body); err != nil {
				readErrors <- err
				return
			}
			var response map[string]interface{}
			if err := json.Unmarshal(body, &response); err != nil {
				readErrors <- err
				return
			}
			responses <- response
		}
	}()
	logLines := make(chan string, 128)
	go func() {
		defer close(logLines)
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			logLines <- scanner.Text()
		}
	}()
	var logs []string
	waitLog := func(fragment string, count int) {
		t.Helper()
		for {
			found := 0
			for _, line := range logs {
				if strings.Contains(line, fragment) {
					found++
				}
			}
			if found >= count {
				return
			}
			select {
			case line, ok := <-logLines:
				if !ok {
					t.Fatalf("server stopped waiting for %q; logs: %v", fragment, logs)
				}
				logs = append(logs, line)
			case <-ctx.Done():
				t.Fatalf("timeout waiting for %q; logs: %v", fragment, logs)
			}
		}
	}
	send := func(id int, method string, params interface{}) {
		t.Helper()
		message := map[string]interface{}{"jsonrpc": "2.0", "method": method, "params": params}
		if id != 0 {
			message["id"] = id
		}
		body, err := json.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fmt.Fprintf(in, "Content-Length: %d\r\n\r\n%s", len(body), body); err != nil {
			t.Fatal(err)
		}
	}
	id := 0
	requestResponse := func(method string, params interface{}) map[string]interface{} {
		t.Helper()
		id++
		send(id, method, params)
		select {
		case response := <-responses:
			if response["id"] != float64(id) {
				t.Fatalf("unexpected response: %v", response)
			}
			return response
		case err := <-readErrors:
			t.Fatalf("server response failed: %v; logs: %v", err, logs)
		case <-ctx.Done():
			t.Fatalf("response timeout: %s", method)
		}
		return nil
	}
	request := func(method string, params interface{}) interface{} {
		t.Helper()
		response := requestResponse(method, params)
		if response["error"] != nil {
			t.Fatalf("unexpected error response: %v", response)
		}
		return response["result"]
	}
	assertRequestError := func(method string, params interface{}, code int) {
		t.Helper()
		response := requestResponse(method, params)
		failure, ok := response["error"].(map[string]interface{})
		if !ok || failure["code"] != float64(code) {
			t.Fatalf("wrong JSON-RPC error: %v", response)
		}
		if _, hasResult := response["result"]; hasResult {
			t.Fatalf("error also returned a result: %v", response)
		}
	}
	assertRequestError("workspace/executeCommand", map[string]interface{}{"command": "rubyLspGo.reindexWorkspace"}, -32002)
	dir := t.TempDir()
	bad := filepath.Join(dir, "a_bad.rb")
	good := filepath.Join(dir, "z_good.rb")
	write := func(path, source string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	uri := func(path string) string { return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String() }
	write(bad, "class Unpublished\nend\n\"unfinished")
	write(good, "class Survives\nend\n")
	initialize := request("initialize", map[string]interface{}{"rootUri": uri(dir)}).(map[string]interface{})
	manifestBytes, err := os.ReadFile("vscode-extension/package.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if initialize["serverInfo"].(map[string]interface{})["version"] != manifest.Version {
		t.Fatalf("server and extension versions differ: %v", initialize["serverInfo"])
	}
	send(0, "initialized", map[string]interface{}{})
	waitLog("Failed to index "+bad, 1)
	waitLog("AST indexing complete", 1)
	assertSymbol := func(name string, count int) {
		t.Helper()
		result := request("workspace/symbol", map[string]interface{}{"query": name})
		symbols, ok := result.([]interface{})
		if !ok {
			t.Fatalf("invalid workspace/symbol response: %v", result)
		}
		found := 0
		for _, raw := range symbols {
			if raw.(map[string]interface{})["name"] == name {
				found++
			}
		}
		if found != count {
			t.Fatalf("symbol %s: want %d, got %v", name, count, result)
		}
	}
	assertSymbol("Survives", 1)
	assertSymbol("Unpublished", 0)
	notifyChanged := func() {
		send(0, "workspace/didChangeWatchedFiles", map[string]interface{}{"changes": []interface{}{map[string]interface{}{"uri": uri(bad), "type": 2}}})
	}
	write(bad, "class Changed\nend\n")
	notifyChanged()
	waitLog("Re-indexed: "+bad, 1)
	assertSymbol("Changed", 1)
	write(bad, `/unfinished\`)
	notifyChanged()
	waitLog("Failed to index "+bad, 2)
	assertSymbol("Changed", 0)
	assertSymbol("Survives", 1)
	write(bad, "class Changed\nend\n")
	notifyChanged()
	waitLog("Re-indexed: "+bad, 2)
	assertSymbol("Changed", 1)

	write(bad, `:"unfinished`)
	write(good, "class Rebuilt\nend\n")
	result := request("rubyLspGo/reindexWorkspace", nil)
	if result.(map[string]interface{})["started"] != true {
		t.Fatalf("reindex not started: %v", result)
	}
	waitLog("AST indexing complete", 2)
	waitLog("Failed to index "+bad, 3)
	assertSymbol("Changed", 0)
	assertSymbol("Survives", 0)
	assertSymbol("Rebuilt", 1)

	// Exercise the standard request actually sent by LanguageClient for the
	// command advertised in initialize, not just the backwards-compatible alias.
	write(good, "class RebuiltViaCommand\nend\n")
	result = request("workspace/executeCommand", map[string]interface{}{"command": "rubyLspGo.reindexWorkspace"})
	if result.(map[string]interface{})["started"] != true {
		t.Fatalf("standard reindex not started: %v", result)
	}
	waitLog("AST indexing complete", 3)
	assertSymbol("Rebuilt", 0)
	assertSymbol("RebuiltViaCommand", 1)
	assertRequestError("workspace/executeCommand", map[string]interface{}{"command": "unknown"}, -32602)
	assertRequestError("workspace/executeCommand", nil, -32602)
	assertRequestError("unknown/request", nil, -32601)

	// Opening and diagnosing the same broken source must also leave the
	// process responsive, without returning an AST from a previous version.
	send(0, "textDocument/didOpen", map[string]interface{}{"textDocument": map[string]interface{}{
		"uri": uri(bad), "text": `"unfinished`, "version": 1, "languageId": "ruby",
	}})
	result = request("textDocument/diagnostic", map[string]interface{}{"textDocument": map[string]interface{}{"uri": uri(bad)}})
	if len(result.(map[string]interface{})["items"].([]interface{})) != 1 {
		t.Fatalf("missing diagnostic: %v", result)
	}
	assertSymbol("RebuiltViaCommand", 1)
	request("shutdown", nil)
	send(0, "exit", nil)
	_ = in.Close()
	// Drain both readers before Wait closes their pipes. Exit code 0 also
	// proves the race-instrumented subprocess reported no races.
	select {
	case err := <-readErrors:
		if err != io.EOF {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("server did not close stdout")
	}
	for range logLines {
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("server did not exit normally: %v", err)
	}
}
