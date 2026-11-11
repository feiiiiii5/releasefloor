package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"os"
	"strings"

	"github.com/feiiiiii5/releasefloor/floor"
)

func main() { os.Exit(runWithStdin(os.Stdin, os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	return runWithStdin(os.Stdin, args, stdout, stderr)
}

func generalUsage() string {
	return "ReleaseFloor " + floor.Version + ` - Preflight gate for native release artifacts

Usage:
  releasefloor <command> [options] [arguments...]

Commands:
  scan         Inspect runtime requirements of an archive or binary
  compare      Compare runtime requirements between baseline and candidate builds
  check        Verify release artifacts against an explicit policy file
  init-policy  Synthesize a starter policy JSON from baseline artifacts
  mcp          Run as a Model Context Protocol (MCP) server over stdio
  version      Print version information
  help         Show help for ReleaseFloor or a specific command

Quick Examples:
  releasefloor scan dist/myapp-v1.0.tar.gz
  releasefloor compare dist/old.zip dist/new.zip
  releasefloor init-policy dist/*.tar.gz > policy.json
  releasefloor check --policy policy.json dist/*.tar.gz
  releasefloor help <command>

Note: ReleaseFloor inspects recorded requirements without executing target binaries.
`
}

func commandHelp(command string) string {
	switch command {
	case "scan":
		return `Usage: releasefloor scan [options] <artifact>

Inspects an archive (.tar.gz, .zip, .whl, .jar) or standalone executable
and prints its recorded runtime requirements (glibc, macOS SDK, libraries).

Arguments:
  <artifact>                  Archive or binary file to inspect

Options:
  --format text|json|html    Output format (default: text)
  --output <file>            Write report to a new file (never overwrites)
  --strip-components <n>     Leading archive path components to strip (default: 0)

Examples:
  releasefloor scan dist/myapp-v1.0.tar.gz
  releasefloor scan dist/myapp-cp310-manylinux_2_17_x86_64.whl --format json
  releasefloor scan myapp --format html --output report.html
`
	case "compare":
		return `Usage: releasefloor compare [options] <old-artifact> <new-artifact>

Compares recorded runtime requirements between two builds and flags
unintended requirement regressions (glibc creep, new dynamic libraries,
missing architectures).

Arguments:
  <old-artifact>             Baseline release archive or binary
  <new-artifact>             Candidate release archive or binary

Options:
  --format text|json|html    Output format (default: text)
  --output <file>            Write report to a new file (never overwrites)
  --strip-components <n>     Leading archive path components to strip (default: 0)

Examples:
  releasefloor compare release-v1.0.tar.gz release-v1.1.tar.gz
  releasefloor compare baseline.zip dist/candidate.zip --format json
  releasefloor compare v1.tar.gz v2.tar.gz --format html --output diff.html
`
	case "check":
		return `Usage: releasefloor check --policy <policy.json> [options] <artifact>...

Evaluates candidate artifacts against a declared policy contract.
Returns exit code 0 if all constraints pass, or code 1 if a ceiling is breached.

Required Flag:
  --policy <file>            Path to policy JSON file

Options:
  --format text|json|html    Output format (default: text)
  --output <file>            Write report to a new file (never overwrites)
  --strip-components <n>     Leading archive path components to strip (default: 0)

Examples:
  releasefloor check --policy policy.json dist/*.tar.gz
  releasefloor check --policy release-policy.json dist/myapp.zip --format json
`
	case "init-policy":
		return `Usage: releasefloor init-policy [options] <artifact>...

Synthesizes a starter policy JSON by inspecting baseline artifacts.
Captures current glibc versions, macOS SDK ceilings, and required architectures.

Options:
  --output <file>            Write generated policy to file (never overwrites)
  --strip-components <n>     Leading archive path components to strip (default: 0)

Examples:
  releasefloor init-policy dist/*.tar.gz > policy.json
  releasefloor init-policy --output policy.json dist/myapp-v1.0.zip
`
	case "mcp":
		return `Usage: releasefloor mcp

Starts a zero-dependency Model Context Protocol (MCP) server over stdio (JSON-RPC 2.0).
Exposes releasefloor_scan, releasefloor_compare, releasefloor_check,
and releasefloor_init_policy tools for AI agents.

Examples:
  releasefloor mcp
`
	case "version":
		return `Usage: releasefloor version

Prints the current ReleaseFloor version.
`
	default:
		return generalUsage()
	}
}

func runWithStdin(stdin io.Reader, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, generalUsage())
		return 2
	}
	if args[0] == "version" || args[0] == "--version" || args[0] == "-v" {
		if len(args) != 1 {
			fmt.Fprintln(stderr, "version takes no arguments")
			return 2
		}
		fmt.Fprintln(stdout, floor.Version)
		return 0
	}
	if args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		if len(args) == 1 {
			fmt.Fprint(stdout, generalUsage())
			return 0
		}
		if len(args) == 2 {
			sub := args[1]
			switch sub {
			case "scan", "compare", "check", "init-policy", "mcp", "version":
				fmt.Fprint(stdout, commandHelp(sub))
				return 0
			default:
				fmt.Fprintf(stderr, "unknown command %q for help\n\n", sub)
				fmt.Fprint(stderr, generalUsage())
				return 2
			}
		}
		fmt.Fprintln(stderr, "help takes at most 1 command argument")
		return 2
	}
	if args[0] == "mcp" {
		if len(args) != 1 {
			fmt.Fprintln(stderr, "mcp takes no arguments; it communicates via stdio")
			return 2
		}
		if err := floor.RunMCPServer(stdin, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "mcp server error: %v\n", err)
			return 2
		}
		return 0
	}
	command := args[0]
	if command != "scan" && command != "compare" && command != "check" && command != "init-policy" {
		fmt.Fprint(stderr, generalUsage())
		return 2
	}

	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() { fmt.Fprint(stderr, commandHelp(command)) }

	var format *string
	var output *string
	var policyPath *string
	var strip *int

	switch command {
	case "scan", "compare":
		format = flags.String("format", "text", "text, json or html")
		output = flags.String("output", "", "write a new report file (never overwrite)")
		strip = flags.Int("strip-components", 0, "explicit archive path prefix components to discard")
	case "check":
		policyPath = flags.String("policy", "", "JSON ceilings policy (check only)")
		format = flags.String("format", "text", "text, json or html")
		output = flags.String("output", "", "write a new report file (never overwrite)")
		strip = flags.Int("strip-components", 0, "explicit archive path prefix components to discard")
	case "init-policy":
		output = flags.String("output", "", "write a new policy file (never overwrite)")
		strip = flags.Int("strip-components", 0, "explicit archive path prefix components to discard")
	}

	if err := flags.Parse(partitionArgs(args[1:])); err != nil {
		if err == flag.ErrHelp {
			fmt.Fprint(stdout, commandHelp(command))
			return 0
		}
		return 2
	}
	if format != nil && *format != "text" && *format != "json" && *format != "html" {
		fmt.Fprintln(stderr, "format must be text, json or html")
		return 2
	}
	paths := flags.Args()
	if command == "scan" && len(paths) != 1 {
		fmt.Fprintf(stderr, "error: scan requires exactly 1 artifact argument, got %d\n", len(paths))
		flags.Usage()
		return 2
	}
	if command == "compare" && len(paths) != 2 {
		fmt.Fprintf(stderr, "error: compare requires exactly 2 artifact arguments (OLD and NEW), got %d\n", len(paths))
		flags.Usage()
		return 2
	}
	if command == "check" && len(paths) == 0 {
		fmt.Fprintln(stderr, "error: check requires at least 1 artifact argument")
		flags.Usage()
		return 2
	}
	if command == "init-policy" && len(paths) == 0 {
		fmt.Fprintln(stderr, "error: init-policy requires at least 1 artifact argument")
		flags.Usage()
		return 2
	}
	if command == "check" && (policyPath == nil || *policyPath == "") {
		fmt.Fprintln(stderr, "error: check requires --policy <file>")
		return 2
	}

	stripVal := 0
	if strip != nil {
		stripVal = *strip
	}
	if stripVal < 0 {
		fmt.Fprintln(stderr, "error: --strip-components must be nonnegative")
		return 2
	}
	outputVal := ""
	if output != nil {
		outputVal = *output
	}

	var policy floor.Policy
	if command == "check" {
		f, err := os.Open(*policyPath)
		if err != nil {
			fmt.Fprintln(stderr, "cannot read policy")
			return 2
		}
		data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
		closeErr := f.Close()
		if err != nil || closeErr != nil {
			fmt.Fprintln(stderr, "cannot read policy")
			return 2
		}
		policy, err = floor.ParsePolicy(data)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
	}
	limits := floor.DefaultLimits()
	limits.StripComponents = stripVal
	inventories := make([]floor.Inventory, 0, len(paths))
	for _, path := range paths {
		inv, err := floor.Scan(path, limits)
		if err != nil {
			fmt.Fprintf(stderr, "artifact %d: %v\n", len(inventories)+1, err)
			return 2
		}
		inventories = append(inventories, inv)
	}

	if command == "init-policy" {
		derived, err := floor.DerivePolicy(inventories)
		if err != nil {
			fmt.Fprintf(stderr, "cannot derive policy: %v\n", err)
			return 2
		}
		data, err := json.MarshalIndent(derived, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "cannot render policy: %v\n", err)
			return 2
		}
		data = append(data, '\n')
		return writeOutput(data, outputVal, stdout, stderr)
	}

	var report floor.Report
	switch command {
	case "scan":
		report = floor.ScanReport(inventories[0])
	case "compare":
		report = floor.Compare(inventories[0], inventories[1])
	case "check":
		var err error
		report, err = floor.Check(inventories, policy)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
	}
	data, err := render(report, *format)
	if err != nil {
		fmt.Fprintln(stderr, "cannot render report:", err)
		return 2
	}
	if rc := writeOutput(data, *output, stdout, stderr); rc != 0 {
		return rc
	}
	if report.Status != "pass" {
		return 1
	}
	return 0
}

