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

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	usage := "ReleaseFloor " + floor.Version + "\nUsage: releasefloor scan [options] ARTIFACT\n       releasefloor compare [options] OLD NEW\n       releasefloor check --policy POLICY [options] ARTIFACT...\n       releasefloor version\nChecks recorded requirements, not complete runtime compatibility.\n"
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	if args[0] == "version" || args[0] == "--version" {
		if len(args) != 1 {
			fmt.Fprintln(stderr, "version takes no arguments")
			return 2
		}
		fmt.Fprintln(stdout, floor.Version)
		return 0
	}
	if args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(stdout, usage)
		return 0
	}
	command := args[0]
	if command != "scan" && command != "compare" && command != "check" {
		fmt.Fprint(stderr, usage)
		return 2
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	format := flags.String("format", "text", "text, json or html")
	output := flags.String("output", "", "write a new report file (never overwrite)")
	policyPath := flags.String("policy", "", "JSON ceilings policy (check only)")
	strip := flags.Int("strip-components", 0, "explicit archive path prefix components to discard")
	flags.Usage = func() { fmt.Fprint(stderr, usage); flags.PrintDefaults() }
	if err := flags.Parse(args[1:]); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if *format != "text" && *format != "json" && *format != "html" {
		fmt.Fprintln(stderr, "format must be text, json or html")
		return 2
	}
	paths := flags.Args()
	if (command == "scan" && len(paths) != 1) || (command == "compare" && len(paths) != 2) || (command == "check" && len(paths) == 0) {
		flags.Usage()
		return 2
	}
	if *strip < 0 || (command != "check" && *policyPath != "") || (command == "check" && *policyPath == "") {
		fmt.Fprintln(stderr, "invalid policy or strip-components option")
		return 2
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
	limits.StripComponents = *strip
	inventories := make([]floor.Inventory, 0, len(paths))
	for _, path := range paths {
		inv, err := floor.Scan(path, limits)
		if err != nil {
			fmt.Fprintf(stderr, "artifact %d: %v\n", len(inventories)+1, err)
			return 2
		}
		inventories = append(inventories, inv)
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
	if *output != "" {
		f, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			fmt.Fprintln(stderr, "cannot create report: destination must be a new writable file")
			return 2
		}
		_, writeErr := f.Write(data)
		syncErr := f.Sync()
		closeErr := f.Close()
		if writeErr != nil || syncErr != nil || closeErr != nil {
			os.Remove(*output)
			fmt.Fprintln(stderr, "cannot finish writing report")
			return 2
		}
	} else {
		if _, err := stdout.Write(data); err != nil {
			fmt.Fprintln(stderr, "cannot write report")
			return 2
		}
	}
	if report.Status != "pass" {
		return 1
	}
	return 0
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
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><meta http-equiv="Content-Security-Policy" content="default-src 'none'; style-src 'unsafe-inline'; base-uri 'none'; form-action 'none'"><title>ReleaseFloor preflight</title>
<style>body{font:16px/1.55 system-ui,sans-serif;color:#e8eef8;background:#101724;margin:0}main{max-width:1100px;margin:auto;padding:40px 24px}h1{font-size:36px;line-height:1.15}h2{margin-top:32px}p{max-width:850px;color:#b8c6da}.status{display:inline-block;border:1px solid #8ca1bf;border-radius:6px;padding:5px 14px;text-transform:uppercase;font-weight:700}.blocked{background:#492223;color:#ffcec6}.pass{background:#133c32;color:#b9f3de}table{width:100%;border-collapse:collapse;background:#192335}th,td{padding:12px;text-align:left;border-bottom:1px solid #34435b;vertical-align:top;overflow-wrap:anywhere}th{color:#b8c6da}code{font:13px ui-monospace,monospace;overflow-wrap:anywhere}li{margin:8px 0}details{margin:16px 0}small{color:#b8c6da}.scroll{overflow:auto}@media(max-width:650px){main{padding:24px 16px}h1{font-size:28px}table{font-size:13px}td,th{padding:8px}}</style></head><body><main>
<small>RELEASEFLOOR {{.ToolVersion}} · OFFLINE PREFLIGHT</small><h1>Recorded runtime requirements</h1><span class="status {{.Status}}">{{.Status}}</span><p>This result covers inspected metadata and declared ceilings. It does not certify compatibility, provenance, security, CPU support or behavior. Keep real target-system smoke tests.</p>
<h2>Gate findings</h2>{{if .Issues}}<ul>{{range .Issues}}<li><strong>{{.Severity}} · {{.Code}}</strong> <code>{{.Member}}</code><br>{{.Detail}}</li>{{end}}</ul>{{else}}<p>No blocking difference found in the inspected fields.</p>{{end}}
{{range .Inventories}}<h2>{{.Artifact}}</h2><p><code>SHA-256 {{.SHA256}}</code><br>Coverage complete: <strong>{{.Complete}}</strong> · {{.Files}} entries · {{len .Binaries}} runtime binaries · {{.Ignored}} ignored regular files</p>
{{if .Issues}}<ul>{{range .Issues}}<li><strong>{{.Code}}</strong> <code>{{.Member}}</code> — {{.Detail}}</li>{{end}}</ul>{{end}}
<div class="scroll"><table><thead><tr><th>Archive member</th><th>Platform / architecture</th><th>Version floors</th><th>Minimum OS / SDK</th></tr></thead><tbody>{{range .Binaries}}<tr><td><code>{{.Member}}</code><br><small>{{.Kind}}</small></td><td>{{.Platform}} / {{.Arch}}</td><td><code>{{json .Floors}}</code></td><td>{{.MinimumOS}}<br><small>SDK {{.SDK}}</small></td></tr>{{end}}</tbody></table></div>
{{range .Binaries}}<details><summary>Evidence · {{.Member}} · {{.Arch}}</summary><p><code>SHA-256 {{.SHA256}}</code></p>{{if .Unknown}}<p>Unresolved: {{json .Unknown}}</p>{{end}}<pre><code>{{json .Requirements}}</code></pre><p>Direct libraries: <code>{{json .Libraries}}</code></p></details>{{end}}{{end}}
<p>Generated locally. No scripts, remote fonts, telemetry or automatic network requests.</p></main></body></html>`
