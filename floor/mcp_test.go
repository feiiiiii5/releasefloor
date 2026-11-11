package floor

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMCPServerProtocol(t *testing.T) {
	// Prepare test files
	dir := t.TempDir()
	elfData := fixtureELF([]fixtureNode{
		{Provider: "libc.so.6", Version: "GLIBC_2.28", Symbol: "foo"},
	}, false)
	elfPath := filepath.Join(dir, "app.elf")
	if err := os.WriteFile(elfPath, elfData, 0600); err != nil {
		t.Fatal(err)
	}

	var in bytes.Buffer
	var out, errOut bytes.Buffer

	writeMsg := func(v any) {
		data, _ := json.Marshal(v)
		in.Write(data)
		in.WriteByte('\n')
	}

	// 1. initialize
	writeMsg(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
		},
	})

	// 2. tools/list
	writeMsg(map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
	})

	// 3. tools/call: releasefloor_scan
	writeMsg(map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "releasefloor_scan",
			"arguments": map[string]any{
				"artifact": elfPath,
			},
		},
	})

	// 4. tools/call: releasefloor_init_policy
	writeMsg(map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "releasefloor_init_policy",
			"arguments": map[string]any{
				"artifacts": []string{elfPath},
			},
		},
	})

	// 5. unknown method
	writeMsg(map[string]any{
		"jsonrpc": "2.0",
		"id":      5,
		"method":  "non_existent_method",
	})

	// Run MCP server loop
	if err := RunMCPServer(&in, &out, &errOut); err != nil {
		t.Fatalf("RunMCPServer failed: %v", err)
	}

	// Parse outputs
	lines := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	if len(lines) != 5 {
		t.Fatalf("expected 5 responses, got %d: %s", len(lines), out.String())
	}

	// Verify initialize response
	var initResp mcpResponse
	if err := json.Unmarshal(lines[0], &initResp); err != nil || initResp.Error != nil {
		t.Fatalf("bad init response: %v, %+v", err, initResp)
	}

	// Verify tools/list response
	var listResp struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  struct {
			Tools []mcpToolParam `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(lines[1], &listResp); err != nil || len(listResp.Result.Tools) != 4 {
		t.Fatalf("bad tools/list response: %v, %+v", err, listResp)
	}

	// Verify releasefloor_scan call response
	var scanCallResp struct {
		JSONRPC string        `json:"jsonrpc"`
		ID      int           `json:"id"`
		Result  mcpToolResult `json:"result"`
	}
	if err := json.Unmarshal(lines[2], &scanCallResp); err != nil || scanCallResp.Result.IsError {
		t.Fatalf("bad scan call response: %v, %+v", err, scanCallResp)
	}
	var scanReport Report
	if err := json.Unmarshal([]byte(scanCallResp.Result.Content[0].Text), &scanReport); err != nil || scanReport.Status != "pass" {
		t.Fatalf("scan call report invalid: %v, %+v", err, scanReport)
	}

	// Verify releasefloor_init_policy response
	var initPolicyResp struct {
		JSONRPC string        `json:"jsonrpc"`
		ID      int           `json:"id"`
		Result  mcpToolResult `json:"result"`
	}
	if err := json.Unmarshal(lines[3], &initPolicyResp); err != nil || initPolicyResp.Result.IsError {
		t.Fatalf("bad init_policy call response: %v, %+v", err, initPolicyResp)
	}
	var derivedPolicy Policy
	if err := json.Unmarshal([]byte(initPolicyResp.Result.Content[0].Text), &derivedPolicy); err != nil || len(derivedPolicy.Rules) != 1 {
		t.Fatalf("init_policy response invalid: %v, %+v", err, derivedPolicy)
	}

	// Verify unknown method error
	var errResp mcpResponse
	if err := json.Unmarshal(lines[4], &errResp); err != nil || errResp.Error == nil || errResp.Error.Code != -32601 {
		t.Fatalf("expected method not found error: %+v", errResp)
	}
}
