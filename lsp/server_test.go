package lsp

import "testing"

func TestExecuteCommandRejectsInvalidOrUnavailableCommands(t *testing.T) {
	server := &Server{}
	for _, tc := range []struct {
		params interface{}
		code   int
	}{
		{nil, -32602}, {map[string]interface{}{}, -32602},
		{map[string]interface{}{"command": 42}, -32602},
		{map[string]interface{}{"command": "unknown"}, -32602},
		{map[string]interface{}{"command": "rubyLspGo.reindexWorkspace"}, -32002},
	} {
		result, err := server.ExecuteCommand(tc.params)
		if result != nil || err == nil || err.Code != tc.code {
			t.Fatalf("unexpected command result: %v %v", result, err)
		}
	}
}
