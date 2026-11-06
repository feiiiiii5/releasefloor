package floor

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func versionParts(s string) ([]uint64, error) {
	items := strings.Split(s, ".")
	if len(items) > 8 || len(s) > 100 {
		return nil, fmt.Errorf("invalid numeric version %q", s)
	}
	out := make([]uint64, len(items))
	for i, part := range items {
		if part == "" {
			return nil, fmt.Errorf("invalid numeric version %q", s)
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return nil, fmt.Errorf("invalid numeric version %q", s)
			}
		}
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid numeric version %q", s)
		}
		out[i] = n
	}
	return out, nil
}

func compareVersion(a, b string) (int, error) {
	x, err := versionParts(a)
	if err != nil {
		return 0, err
	}
	y, err := versionParts(b)
	if err != nil {
		return 0, err
	}
	for i := 0; i < max(len(x), len(y)); i++ {
		var p, q uint64
		if i < len(x) {
			p = x[i]
		}
		if i < len(y) {
			q = y[i]
		}
		if p < q {
			return -1, nil
		}
		if p > q {
			return 1, nil
		}
	}
	return 0, nil
}

func identity(b Binary) string {
	return b.Member + "\x00" + b.Format + "\x00" + b.Platform + "\x00" + b.Arch
}
func display(b Binary) string { return b.Member + " [" + b.Platform + "/" + b.Arch + "]" }

func newReport(inventories ...Inventory) Report {
	r := Report{Schema: ReportSchema, ToolVersion: Version, Status: "pass", Inventories: inventories, Issues: []Issue{}}
	for _, inv := range inventories {
		if inv.Schema != InventorySchema {
			r.Issues = append(r.Issues, Issue{"invalid_inventory", "error", "", "Unrecognized inventory schema"})
		}
		if !inv.Complete || len(inv.Binaries) == 0 {
			r.Issues = append(r.Issues, Issue{"incomplete_inventory", "error", "", fmt.Sprintf("%s: coverage incomplete; see inventory issues", inv.Artifact)})
		}
		for _, b := range inv.Binaries {
			for _, reason := range b.Unknown {
				r.Issues = append(r.Issues, Issue{"unknown_requirement", "error", display(b), reason})
			}
		}
	}
	return r
}

func finish(r Report) Report {
	sort.SliceStable(r.Issues, func(i, j int) bool {
		a, b := r.Issues[i], r.Issues[j]
		return a.Severity+"\x00"+a.Code+"\x00"+a.Member+"\x00"+a.Detail < b.Severity+"\x00"+b.Code+"\x00"+b.Member+"\x00"+b.Detail
	})
	for _, issue := range r.Issues {
		if issue.Severity == "error" {
			r.Status = "blocked"
		}
	}
	return r
}

// ScanReport applies the coverage gate to a single inventory.
func ScanReport(inv Inventory) Report { return finish(newReport(inv)) }

