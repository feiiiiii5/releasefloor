# ReleaseFloor for AI Agents

ReleaseFloor is built with an **Agent-First** interface. AI coding assistants (such as Claude Code, Codex, Antigravity, Cursor, Copilot Workspace, and autonomous CI release agents) can execute, parse, and act upon ReleaseFloor's deterministic outputs without fragile screen-scraping or regex parsing.

---

## Why AI Agents Use ReleaseFloor

When an AI Agent is tasked with setting up CI/CD, bumping toolchains, cross-compiling, or preparing a release, silent runtime incompatibility (such as an upgraded GLIBC floor or a bumped macOS deployment target) is one of the hardest bugs for an agent to diagnose because:
1. The binary builds successfully with exit code 0.
2. The binary runs fine on the host agent's machine.
3. The failure only occurs on older user machines.

ReleaseFloor gives agents a **single, deterministic, zero-overhead gate** to inspect final release artifacts before publishing.

---

## Agent Invocation Pattern

AI Agents should always run ReleaseFloor with `--format json`:

```sh
# 1. Pre-scan a newly built archive
releasefloor scan --format json <ARTIFACT>

# 2. Compare a candidate build against the prior release
releasefloor compare --format json <PREVIOUS_RELEASE> <CANDIDATE_RELEASE>

# 3. Verify an entire cross-platform release matrix against ceilings
releasefloor check --policy <POLICY_JSON> --format json <ARTIFACTS...>
```

### Exit Codes for Agents

| Exit Code | Agent Action | Meaning |
| :---: | :--- | :--- |
| `0` | **Proceed** | All matrix rules passed. Zero blocking changes detected. Safe to publish. |
| `1` | **Remediate / Block** | Policy violation or requirement creep detected. Parse `report.issues` and apply the remediation playbook below. |
| `2` | **Fix Invocation** | Syntax error, missing policy, or corrupted archive. Fix arguments or inspect the archive integrity. |

---

## JSON Report Schema (`releasefloor.report.v1`)

When running with `--format json`, the root object schema is:

```json
{
  "schema": "releasefloor.report.v1",
  "tool_version": "0.1.0",
  "status": "pass" | "blocked",
  "inventories": [
    {
      "schema": "releasefloor.inventory.v1",
      "artifact": "app-linux-amd64.tar.gz",
      "sha256": "...",
      "complete": true,
      "files": 4,
      "ignored": 1,
      "binaries": [
        {
          "member": "bin/app",
          "sha256": "...",
          "format": "elf",
          "platform": "elf",
          "arch": "amd64",
          "kind": "executable",
          "floors": {
            "GLIBC": "2.28",
            "GLIBCXX": "3.4.28"
          },
          "libraries": ["libc.so.6", "libstdc++.so.6"]
        }
      ]
    }
  ],
  "issues": [
    {
      "code": "policy_floor_exceeded",
      "severity": "error",
      "member": "bin/app [elf/amd64]",
      "detail": "GLIBC: 2.38 exceeds 2.28"
    }
  ]
}
```

---

## Agent Remediation Playbook

When `status == "blocked"`, iterate over `report.issues` and take the recommended fix:

### 1. `policy_floor_exceeded` / `floor_raised` (e.g. `GLIBC: 2.38 exceeds 2.28`)
- **Diagnosis**: The binary was linked against a newer host `libc.so.6` than allowed.
- **Agent Fixes**:
  - In GitHub Actions: Change `runs-on: ubuntu-latest` to an older runner like `ubuntu-20.04`.
  - For Go: Ensure `CGO_ENABLED=0` if no C dependencies are required, or use `zig cc` as `CC`.
  - For Rust: Target `x86_64-unknown-linux-musl`, use `cross`, or use `zigbuild` (`cargo-zigbuild`).
  - For C/C++: Build inside a `manylinux` or Debian oldstable Docker container.