func writeOutput(data []byte, output string, stdout, stderr io.Writer) int {
	if output != "" {
		f, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			fmt.Fprintln(stderr, "cannot create report: destination must be a new writable file")
			return 2
		}
		_, writeErr := f.Write(data)
		syncErr := f.Sync()
		closeErr := f.Close()
		if writeErr != nil || syncErr != nil || closeErr != nil {
			os.Remove(output)
			fmt.Fprintln(stderr, "cannot finish writing report")
			return 2
		}
	} else {
		if _, err := stdout.Write(data); err != nil {
			fmt.Fprintln(stderr, "cannot write report")
			return 2
		}
	}
	return 0
}

// partitionArgs enables flags to appear before, between, or after positional arguments.
func partitionArgs(args []string) []string {
	var flagArgs []string
	var posArgs []string
	takesValue := map[string]bool{
		"format":           true,
		"output":           true,
		"policy":           true,
		"strip-components": true,
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			posArgs = append(posArgs, args[i+1:]...)
			break
		}
		if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			name := strings.TrimLeft(arg, "-")
			if strings.Contains(name, "=") {
				flagArgs = append(flagArgs, arg)
				continue
			}
			if takesValue[name] && i+1 < len(args) {
				flagArgs = append(flagArgs, arg, args[i+1])
				i++
				continue
			}
			flagArgs = append(flagArgs, arg)
			continue
		}
		posArgs = append(posArgs, arg)
	}
	return append(flagArgs, posArgs...)
}

