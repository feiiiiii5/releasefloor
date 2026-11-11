package floor

import (
	"strings"
	"testing"
)

func inv(bs ...Binary) Inventory {
	return Inventory{Schema: InventorySchema, Artifact: "synthetic.zip", Binaries: bs, Complete: true, Issues: []Issue{}}
}
func elfBinary(member, arch, glibc string) Binary {
	floors := map[string]string{}
	if glibc != "" {
		floors["GLIBC"] = glibc
	}
	return Binary{Member: member, Format: "elf", Platform: "elf", Arch: arch, Floors: floors}
}
func hasCode(r Report, code string) bool {
	for _, issue := range r.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func TestCompareActualRequirementChanges(t *testing.T) {
	old := inv(elfBinary("bin/tool", "amd64", "2.9"), elfBinary("lib/plugin.so", "amd64", "2.17"), elfBinary("bin/tool", "arm64", "2.17"))
	current := inv(elfBinary("bin/tool", "amd64", "2.10"), elfBinary("lib/plugin.so", "amd64", "2.38"))
	r := Compare(old, current)
	if r.Status != "blocked" || !hasCode(r, "floor_raised") || !hasCode(r, "removed_binary") {
		t.Fatalf("missed raised floor/architecture loss: %+v", r)
	}
	count := 0
	for _, issue := range r.Issues {
		if issue.Code == "floor_raised" {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("wanted two independent raises, got %d", count)
	}
	if Compare(old, old).Status != "pass" {
		t.Fatal("unchanged complete inventory blocked")
	}
}

func TestNoGuessingPathMappingOrStaticPortability(t *testing.T) {
	a := inv(elfBinary("v1/bin/tool", "amd64", ""))
	b := inv(elfBinary("v2/bin/tool", "amd64", ""))
	if !hasCode(Compare(a, b), "removed_binary") {
		t.Fatal("implicitly stripped a changed member path")
	}
	c := inv(elfBinary("v1/bin/tool", "amd64", "2.17"))
	if !hasCode(Compare(a, c), "floor_added") {
		t.Fatal("missed newly introduced runtime requirement")
	}
}

func TestUnknownAndMissingInventoryNeverPass(t *testing.T) {
	b := elfBinary("binary", "amd64", "2.17")
	b.Unknown = []string{"unsupported version requirement"}
	if Compare(inv(b), inv(b)).Status != "blocked" {
		t.Fatal("unknown metadata passed")
	}
	partial := inv(elfBinary("binary", "amd64", "2.17"))
	partial.Complete = false
	if ScanReport(partial).Status != "blocked" {
		t.Fatal("partial scan passed")
	}
	if ScanReport(inv()).Status != "blocked" {
		t.Fatal("empty scan passed")
	}
}

func TestPolicyMatrixTargetsCeilingsAndStatic(t *testing.T) {
	p := Policy{Schema: PolicySchema, Rules: []Rule{{Platform: "elf", Arch: "amd64", MaxFloors: map[string]string{"GLIBC": "2.35"}}, {Platform: "elf", Arch: "arm64", MaxFloors: map[string]string{"GLIBC": "2.35"}}}}
	r, err := Check([]Inventory{inv(elfBinary("binary", "amd64", ""))}, p)
	if err != nil || r.Status != "blocked" || !hasCode(r, "expected_target_missing") {
		t.Fatalf("absent explicit matrix target passed: %+v %v", r, err)
	}
	r, err = Check([]Inventory{inv(elfBinary("binary", "amd64", "")), inv(elfBinary("binary", "arm64", "2.35"))}, p)
	if err != nil || r.Status != "pass" {
		t.Fatalf("valid ceilings including static binary failed: %+v %v", r, err)
	}
	r, _ = Check([]Inventory{inv(elfBinary("binary", "amd64", "2.38")), inv(elfBinary("binary", "arm64", "2.35"))}, p)
	if !hasCode(r, "policy_floor_exceeded") {
		t.Fatal("ceiling violation not found")
	}
	p.Rules = p.Rules[:1]
	r, _ = Check([]Inventory{inv(elfBinary("binary", "arm64", "2.35"))}, p)
	if !hasCode(r, "unmatched_policy") {
		t.Fatal("unexpected architecture passed")
	}
}

func TestDerivePolicy(t *testing.T) {
	elf1 := elfBinary("bin/tool", "amd64", "2.28")
	elf1.Floors["GLIBCXX"] = "3.4.28"
	elf2 := elfBinary("lib/plugin.so", "amd64", "2.31")
	machoArm := Binary{Member: "bin/mac", Format: "macho", Platform: "macos", Arch: "arm64", MinimumOS: "12.0.0", Floors: map[string]string{}}
	machoX86 := Binary{Member: "bin/mac", Format: "macho", Platform: "macos", Arch: "amd64", MinimumOS: "11.0.0", Floors: map[string]string{}}
	staticElf := elfBinary("bin/static", "arm64", "")

	inventories := []Inventory{
		inv(elf1, elf2),
		inv(machoArm, machoX86, staticElf),
	}

	p, err := DerivePolicy(inventories)
	if err != nil {
		t.Fatalf("DerivePolicy failed: %v", err)
	}
	if p.Schema != PolicySchema || len(p.Rules) != 4 {
		t.Fatalf("unexpected derived policy: %+v", p)
	}

	// Verify amd64 ELF got the highest GLIBC floor (2.31) and GLIBCXX (3.4.28)
	foundElfAmd64 := false
	foundStaticArm64 := false
	for _, rule := range p.Rules {
		if rule.Platform == "elf" && rule.Arch == "amd64" {
			foundElfAmd64 = true
			if rule.MaxFloors["GLIBC"] != "2.31" || rule.MaxFloors["GLIBCXX"] != "3.4.28" {
				t.Fatalf("incorrect ELF amd64 ceilings: %+v", rule.MaxFloors)
			}
		}
		if rule.Platform == "elf" && rule.Arch == "arm64" {
			foundStaticArm64 = true
			if rule.MaxFloors["GLIBC"] != "2.17" {
				t.Fatalf("static binary did not get conservative fallback ceiling: %+v", rule.MaxFloors)
			}
		}
		if rule.Platform == "macos" && rule.Arch == "arm64" {
			if rule.MaxOS != "12.0.0" {
				t.Fatalf("incorrect macOS arm64 ceiling: %s", rule.MaxOS)
			}
		}
	}
	if !foundElfAmd64 || !foundStaticArm64 {
		t.Fatal("missing expected rules in derived policy")
	}

	// The derived policy must pass against the baseline inventories themselves!
	report, err := Check(inventories, p)
	if err != nil || report.Status != "pass" {
		t.Fatalf("baseline failed its own derived policy: %v, %+v", err, report)
	}
}

func TestMacOSCeilingAndSDKNotRuntime(t *testing.T) {
	b := Binary{Member: "bin/tool", Format: "macho", Platform: "macos", Arch: "arm64", MinimumOS: "12.0.0", SDK: "26.0.0", Floors: map[string]string{}}
	p := Policy{Schema: PolicySchema, Rules: []Rule{{Platform: "macos", Arch: "arm64", MaxOS: "12"}}}
	r, err := Check([]Inventory{inv(b)}, p)
	if err != nil || r.Status != "pass" {
		t.Fatal("SDK was confused with minimum runtime OS")
	}
	newB := b
	newB.MinimumOS = "14.0.0"
	if !hasCode(Compare(inv(b), inv(newB)), "minimum_os_raised") {
		t.Fatal("minimum OS increase missed")
	}
	newB = b
	newB.MinimumOS = ""
	r, _ = Check([]Inventory{inv(newB)}, p)
	if !hasCode(r, "missing_minimum_os") {
		t.Fatal("missing required OS evidence passed")
	}
}

func TestPolicyRejectsTyposAndAmbiguity(t *testing.T) {
	valid := `{"schema":"releasefloor.policy.v1","rules":[{"platform":"elf","arch":"amd64","max_floors":{"GLIBC":"2.35"}}]}`
	if _, err := ParsePolicy([]byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{
		strings.Replace(valid, "max_floors", "max_floor", 1),
		strings.Replace(valid, "GLIBC", "GILBC", 1),
		strings.Replace(valid, "2.35", "2.x", 1),
		strings.Replace(valid, `"arch":"amd64"`, `"arch":"amd64","arch":"arm64"`, 1),
		strings.Replace(valid, `"max_floors":{"GLIBC":"2.35"}`, `"max_floors":{"GLIBC":"2.17"},"MAX_FLOORS":{"GLIBC":"2.99"}`, 1),
		strings.Replace(valid, `"schema"`, `"SCHEMA"`, 1),
		valid + ` {}`,
		`{"schema":"releasefloor.policy.v1","rules":[]}`,
		`{"schema":"releasefloor.policy.v1","rules":[{"platform":"macos","arch":"arm64","max_floors":{"GLIBC":"2.35"}}]}`,
	} {
		if _, err := ParsePolicy([]byte(bad)); err == nil {
			t.Fatalf("accepted ambiguous policy: %s", bad)
		}
	}
}

func TestMachODependencyVersionAndStrengthChanges(t *testing.T) {
	a := Binary{Member: "bin/tool", Format: "macho", Platform: "macos", Arch: "arm64", MinimumOS: "12.0", Floors: map[string]string{}, Libraries: []string{"@rpath/libfoo.dylib"}, Requirements: []Requirement{{Provider: "@rpath/libfoo.dylib", Version: "1.0.0", Weak: true}}}
	b := a
	b.Requirements = []Requirement{{Provider: "@rpath/libfoo.dylib", Version: "2.0.0", Weak: false}}
	r := Compare(inv(a), inv(b))
	if r.Status != "blocked" || !hasCode(r, "dylib_version_raised") || !hasCode(r, "dependency_strengthened") {
		t.Fatalf("recorded dylib changes passed: %+v", r)
	}
	a.Requirements = []Requirement{{Provider: "@rpath/libfoo.dylib", Version: "1.0.0"}, {Provider: "@rpath/libfoo.dylib", Version: "2.0.0", Weak: true}}
	b.Requirements = []Requirement{{Provider: "@rpath/libfoo.dylib", Version: "2.0.0"}, {Provider: "@rpath/libfoo.dylib", Version: "2.0.0", Weak: true}}
	if !hasCode(Compare(inv(a), inv(b)), "dylib_version_raised") {
		t.Fatal("higher weak version masked an increased required version")
	}
}

func TestNumericVersions(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{{"2.10", "2.9", 1}, {"12", "12.0.0", 0}, {"2.17", "2.38", -1}} {
		got, err := compareVersion(tc.a, tc.b)
		if err != nil || got != tc.want {
			t.Fatalf("%+v => %d %v", tc, got, err)
		}
	}
}