### 2. `minimum_os_raised` / `policy_os_exceeded` (e.g. `14.0.0 exceeds 12.0.0`)
- **Diagnosis**: The compiler target SDK raised the minimum supported macOS version.
- **Agent Fixes**:
  - Export `MACOSX_DEPLOYMENT_TARGET=12.0` in the build environment or Makefile.
  - For Rust: Set target deployment flag via `RUSTFLAGS="-C link-arg=-mmacosx-version-min=12.0"`.
  - For Go: Set `CGO_CFLAGS="-mmacosx-version-min=12.0" CGO_LDFLAGS="-mmacosx-version-min=12.0"`.

### 3. `expected_target_missing` (e.g. `macos/arm64`)
- **Diagnosis**: A policy ceiling rule explicitly required an architecture that is completely missing from the artifact list.
- **Agent Fixes**:
  - Check the packaging workflow: Did the `lipo` step fail? Did an arm64 build job get excluded in the CI matrix?
  - Verify artifact paths: Did the workflow upload the wrong folder?

### 4. `removed_binary`
- **Diagnosis**: A binary member present in the prior release is missing in the candidate release.
- **Agent Fixes**:
  - Check if member paths shifted: If the archive added an outer directory (e.g. `app-v2.0/bin/tool`), pass `--strip-components 1`.
  - Check build output: Did a subcomponent fail to compile or get omitted from the packaging script?

---

## Native MCP Server (`releasefloor mcp`)

ReleaseFloor includes a **built-in, zero-dependency MCP server** that speaks JSON-RPC 2.0 over standard I/O (`stdio`). No background daemon, no Node.js/Python dependencies, and sub-millisecond invocation.

### Connecting to Claude Desktop / Cursor / Antigravity

Add ReleaseFloor to your MCP client configuration (e.g. `claude_desktop_config.json` or `.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "releasefloor": {
      "command": "releasefloor",
      "args": ["mcp"]
    }
  }
}
```

Or if running directly via `go`:
```json
{
  "mcpServers": {
    "releasefloor": {
      "command": "go",
      "args": ["run", "github.com/feiiiiii5/releasefloor/cmd/releasefloor@latest", "mcp"]
    }
  }
}
```

### Registered MCP Tools

When connected, the following 4 tools are automatically available to the agent:

1. **`releasefloor_scan`**:
   - `artifact`: Path to `.tar.gz`, `.zip`, `.whl`, `.jar`, or raw binary.
   - `strip_components` *(optional)*: Leading path parts to strip.
2. **`releasefloor_compare`**:
   - `old_artifact`: Path to baseline release archive.
   - `new_artifact`: Path to candidate release archive.
   - `strip_components` *(optional)*: Leading path parts to strip.
3. **`releasefloor_check`**:
   - `policy_path`: Path to `policy.json`.
   - `artifacts`: Array of file paths to check.
   - `strip_components` *(optional)*: Leading path parts to strip.
4. **`releasefloor_init_policy`**:
   - `artifacts`: Array of baseline artifact paths to inspect and synthesize a policy from.
   - `strip_components` *(optional)*: Leading path parts to strip.

---

## Function Calling Tool Definition (OpenAI / Anthropic Direct)

For agents using manual function calling instead of stdio MCP, register this tool schema:

```json
{
  "name": "releasefloor",
  "description": "Inspect compiled release archives (tar.gz, zip, raw binaries, wheels, jars) for GLIBC version drift, minimum macOS bumps, and missing architectures without executing them.",
  "parameters": {
    "type": "object",
    "properties": {
      "command": {
        "type": "string",
        "enum": ["scan", "compare", "check", "init-policy"],
        "description": "Subcommand to execute"
      },
      "policy_path": {
        "type": "string",
        "description": "Path to releasefloor JSON policy file (required for check)"
      },
      "artifacts": {
        "type": "array",
        "items": { "type": "string" },
        "description": "File paths to the archives or binaries to inspect"
      },
      "strip_components": {
        "type": "integer",
        "description": "Number of leading archive directory path components to strip"
      }
    },
    "required": ["command", "artifacts"]
  }
}
```