func render(r floor.Report, format string) ([]byte, error) {
	if format == "json" {
		data, err := json.MarshalIndent(r, "", "  ")
		return append(data, '\n'), err
	}
	var out bytes.Buffer
	if format == "html" {
		t, err := template.New("report").Funcs(template.FuncMap{"json": func(v any) string { b, _ := json.Marshal(v); return string(b) }}).Parse(htmlReport)
		if err != nil {
			return nil, err
		}
		err = t.Execute(&out, r)
		return out.Bytes(), err
	}
	fmt.Fprintf(&out, "ReleaseFloor %s — %s\nRecorded requirements only; keep target-system smoke tests.\n", r.ToolVersion, strings.ToUpper(r.Status))
	for _, inv := range r.Inventories {
		fmt.Fprintf(&out, "\nArtifact: %q\nSHA-256: %s\nCoverage: complete=%t, files=%d, binaries=%d, ignored=%d\n", inv.Artifact, inv.SHA256, inv.Complete, inv.Files, len(inv.Binaries), inv.Ignored)
		for _, b := range inv.Binaries {
			floors, _ := json.Marshal(b.Floors)
			fmt.Fprintf(&out, "  %q [%s/%s] floors=%s minOS=%q\n", b.Member, b.Platform, b.Arch, floors, b.MinimumOS)
		}
		for _, issue := range inv.Issues {
			fmt.Fprintf(&out, "  %s %s %q: %q\n", issue.Severity, issue.Code, issue.Member, issue.Detail)
		}
	}
	for _, issue := range r.Issues {
		fmt.Fprintf(&out, "\n%s %s %q: %q\n", issue.Severity, issue.Code, issue.Member, issue.Detail)
	}
	return out.Bytes(), nil
}

