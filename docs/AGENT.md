# ReleaseFloor for AI Agents

ReleaseFloor is built with an **Agent-First** interface. AI coding assistants (such as Claude Code, Codex, Antigravity, Cursor, Copilot Workspace, and autonomous CI release agents) can execute, parse, and act upon ReleaseFloor's deterministic outputs without fragile screen-scraping or regex parsing.

---

## Why AI Agents Use ReleaseFloor

When an AI Agent is tasked with setting up CI/CD, bumping toolchains, cross-compiling, or preparing a release, silent runtime incompatibility (such as an upgraded GLIBC floor or a bumped macOS deployment target) is one of the hardest bugs for an agent to diagnose because:
1. The binary builds successfully with exit code 0.
2. The binary runs fine on the host agent's machine.
3. The failure only occurs on older user machines.

ReleaseFloor gives agents a command-line gate for inspecting recorded runtime requirements before publishing.

---

## Source version

MCP, `init-policy`, and wheel/JAR inputs are available in source revision `bf469b35a0fe4a1809b8073c9499bc464ec61419`, after the v0.1.0 pre-release. Install the revision documented in the [README](../README.md#step-1-install-the-documented-source-revision). The source still reports version `0.1.0`; the commit identifies the newer build.

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
| `0` | **Continue the release checks** | No blocking issue in this command's metadata checks. Review warnings and run the project's other release checks. |
| `1` | **Inspect the report** | A policy/comparison error or incomplete coverage blocks this gate. Read `report.issues` and the inventories. |
| `2` | **Fix Invocation** | Syntax error, missing policy, or corrupted archive. Fix arguments or inspect the archive integrity. |

---

## JSON Report Schema (`releasefloor.report.v1`)

A shortened illustrative blocked report follows. `status` is `pass` or `blocked`; `inventories` contains the scanned artifacts. Added direct dependencies are warning-severity comparison issues and do not block on their own.

```json
{
  "schema": "releasefloor.report.v1",
  "tool_version": "0.1.0",
  "status": "blocked",
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
            "GLIBC": "2.38",
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
- **Diagnosis**: The recorded version requirement exceeds the configured ceiling or the baseline. Inspect the build and dependencies to determine where it came from.
- **Agent Fixes**:
  - Use a build image or sysroot matching the supported runtime baseline; pin it in CI.
  - For Go: If the program and its dependencies support a build without CGo, evaluate `CGO_ENABLED=0`; otherwise use a suitable C toolchain and sysroot.
  - For Rust: Review the target, linker and sysroot. A musl target changes the runtime target and should be chosen according to the program's dependencies and support policy.
  - For C/C++: Rebuild against the intended headers and libraries, then rescan the resulting artifact.

### 2. `minimum_os_raised` / `policy_os_exceeded` (e.g. `14.0.0 exceeds 12.0.0`)
- **Diagnosis**: The recorded minimum macOS version increased. Check deployment-target settings and linked dependencies separately from the SDK version.
- **Agent Fixes**:
  - Configure the compiler/linker deployment target for the project's supported macOS release.
  - Rebuild and inspect the resulting load commands. A flag alone does not establish that dependencies or toolchain runtime support that OS.

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

ReleaseFloor includes a **built-in MCP server** using JSON-RPC 2.0 over standard I/O (`stdio`). It runs in the CLI process and requires no Node.js or Python runtime.

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
      "args": ["run", "github.com/feiiiiii5/releasefloor/cmd/releasefloor@bf469b35a0fe4a1809b8073c9499bc464ec61419", "mcp"]
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

For a custom function-calling adapter, the following provider-neutral schema describes a wrapper you implement around the CLI. Map `command`, `policy_path`, `artifacts` and `strip_components` to the corresponding arguments; registering the schema alone does not execute the command.

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
