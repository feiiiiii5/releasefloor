package floor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// MCP JSON-RPC 2.0 structures
type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id"`
	Result  any       `json:"result,omitempty"`
	Error   *mcpError `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpToolResult struct {
	Content []mcpToolContent `json:"content"`
	IsError bool             `json:"isError,omitempty"`
}

type mcpToolParam struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// RunMCPServer runs the Model Context Protocol stdio server loop.
// All JSON-RPC frames are read from in and written to out.
// All human diagnostics and debug messages are written to errOut.
func RunMCPServer(in io.Reader, out io.Writer, errOut io.Writer) error {
	reader := bufio.NewReader(in)
	send := func(resp mcpResponse) error {
		data, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		data = append(data, '\n')
		_, err = out.Write(data)
		return err
	}

	for {
		line, err := reader.ReadBytes('\n')
		if len(line) == 0 && err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		var req mcpRequest
		if unmarshalErr := json.Unmarshal(line, &req); unmarshalErr != nil {
			if len(line) > 0 {
				send(mcpResponse{
					JSONRPC: "2.0",
					ID:      nil,
					Error:   &mcpError{Code: -32700, Message: "Parse error: invalid JSON"},
				})
			}
			if err == io.EOF {
				return nil
			}
			continue
		}

		switch req.Method {
		case "initialize":
			send(mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities": map[string]any{
						"tools": map[string]any{},
					},
					"serverInfo": map[string]any{
						"name":    "releasefloor",
						"version": Version,
					},
				},
			})

		case "notifications/initialized":
			// Notification, no response required

		case "ping":
			send(mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  map[string]any{},
			})

		case "tools/list":
			tools := []mcpToolParam{
				{
					Name:        "releasefloor_scan",
					Description: "Inspect an ELF/Mach-O release artifact (.tar.gz, .zip, .whl, .jar, or raw binary) without executing it. Returns complete runtime inventory, GLIBC/GLIBCXX/CXXABI floors, and minimum macOS deployment targets.",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"artifact": map[string]any{
								"type":        "string",
								"description": "Path to the archive or binary to scan",
							},
							"strip_components": map[string]any{
								"type":        "integer",
								"description": "Leading archive path components to strip (default 0)",
							},
						},
						"required": []string{"artifact"},
					},
				},
				{
					Name:        "releasefloor_compare",
					Description: "Compare a candidate release artifact against a trusted prior release. Detects raised requirement floors, raised minimum OS versions, removed binaries, or dropped architectures.",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"old_artifact": map[string]any{
								"type":        "string",
								"description": "Path to the baseline / prior release archive",
							},
							"new_artifact": map[string]any{
								"type":        "string",
								"description": "Path to the candidate release archive",
							},
							"strip_components": map[string]any{
								"type":        "integer",
								"description": "Leading archive path components to strip",
							},
						},
						"required": []string{"old_artifact", "new_artifact"},
					},
				},
				{
					Name:        "releasefloor_check",
					Description: "Gate a multi-platform release matrix against declared policy ceilings. Blocks if any binary violates ceilings or an expected target is missing.",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"policy_path": map[string]any{
								"type":        "string",
								"description": "Path to JSON policy file (schema releasefloor.policy.v1)",
							},
							"artifacts": map[string]any{
								"type":        "array",
								"items":       map[string]any{"type": "string"},
								"description": "Paths to artifacts in the release matrix",
							},
							"strip_components": map[string]any{
								"type":        "integer",
								"description": "Leading archive path components to strip",
							},
						},
						"required": []string{"policy_path", "artifacts"},
					},
				},
				{
					Name:        "releasefloor_init_policy",
					Description: "Synthesize and generate a canonical releasefloor.policy.v1 JSON specification matching the current ceilings of baseline artifacts.",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"artifacts": map[string]any{
								"type":        "array",
								"items":       map[string]any{"type": "string"},
								"description": "Paths to baseline artifacts to derive policy ceilings from",
							},
							"strip_components": map[string]any{
								"type":        "integer",
								"description": "Leading archive path components to strip",
							},
						},
						"required": []string{"artifacts"},
					},
				},
			}
			send(mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: map[string]any{
					"tools": tools,
				},
			})

		case "tools/call":
			result := handleToolCall(req.Params)
			send(mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  result,
			})

		default:
			send(mcpResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &mcpError{
					Code:    -32601,
					Message: fmt.Sprintf("Method not found: %s", req.Method),
				},
			})
		}

		if err == io.EOF {
			return nil
		}
	}
}

func handleToolCall(paramsRaw json.RawMessage) mcpToolResult {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(paramsRaw, &call); err != nil {
		return mcpToolResult{
			Content: []mcpToolContent{{Type: "text", Text: "invalid tool call parameters"}},
			IsError: true,
		}
	}

	switch call.Name {
	case "releasefloor_scan":
		var args struct {
			Artifact        string `json:"artifact"`
			StripComponents int    `json:"strip_components"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil || args.Artifact == "" {
			return mcpToolResult{
				Content: []mcpToolContent{{Type: "text", Text: "invalid arguments: 'artifact' is required"}},
				IsError: true,
			}
		}
		limits := DefaultLimits()
		limits.StripComponents = args.StripComponents
		inv, err := Scan(args.Artifact, limits)
		if err != nil {
			return mcpToolResult{
				Content: []mcpToolContent{{Type: "text", Text: fmt.Sprintf("scan error: %v", err)}},
				IsError: true,
			}
		}
		report := ScanReport(inv)
		data, _ := json.MarshalIndent(report, "", "  ")
		return mcpToolResult{
			Content: []mcpToolContent{{Type: "text", Text: string(data)}},
			IsError: report.Status != "pass",
		}

	case "releasefloor_compare":
		var args struct {
			OldArtifact     string `json:"old_artifact"`
			NewArtifact     string `json:"new_artifact"`
			StripComponents int    `json:"strip_components"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil || args.OldArtifact == "" || args.NewArtifact == "" {
			return mcpToolResult{
				Content: []mcpToolContent{{Type: "text", Text: "invalid arguments: 'old_artifact' and 'new_artifact' are required"}},
				IsError: true,
			}
		}
		limits := DefaultLimits()
		limits.StripComponents = args.StripComponents
		oldInv, err := Scan(args.OldArtifact, limits)
		if err != nil {
			return mcpToolResult{
				Content: []mcpToolContent{{Type: "text", Text: fmt.Sprintf("cannot scan old_artifact: %v", err)}},
				IsError: true,
			}
		}
		newInv, err := Scan(args.NewArtifact, limits)
		if err != nil {
			return mcpToolResult{
				Content: []mcpToolContent{{Type: "text", Text: fmt.Sprintf("cannot scan new_artifact: %v", err)}},
				IsError: true,
			}
		}
		report := Compare(oldInv, newInv)
		data, _ := json.MarshalIndent(report, "", "  ")
		return mcpToolResult{
			Content: []mcpToolContent{{Type: "text", Text: string(data)}},
			IsError: report.Status != "pass",
		}

	case "releasefloor_check":
		var args struct {
			PolicyPath      string   `json:"policy_path"`
			Artifacts       []string `json:"artifacts"`
			StripComponents int      `json:"strip_components"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil || args.PolicyPath == "" || len(args.Artifacts) == 0 {
			return mcpToolResult{
				Content: []mcpToolContent{{Type: "text", Text: "invalid arguments: 'policy_path' and 'artifacts' are required"}},
				IsError: true,
			}
		}
		policyData, err := os.ReadFile(args.PolicyPath)
		if err != nil {
			return mcpToolResult{
				Content: []mcpToolContent{{Type: "text", Text: fmt.Sprintf("cannot read policy file: %v", err)}},
				IsError: true,
			}
		}
		policy, err := ParsePolicy(policyData)
		if err != nil {
			return mcpToolResult{
				Content: []mcpToolContent{{Type: "text", Text: fmt.Sprintf("invalid policy: %v", err)}},
				IsError: true,
			}
		}
		limits := DefaultLimits()
		limits.StripComponents = args.StripComponents
		var inventories []Inventory
		for _, art := range args.Artifacts {
			inv, err := Scan(art, limits)
			if err != nil {
				return mcpToolResult{
					Content: []mcpToolContent{{Type: "text", Text: fmt.Sprintf("cannot scan artifact %q: %v", art, err)}},
					IsError: true,
				}
			}
			inventories = append(inventories, inv)
		}
		report, err := Check(inventories, policy)
		if err != nil {
			return mcpToolResult{
				Content: []mcpToolContent{{Type: "text", Text: fmt.Sprintf("check error: %v", err)}},
				IsError: true,
			}
		}
		data, _ := json.MarshalIndent(report, "", "  ")
		return mcpToolResult{
			Content: []mcpToolContent{{Type: "text", Text: string(data)}},
			IsError: report.Status != "pass",
		}

	case "releasefloor_init_policy":
		var args struct {
			Artifacts       []string `json:"artifacts"`
			StripComponents int      `json:"strip_components"`
		}
		if err := json.Unmarshal(call.Arguments, &args); err != nil || len(args.Artifacts) == 0 {
			return mcpToolResult{
				Content: []mcpToolContent{{Type: "text", Text: "invalid arguments: 'artifacts' array is required"}},
				IsError: true,
			}
		}
		limits := DefaultLimits()
		limits.StripComponents = args.StripComponents
		var inventories []Inventory
		for _, art := range args.Artifacts {
			inv, err := Scan(art, limits)
			if err != nil {
				return mcpToolResult{
					Content: []mcpToolContent{{Type: "text", Text: fmt.Sprintf("cannot scan artifact %q: %v", art, err)}},
					IsError: true,
				}
			}
			inventories = append(inventories, inv)
		}
		policy, err := DerivePolicy(inventories)
		if err != nil {
			return mcpToolResult{
				Content: []mcpToolContent{{Type: "text", Text: fmt.Sprintf("cannot derive policy: %v", err)}},
				IsError: true,
			}
		}
		data, _ := json.MarshalIndent(policy, "", "  ")
		return mcpToolResult{
			Content: []mcpToolContent{{Type: "text", Text: string(data)}},
			IsError: false,
		}

	default:
		return mcpToolResult{
			Content: []mcpToolContent{{Type: "text", Text: fmt.Sprintf("unknown tool: %s", call.Name)}},
			IsError: true,
		}
	}
}