const htmlReport = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'"><title>ReleaseFloor Preflight Report</title>
<style>
body{font:16px/1.55 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,Helvetica,Arial,sans-serif;color:#e8eef8;background:#0d131f;margin:0}
main{max-width:1100px;margin:auto;padding:40px 24px}
h1{font-size:32px;line-height:1.2;margin:12px 0}
h2{font-size:22px;margin-top:36px;border-bottom:1px solid #233148;padding-bottom:8px}
p{max-width:850px;color:#a9bcd3}
.header{margin-bottom:28px}
.status{display:inline-block;border-radius:6px;padding:6px 16px;text-transform:uppercase;font-weight:700;font-size:14px;letter-spacing:0.5px}
.status.blocked{background:#4e1c1e;color:#ffc0b8;border:1px solid #852a2d}
.status.pass{background:#113c2f;color:#b2f1dc;border:1px solid #1a654f}
.badge{display:inline-block;padding:2px 7px;border-radius:4px;font-size:12px;font-weight:600;text-transform:uppercase}
.sev-error{background:#541c1f;color:#ff9b9b}
.sev-warning{background:#4a3b12;color:#ffe082}
.sev-info{background:#163450;color:#90caf9}
table{width:100%;border-collapse:collapse;background:#151e2e;border-radius:6px;overflow:hidden;margin:16px 0}
th,td{padding:12px 14px;text-align:left;border-bottom:1px solid #253349;vertical-align:top;overflow-wrap:anywhere}
th{background:#1a2538;color:#a9bcd3;font-size:13px;text-transform:uppercase;letter-spacing:0.5px}
code{font:13px ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace;background:#1d293d;padding:2px 5px;border-radius:4px;overflow-wrap:anywhere}
pre code{display:block;padding:12px;background:#121a28;border-radius:6px;overflow-x:auto}
ul{list-style:none;padding-left:0}
li{margin:10px 0;padding:10px 14px;background:#151e2e;border-radius:6px;border-left:4px solid #3d4f6c}
li.item-error{border-left-color:#e53935}
li.item-warning{border-left-color:#fbc02d}
li.item-info{border-left-color:#1e88e5}
details{margin:14px 0;background:#131c2b;border:1px solid #24334b;border-radius:6px;padding:10px 14px}
details summary{cursor:pointer;font-weight:600;color:#cbdcf2}
small{color:#889bb4}
.scroll{overflow:auto}
footer{margin-top:48px;padding-top:16px;border-top:1px solid #222f44;font-size:13px;color:#788ca6}
@media(max-width:650px){main{padding:20px 14px}h1{font-size:26px}table{font-size:13px}td,th{padding:8px 10px}}
</style></head><body><main>
<div class="header">
<small>RELEASEFLOOR {{.ToolVersion}} · OFFLINE PREFLIGHT GATE</small>
<h1>Recorded Runtime Requirements</h1>
<span class="status {{.Status}}">{{.Status}}</span>
<p>This result inspects object-format metadata and declared policy ceilings. It does not certify target-system execution, transitive dependencies, CPU features, or security. Maintain real target-system smoke tests.</p>
</div>

<h2>Gate Findings</h2>
{{if .Issues}}
<ul>
{{range .Issues}}
<li class="item-{{.Severity}}"><span class="badge sev-{{.Severity}}">{{.Severity}}</span> <strong>{{.Code}}</strong>{{if .Member}} <code>{{.Member}}</code>{{end}}<br><span style="color:#d4e1f4">{{.Detail}}</span></li>
{{end}}
</ul>
{{else}}
<p style="color:#a7f3d0">✔ No blocking differences or policy ceiling violations found.</p>
{{end}}

{{range .Inventories}}
<h2>Artifact: {{.Artifact}}</h2>
<p><code>SHA-256 {{.SHA256}}</code><br>Coverage complete: <strong>{{.Complete}}</strong> · {{.Files}} entries · {{len .Binaries}} runtime binaries · {{.Ignored}} ignored regular files</p>

{{if .Issues}}
<ul>
{{range .Issues}}
<li class="item-{{.Severity}}"><span class="badge sev-{{.Severity}}">{{.Severity}}</span> <strong>{{.Code}}</strong>{{if .Member}} <code>{{.Member}}</code>{{end}} — {{.Detail}}</li>
{{end}}
</ul>
{{end}}

<div class="scroll">
<table>
<thead><tr><th>Archive member</th><th>Platform / Architecture</th><th>Version Floors</th><th>Minimum OS / SDK</th></tr></thead>
<tbody>
{{range .Binaries}}
<tr>
<td><code>{{.Member}}</code><br><small>{{.Kind}}</small></td>
<td>{{.Platform}} / {{.Arch}}</td>
<td><code>{{json .Floors}}</code></td>
<td>{{if .MinimumOS}}{{.MinimumOS}}{{else}}<small>n/a</small>{{end}}{{if .SDK}}<br><small>SDK {{.SDK}}</small>{{end}}</td>
</tr>
{{end}}
</tbody>
</table>
</div>

{{range .Binaries}}
<details>
<summary>Evidence · {{.Member}} · {{.Arch}}</summary>
<p><code>SHA-256 {{.SHA256}}</code></p>
{{if .Unknown}}<p style="color:#ffb4ab">Unresolved: {{json .Unknown}}</p>{{end}}
<pre><code>{{json .Requirements}}</code></pre>
<p>Direct libraries: <code>{{json .Libraries}}</code></p>
</details>
{{end}}
{{end}}

<footer>Generated locally by ReleaseFloor {{.ToolVersion}}. No scripts, remote fonts, telemetry, or network requests.</footer>
</main></body></html>`
