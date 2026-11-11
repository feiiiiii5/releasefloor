package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/feiiiiii5/releasefloor/floor"
)

func machoFixture(minor uint32) []byte {
	b := make([]byte, 56)
	le := binary.LittleEndian
	le.PutUint32(b, 0xfeedfacf)
	le.PutUint32(b[4:], 0x0100000c)
	le.PutUint32(b[12:], 2)
	le.PutUint32(b[16:], 1)
	le.PutUint32(b[20:], 24)
	le.PutUint32(b[32:], 0x32)
	le.PutUint32(b[36:], 24)
	le.PutUint32(b[40:], 1)
	le.PutUint32(b[44:], minor<<16)
	le.PutUint32(b[48:], 26<<16)
	return b
}

func TestCommandEndToEndNoClobberAndExitCodes(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old")
	current := filepath.Join(dir, "new")
	for name, data := range map[string][]byte{old: machoFixture(12), current: machoFixture(14)} {
		if err := os.WriteFile(name, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	var out, errOut bytes.Buffer
	rc := run([]string{"compare", "--format", "json", old, current}, &out, &errOut)
	if rc != 1 {
		t.Fatalf("wanted blocked code 1, got %d: %s", rc, errOut.String())
	}
	var r floor.Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if r.Status != "blocked" {
		t.Fatalf("not a blocked report: %+v", r)
	}
	reportPath := filepath.Join(dir, "report.html")
	out.Reset()
	errOut.Reset()
	if rc := run([]string{"compare", "--format", "html", "--output", reportPath, old, current}, &out, &errOut); rc != 1 {
		t.Fatalf("HTML gate: %d %s", rc, errOut.String())
	}
	before, _ := os.ReadFile(reportPath)
	if rc := run([]string{"scan", "--output", reportPath, old}, &out, &errOut); rc != 2 {
		t.Fatalf("existing report overwritten: %d", rc)
	}
	after, _ := os.ReadFile(reportPath)
	if !bytes.Equal(before, after) {
		t.Fatal("existing report changed")
	}
	if rc := run([]string{"scan", "--output", old, old}, &out, &errOut); rc != 2 {
		t.Fatal("source overwrite accepted")
	}
	if got, _ := os.ReadFile(old); !bytes.Equal(got, machoFixture(12)) {
		t.Fatal("source bytes changed")
	}
	out.Reset()
	if rc := run([]string{"scan", "--format", "json", old}, &out, &errOut); rc != 0 {
		t.Fatalf("good metadata scan failed: %d %s", rc, errOut.String())
	}
	if strings.Contains(out.String(), dir) {
		t.Fatal("absolute local path leaked into report")
	}
}

func TestHTMLAndTerminalEscapeUntrustedMembers(t *testing.T) {
	member := "<script>alert(1)</script>\x1b[31m"
	r := floor.Report{Schema: floor.ReportSchema, ToolVersion: floor.Version, Status: "blocked", Issues: []floor.Issue{{Code: "fixture", Severity: "error", Member: member, Detail: "<img src=https://bad.invalid>"}}, Inventories: []floor.Inventory{{Artifact: member, Binaries: []floor.Binary{{Member: member, Platform: "macos", Arch: "arm64", Floors: map[string]string{}}}}}}
	html, err := render(r, "html")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(html, []byte("<script>alert")) || bytes.Contains(html, []byte("<img src=https")) {
		t.Fatal("untrusted HTML became markup")
	}
	if !bytes.Contains(html, []byte("default-src 'none'")) {
		t.Fatal("report lacks network-blocking CSP")
	}
	text, _ := render(r, "text")
	if bytes.Contains(text, []byte("\x1b")) {
		t.Fatal("terminal control sequence not escaped")
	}
}

func TestInvocationAndPolicyErrors(t *testing.T) {
	for _, args := range [][]string{nil, {"unknown"}, {"scan"}, {"scan", "--format", "bad", "x"}, {"check", "x"}, {"scan", "--policy", "p", "x"}, {"compare", "a"}, {"scan", "--strip-components", "-1", "x"}} {
		var out, err bytes.Buffer
		if got := run(args, &out, &err); got != 2 {
			t.Fatalf("%v => %d", args, got)
		}
	}
	for _, versionArg := range []string{"version", "--version", "-v"} {
		var out, err bytes.Buffer
		if run([]string{versionArg}, &out, &err) != 0 || strings.TrimSpace(out.String()) != floor.Version {
			t.Fatalf("%s output incorrect", versionArg)
		}
	}
}

func TestFlagOrderingAndInterleaving(t *testing.T) {
	dir := t.TempDir()
	old := filepath.Join(dir, "old")
	current := filepath.Join(dir, "new")
	if err := os.WriteFile(old, machoFixture(12), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(current, machoFixture(14), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	// Test flags after positional arguments: scan ARTIFACT --format json
	if rc := run([]string{"scan", old, "--format", "json"}, &out, &errOut); rc != 0 {
		t.Fatalf("scan with flag at end failed (%d): %s", rc, errOut.String())
	}
	var r floor.Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	if r.Status != "pass" || len(r.Inventories) != 1 {
		t.Fatalf("unexpected scan report: %+v", r)
	}
	// Test compare with flags at end: compare OLD NEW --format json
	out.Reset()
	errOut.Reset()
	if rc := run([]string{"compare", old, current, "--format", "json"}, &out, &errOut); rc != 1 {
		t.Fatalf("compare with flag at end failed (%d): %s", rc, errOut.String())
	}
	// Test double-dash separator
	out.Reset()
	errOut.Reset()
	if rc := run([]string{"scan", "--format", "json", "--", old}, &out, &errOut); rc != 0 {
		t.Fatalf("scan with -- separator failed (%d): %s", rc, errOut.String())
	}
}

func TestCLIInitPolicyAndMCP(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, "app")
	if err := os.WriteFile(app, machoFixture(12), 0600); err != nil {
		t.Fatal(err)
	}

	// 1. Test init-policy to stdout
	var out, errOut bytes.Buffer
	rc := run([]string{"init-policy", app}, &out, &errOut)
	if rc != 0 {
		t.Fatalf("init-policy failed (%d): %s", rc, errOut.String())
	}
	var p floor.Policy
	if err := json.Unmarshal(out.Bytes(), &p); err != nil || len(p.Rules) != 1 {
		t.Fatalf("invalid generated policy: %v, %+v", err, p)
	}

	// 2. Test init-policy with --output
	policyOut := filepath.Join(dir, "policy.json")
	out.Reset()
	errOut.Reset()
	rc = run([]string{"init-policy", "--output", policyOut, app}, &out, &errOut)
	if rc != 0 {
		t.Fatalf("init-policy --output failed (%d): %s", rc, errOut.String())
	}
	fileBytes, err := os.ReadFile(policyOut)
	if err != nil {
		t.Fatal(err)
	}
	var pFromFile floor.Policy
	if err := json.Unmarshal(fileBytes, &pFromFile); err != nil || len(pFromFile.Rules) != 1 {
		t.Fatalf("invalid policy file: %v, %+v", err, pFromFile)
	}

	// 3. Test mcp subcommand via runWithStdin
	var mcpIn, mcpOut, mcpErr bytes.Buffer
	mcpIn.WriteString(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}` + "\n")
	rc = runWithStdin(&mcpIn, []string{"mcp"}, &mcpOut, &mcpErr)
	if rc != 0 {
		t.Fatalf("mcp command failed (%d): %s", rc, mcpErr.String())
	}
	if !bytes.Contains(mcpOut.Bytes(), []byte(`"protocolVersion":"2024-11-05"`)) {
		t.Fatalf("mcp command output invalid: %s", mcpOut.String())
	}
}

func TestSubcommandHelp(t *testing.T) {
	// General help
	for _, args := range [][]string{{"help"}, {"--help"}, {"-h"}} {
		var out, err bytes.Buffer
		rc := run(args, &out, &err)
		if rc != 0 || !strings.Contains(out.String(), "releasefloor <command>") {
			t.Fatalf("%v => %d, out: %s", args, rc, out.String())
		}
	}
	// Subcommand specific help via 'help <cmd>'
	for _, cmd := range []string{"scan", "compare", "check", "init-policy", "mcp", "version"} {
		var out, err bytes.Buffer
		rc := run([]string{"help", cmd}, &out, &err)
		if rc != 0 || !strings.Contains(out.String(), "Usage: releasefloor "+cmd) {
			t.Fatalf("help %s => %d, out: %s", cmd, rc, out.String())
		}
	}
	// Subcommand specific help via '<cmd> --help'
	for _, cmd := range []string{"scan", "compare", "check", "init-policy"} {
		var out, err bytes.Buffer
		rc := run([]string{cmd, "--help"}, &out, &err)
		if rc != 0 || !strings.Contains(out.String(), "Usage: releasefloor "+cmd) {
			t.Fatalf("%s --help => %d, out: %s", cmd, rc, out.String())
		}
	}
	// Unknown command for help returns 2
	var out, err bytes.Buffer
	rc := run([]string{"help", "nonexistent"}, &out, &err)
	if rc != 2 || !strings.Contains(err.String(), "unknown command") {
		t.Fatalf("help nonexistent => %d, err: %s", rc, err.String())
	}
}
