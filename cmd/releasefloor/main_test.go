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
	var out, err bytes.Buffer
	if run([]string{"--version"}, &out, &err) != 0 || strings.TrimSpace(out.String()) != floor.Version {
		t.Fatal("version output incorrect")
	}
}
