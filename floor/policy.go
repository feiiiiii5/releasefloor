package floor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// ParsePolicy rejects ambiguous or misspelled policy inputs rather than silently dropping fields.
func ParsePolicy(data []byte) (Policy, error) {
	if len(data) > 1<<20 {
		return Policy{}, fmt.Errorf("policy exceeds 1 MiB")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	if err := uniqueJSON(d, 0); err != nil {
		return Policy{}, err
	}
	if _, err := d.Token(); err != io.EOF {
		return Policy{}, fmt.Errorf("policy must contain one JSON document")
	}
	// encoding/json matches struct fields without case sensitivity. Policies
	// require exact spellings so an alias cannot silently replace a ceiling.
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil || root == nil {
		return Policy{}, fmt.Errorf("policy must be an object")
	}
	for key := range root {
		if key != "schema" && key != "rules" {
			return Policy{}, fmt.Errorf("unknown or noncanonical policy property %q", key)
		}
	}
	var rules []map[string]json.RawMessage
	if err := json.Unmarshal(root["rules"], &rules); err != nil || len(rules) > 256 {
		return Policy{}, fmt.Errorf("invalid policy rules")
	}
	for _, rule := range rules {
		for key := range rule {
			if key != "platform" && key != "arch" && key != "max_floors" && key != "max_os" {
				return Policy{}, fmt.Errorf("unknown or noncanonical rule property %q", key)
			}
		}
	}
	d = json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var p Policy
	if err := d.Decode(&p); err != nil {
		return Policy{}, fmt.Errorf("invalid policy: %w", err)
	}
	return p, ValidatePolicy(p)
}

func uniqueJSON(d *json.Decoder, depth int) error {
	if depth > 32 {
		return fmt.Errorf("policy nesting exceeds 32")
	}
	t, err := d.Token()
	if err != nil {
		return fmt.Errorf("invalid policy JSON: %w", err)
	}
	if delim, ok := t.(json.Delim); ok {
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				if err != nil {
					return err
				}
				name, ok := key.(string)
				if !ok {
					return fmt.Errorf("invalid object key")
				}
				if seen[name] {
					return fmt.Errorf("duplicate policy property %q", name)
				}
				seen[name] = true
				if err := uniqueJSON(d, depth+1); err != nil {
					return err
				}
			}
		case '[':
			for d.More() {
				if err := uniqueJSON(d, depth+1); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("invalid policy delimiter")
		}
		_, err = d.Token()
		return err
	}
	return nil
}

func ValidatePolicy(p Policy) error {
	if p.Schema != PolicySchema {
		return fmt.Errorf("policy schema must be %q", PolicySchema)
	}
	if len(p.Rules) == 0 || len(p.Rules) > 256 {
		return fmt.Errorf("policy requires 1..256 rules")
	}
	seen := map[string]bool{}
	for _, r := range p.Rules {
		if r.Platform != "elf" && r.Platform != "macos" && r.Platform != "*" {
			return fmt.Errorf("unsupported policy platform %q", r.Platform)
		}
		if r.Platform == "" || r.Arch == "" || len(r.Platform) > 100 || len(r.Arch) > 100 {
			return fmt.Errorf("each rule requires a platform and architecture")
		}
		for _, v := range []string{r.Platform, r.Arch} {
			if strings.ContainsAny(v, "\x00\r\n\t ") {
				return fmt.Errorf("invalid policy identity")
			}
		}
		key := r.Platform + "/" + r.Arch
		if seen[key] {
			return fmt.Errorf("duplicate policy rule %q", key)
		}
		seen[key] = true
		if len(r.MaxFloors) == 0 && r.MaxOS == "" {
			return fmt.Errorf("rule %s has no ceiling", key)
		}
		for family, v := range r.MaxFloors {
			if family != "GLIBC" && family != "GLIBCXX" && family != "CXXABI" {
				return fmt.Errorf("unsupported floor family %q (expected GLIBC, GLIBCXX or CXXABI)", family)
			}
			if family == "" || len(family) > 100 || strings.ContainsAny(family, "\x00\r\n\t ") {
				return fmt.Errorf("invalid floor family")
			}
			if _, err := versionParts(v); err != nil {
				return err
			}
		}
		if r.Platform == "macos" && len(r.MaxFloors) != 0 {
			return fmt.Errorf("macos rules use max_os, not ELF version floors")
		}
		if r.Platform == "elf" && r.MaxOS != "" {
			return fmt.Errorf("ELF rules use max_floors, not a minimum OS inferred from ELF")
		}
		if r.MaxOS != "" {
			if _, err := versionParts(r.MaxOS); err != nil {
				return err
			}
		}
	}
	return nil
}
