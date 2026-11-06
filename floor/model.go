// Package floor inspects recorded runtime requirements without executing inputs.
package floor

import "errors"

const Version = "0.1.0"
const InventorySchema = "releasefloor.inventory.v1"
const ReportSchema = "releasefloor.report.v1"
const PolicySchema = "releasefloor.policy.v1"

var ErrNotBinary = errors.New("not an ELF or Mach-O runtime binary")
var ErrUnsupported = errors.New("unsupported binary")

type Requirement struct {
	Provider string   `json:"provider"`
	Version  string   `json:"version"`
	Weak     bool     `json:"weak,omitempty"`
	Symbols  []string `json:"symbols,omitempty"`
}

type Binary struct {
	Member       string            `json:"member"`
	SHA256       string            `json:"sha256"`
	Format       string            `json:"format"`
	Platform     string            `json:"platform"`
	Arch         string            `json:"arch"`
	Kind         string            `json:"kind"`
	Floors       map[string]string `json:"floors"`
	Requirements []Requirement     `json:"requirements,omitempty"`
	Libraries    []string          `json:"libraries,omitempty"`
	MinimumOS    string            `json:"minimum_os,omitempty"`
	SDK          string            `json:"sdk,omitempty"`
	Unknown      []string          `json:"unknown,omitempty"`
}

type Issue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Member   string `json:"member,omitempty"`
	Detail   string `json:"detail"`
}

type Inventory struct {
	Schema   string   `json:"schema"`
	Artifact string   `json:"artifact"`
	SHA256   string   `json:"sha256"`
	Binaries []Binary `json:"binaries"`
	Issues   []Issue  `json:"issues"`
	Files    int      `json:"files"`
	Ignored  int      `json:"ignored"`
	Complete bool     `json:"complete"`
}

type Limits struct {
	MaxInputBytes   int64
	MaxMemberBytes  int64
	MaxTotalBytes   int64
	MaxFiles        int
	StripComponents int
}

func DefaultLimits() Limits {
	return Limits{MaxInputBytes: 128 << 20, MaxMemberBytes: 64 << 20, MaxTotalBytes: 256 << 20, MaxFiles: 4096}
}

type Rule struct {
	Platform  string            `json:"platform"`
	Arch      string            `json:"arch"`
	MaxFloors map[string]string `json:"max_floors,omitempty"`
	MaxOS     string            `json:"max_os,omitempty"`
}

type Policy struct {
	Schema string `json:"schema"`
	Rules  []Rule `json:"rules"`
}

type Report struct {
	Schema      string      `json:"schema"`
	ToolVersion string      `json:"tool_version"`
	Status      string      `json:"status"`
	Inventories []Inventory `json:"inventories"`
	Issues      []Issue     `json:"issues"`
}