// Compare aligns exact member/platform/architecture identities. It does not infer renames.
func Compare(old, current Inventory) Report {
	r := newReport(old, current)
	previous, next := map[string]Binary{}, map[string]Binary{}
	for _, pair := range []struct {
		inv   Inventory
		index map[string]Binary
	}{{old, previous}, {current, next}} {
		for _, b := range pair.inv.Binaries {
			key := identity(b)
			if _, exists := pair.index[key]; exists {
				r.Issues = append(r.Issues, Issue{"duplicate_identity", "error", display(b), "Ambiguous binary identity"})
			}
			pair.index[key] = b
		}
	}
	for key, before := range previous {
		after, exists := next[key]
		if !exists {
			r.Issues = append(r.Issues, Issue{"removed_binary", "error", display(before), "No matching member in the new artifact; use explicit path stripping when appropriate"})
			continue
		}
		for family, newFloor := range after.Floors {
			oldFloor, hadFloor := before.Floors[family]
			if !hadFloor {
				r.Issues = append(r.Issues, Issue{"floor_added", "error", display(after), family + " requirement introduced: " + newFloor})
				continue
			}
			n, err := compareVersion(newFloor, oldFloor)
			if err != nil {
				r.Issues = append(r.Issues, Issue{"unknown_requirement", "error", display(after), err.Error()})
			} else if n > 0 {
				r.Issues = append(r.Issues, Issue{"floor_raised", "error", display(after), family + ": " + oldFloor + " -> " + newFloor})
			}
		}
		if after.MinimumOS != "" {
			if before.MinimumOS == "" {
				r.Issues = append(r.Issues, Issue{"minimum_os_added", "error", display(after), "Minimum OS introduced: " + after.MinimumOS})
			} else {
				n, err := compareVersion(after.MinimumOS, before.MinimumOS)
				if err != nil {
					r.Issues = append(r.Issues, Issue{"unknown_requirement", "error", display(after), err.Error()})
				} else if n > 0 {
					r.Issues = append(r.Issues, Issue{"minimum_os_raised", "error", display(after), before.MinimumOS + " -> " + after.MinimumOS})
				}
			}
		}
		if before.MinimumOS != "" && after.MinimumOS == "" {
			r.Issues = append(r.Issues, Issue{"minimum_os_missing", "error", display(after), "Previously recorded minimum OS is unavailable"})
		}
		if after.Format == "macho" {
			oldRequirements := map[string]Requirement{}
			for _, requirement := range before.Requirements {
				key := fmt.Sprintf("%s\x00%t", requirement.Provider, requirement.Weak)
				prior, ok := oldRequirements[key]
				if !ok {
					oldRequirements[key] = requirement
					continue
				}
				if cmp, err := compareVersion(requirement.Version, prior.Version); err == nil && cmp > 0 {
					prior.Version = requirement.Version
				}
				oldRequirements[key] = prior
			}
			for _, requirement := range after.Requirements {
				key := fmt.Sprintf("%s\x00%t", requirement.Provider, requirement.Weak)
				prior, ok := oldRequirements[key]
				if !ok && !requirement.Weak {
					prior, ok = oldRequirements[fmt.Sprintf("%s\x00%t", requirement.Provider, true)]
				}
				if !ok {
					continue
				}
				cmp, err := compareVersion(requirement.Version, prior.Version)
				if err != nil {
					r.Issues = append(r.Issues, Issue{"unknown_requirement", "error", display(after), err.Error()})
				} else if cmp > 0 {
					r.Issues = append(r.Issues, Issue{"dylib_version_raised", "error", display(after), requirement.Provider + ": " + prior.Version + " -> " + requirement.Version})
				}
				if prior.Weak && !requirement.Weak {
					r.Issues = append(r.Issues, Issue{"dependency_strengthened", "error", display(after), requirement.Provider + ": weak import became required"})
				}
			}
		}
		known := map[string]bool{}
		for _, lib := range before.Libraries {
			known[lib] = true
		}
		for _, lib := range after.Libraries {
			if !known[lib] {
				r.Issues = append(r.Issues, Issue{"added_dependency", "warning", display(after), "Review new direct dependency: " + lib})
			}
		}
	}
	for key, b := range next {
		if _, exists := previous[key]; !exists {
			r.Issues = append(r.Issues, Issue{"added_binary", "info", display(b), "New member; no previous floor comparison is possible"})
		}
	}
	return finish(r)
}

// Check applies declared ceilings to every inspected binary in a release matrix.
func Check(inventories []Inventory, policy Policy) (Report, error) {
	if err := ValidatePolicy(policy); err != nil {
		return Report{}, err
	}
	if len(inventories) == 0 {
		return Report{}, fmt.Errorf("at least one inventory is required")
	}
	r := newReport(inventories...)
	matchedRules := make([]bool, len(policy.Rules))
	for _, inv := range inventories {
		for _, b := range inv.Binaries {
			matched := false
			for ruleIndex, rule := range policy.Rules {
				if rule.Platform != "*" && rule.Platform != b.Platform {
					continue
				}
				if rule.Arch != "*" && rule.Arch != b.Arch {
					continue
				}
				matched = true
				matchedRules[ruleIndex] = true
				for family, ceiling := range rule.MaxFloors {
					if actual, exists := b.Floors[family]; exists {
						n, err := compareVersion(actual, ceiling)
						if err != nil {
							r.Issues = append(r.Issues, Issue{"unknown_requirement", "error", display(b), err.Error()})
						} else if n > 0 {
							r.Issues = append(r.Issues, Issue{"policy_floor_exceeded", "error", display(b), family + ": " + actual + " exceeds " + ceiling})
						}
					}
				}
				if rule.MaxOS != "" {
					if b.MinimumOS == "" {
						r.Issues = append(r.Issues, Issue{"missing_minimum_os", "error", display(b), "Policy requires a recorded minimum OS"})
					} else {
						n, err := compareVersion(b.MinimumOS, rule.MaxOS)
						if err != nil {
							r.Issues = append(r.Issues, Issue{"unknown_requirement", "error", display(b), err.Error()})
						} else if n > 0 {
							r.Issues = append(r.Issues, Issue{"policy_os_exceeded", "error", display(b), b.MinimumOS + " exceeds " + rule.MaxOS})
						}
					}
				}
			}
			if !matched {
				r.Issues = append(r.Issues, Issue{"unmatched_policy", "error", display(b), "No policy rule covers this platform and architecture"})
			}
		}
	}
	for i, rule := range policy.Rules {
		if !matchedRules[i] && rule.Platform != "*" && rule.Arch != "*" {
			r.Issues = append(r.Issues, Issue{"expected_target_missing", "error", rule.Platform + "/" + rule.Arch, "No binary in the artifact matrix matches this explicit target"})
		}
	}
	return finish(r), nil
}
