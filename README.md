# ReleaseFloor

[![CI](https://github.com/feiiiiii5/releasefloor/actions/workflows/ci.yml/badge.svg)](https://github.com/feiiiiii5/releasefloor/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/feiiiiii5/releasefloor?include_prereleases&display_name=tag)](https://github.com/feiiiiii5/releasefloor/releases/tag/v0.1.0)
[![Go Version](https://img.shields.io/badge/go-1.25+-00ADD8?logo=go)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey)](#)

[中文说明](README.zh-CN.md) · [Behavior Contract](docs/CONTRACT.md) · [AI Agent Guide](docs/AGENT.md) · [Product Decisions & Alternatives](docs/PRODUCT.md) · [Validation Record](docs/VALIDATION.md)

**Inspect recorded runtime requirements in ELF and Mach-O release artifacts without executing them.**  
Catch accidental GLIBC version creep, macOS deployment target bumps, unexpected dynamic libraries, and dropped architectures in binary release archives *before* you publish.

---

## 🎯 Target Audience (Who Needs ReleaseFloor?)

ReleaseFloor is built for anyone who publishes or automates the release of compiled native software:

| User Persona | Common Pain Point | How ReleaseFloor Solves It |
| :--- | :--- | :--- |
| **CLI & Systems Tool Authors**<br>*(Go, Rust, C/C++, Zig, Swift)* | Binaries compiled on newer CI machines fail on older user distros (e.g., CentOS 7, Debian 10, Ubuntu 20.04). | Catches GLIBC/GLIBCXX floor hikes and macOS target bumps before distribution. |
| **DevOps & Release Engineers**<br>*(CI/CD Pipeline Maintainers)* | Upgrades to GitHub Actions runners (`ubuntu-latest`) or Xcode silently break release matrices. | Provides a CI gate with explicit exit codes (0 = pass, 1 = blocked, 2 = command/input error). |
| **Package & Wheel Maintainers**<br>*(Python Wheels & Java JNI)* | C-extensions in binary wheels (`.whl`) or native libraries in JARs (`.jar`) silently exceed platform ceilings. | Direct inspection of `.whl` and `.jar` archives without disk extraction or execution. |
| **Autonomous AI Coding Agents**<br>*(Claude Code, Antigravity, Cursor)* | Toolchain changes can raise recorded library or macOS requirements. | MCP tools and JSON reports expose recorded requirements to an agent's build workflow. |

---

## 💡 Example Workflows

These are illustrative workflows. The [validation record](docs/VALIDATION.md) identifies artifacts and commands actually tested.

### 1. The Silent GLIBC Creep (CI Runner Upgrade)
* **The Situation**: A CI runner or toolchain upgrade changes the libraries used for linking.
* **The Disaster**: Your Go/Rust/C tool compiles cleanly with exit code 0, but links against `libc.so.6 (GLIBC_2.38)`. When a user runs it on Debian 11 or RHEL 8, it crashes immediately:  
  `./mytool: /lib/x86_64-linux-gnu/libc.so.6: version 'GLIBC_2.38' not found`.
* **The Fix**: `releasefloor compare old-release.tar.gz new-release.tar.gz` detects the floor jump from `2.28` to `2.38` and halts CI with exit code 1.

### 2. The Accidental macOS SDK Drift
* **The Situation**: A developer or macOS runner upgrades Xcode, unintentionally raising the minimum deployment target from macOS 11.0 (Big Sur) to 14.0 (Sonoma).
* **The Disaster**: Users on macOS Monterey or Ventura get an immediate OS rejection error upon launching your app.
* **The Fix**: `releasefloor check --policy policy.json dist/*.zip` flags `policy_os_exceeded` instantly.

### 3. Dropped Architecture in Universal Binaries
* **The Situation**: A misconfigured packaging script or CMake target accidentally leaves out the `arm64` slice from a macOS universal binary, shipping only `x86_64`.
* **The Disaster**: Apple Silicon (M1/M2/M3/M4) users suffer degraded performance via Rosetta emulation or encounter missing binary errors.
* **The Fix**: `releasefloor check` enforces required architectures per rule; `compare` detects `removed_binary` for the missing slice.

### 4. Accidental Dynamic Dependency Ingestion
* **The Situation**: A code change or third-party crate/module introduces an unintentional dynamic link to `libssl.so.3` or `libstdc++.so.6` in a binary intended to be static or libc-only.
* **The Disaster**: The binary fails to launch on any target machine lacking that exact shared library.
* **The Fix**: ReleaseFloor records all direct library imports and flags new dynamic dependencies for review.

### 5. AI Agent Self-Healing Build Loop
* **The Situation**: An AI coding agent generates code, compiles cross-platform binaries, and prepares a release tag.
* **The Disaster**: The agent needs to inspect recorded runtime requirements before target-system testing.
* **The Fix**: The agent calls `releasefloor_check` via MCP, reads the reported requirement, adjusts the relevant build configuration and checks the rebuilt artifact. A passing gate means the recorded metadata meets the policy; target-system execution is a separate check.

---

## ⚡ Quickstart

### Step 1: Install the documented source revision

The commands below use source revision `bf469b35a0fe4a1809b8073c9499bc464ec61419` (11 November 2026), which adds MCP, `init-policy`, wheel/JAR input support and the composite GitHub Action after v0.1.0.

With Go 1.25 or newer:
```sh
go install github.com/feiiiiii5/releasefloor/cmd/releasefloor@bf469b35a0fe4a1809b8073c9499bc464ec61419
releasefloor help
```

Prebuilt Linux, macOS and Windows commands are available for the [v0.1.0 pre-release](https://github.com/feiiiiii5/releasefloor/releases/tag/v0.1.0). That release provides `scan`, `compare`, `check` and `version`, without the later features above. The newer source still prints `0.1.0` for `version`; use the source revision to identify this build. Windows is a scanner host; PE input inspection is unsupported.

---

### Step 2: Inspect a Supported Release Artifact (`scan`)

Run `releasefloor scan` on ELF/Mach-O executables or libraries, including those packaged in supported archives (`.tar.gz`, `.zip`, `.whl`, `.jar`):

```sh
releasefloor scan dist/myapp-linux-amd64.tar.gz
```

**Illustrative terminal output (values abbreviated):**
```
releasefloor 0.1.0 · pass

artifact: myapp-linux-amd64.tar.gz
sha256:   <artifact SHA-256>
coverage: complete (4 regular entries, 1 runtime binary, 3 ignored files)

  bin/myapp (elf, amd64, executable)
    floors:       GLIBC 2.28, GLIBCXX 3.4.28
    libraries:    libc.so.6, libstdc++.so.6
```

Want JSON or an offline HTML report?
```sh
# Machine-readable JSON for scripts & agents
releasefloor scan dist/myapp-linux-amd64.tar.gz --format json

# Self-contained offline HTML audit report
releasefloor scan dist/myapp-linux-amd64.tar.gz --format html --output report.html
```

---

### Step 3: Compare Candidate Against Last Release (`compare`)

Prevent requirement regressions by comparing your new build candidate against your previous stable release:

```sh
releasefloor compare releases/v1.0.0.tar.gz dist/v1.1.0.tar.gz
```

* **Passes (Exit Code 0)**: No error-severity issue is reported. Added binaries are informational; added direct libraries produce warnings for review.
* **Blocks (Exit Code 1)**: Raised tracked version floors or minimum macOS requirements, removed binary identities or slices, and incomplete or unknown coverage produce errors. See the [behavior contract](docs/CONTRACT.md) for the complete comparison rules.

---

### Step 4: Auto-Generate Policy & Gate CI (`init-policy` + `check`)

Generate a starter policy from your baseline artifacts, then review its ceilings and required targets before using it in CI:

```sh
# 1. Synthesize baseline policy from your current stable build
mkdir -p .github
releasefloor init-policy --output .github/releasefloor-policy.json dist/*.tar.gz dist/*.zip

# 2. Gate all candidate builds in CI against this policy
releasefloor check --policy .github/releasefloor-policy.json dist/*.tar.gz dist/*.zip
```

**Example `.github/releasefloor-policy.json`:**
```json
{
  "schema": "releasefloor.policy.v1",
  "rules": [
    {"platform": "elf", "arch": "amd64", "max_floors": {"GLIBC": "2.28"}},
    {"platform": "elf", "arch": "arm64", "max_floors": {"GLIBC": "2.28"}},
    {"platform": "macos", "arch": "amd64", "max_os": "11.0"},
    {"platform": "macos", "arch": "arm64", "max_os": "11.0"}
  ]
}
```

---

### Step 5: Connect to Your AI Agent (MCP Server)

ReleaseFloor includes a built-in Model Context Protocol (MCP) server over `stdio` with **zero external dependencies**.

Add it to your Claude Desktop (`claude_desktop_config.json`) or Cursor (`mcp.json`):

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

AI Agents can now autonomously call:
- `releasefloor_scan`: Inspect runtime requirements in supported release artifacts.
- `releasefloor_compare`: Diff candidate builds against stable baselines.
- `releasefloor_check`: Validate full release matrices against policy ceilings.
- `releasefloor_init_policy`: Generate policy contracts automatically.

---

## 🚀 GitHub Actions CI/CD Integration

Install Go and add the gate after building your release artifacts:

```yaml
name: Release
on:
  push:
    tags: ['v*']

jobs:
  publish:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.25.x'
          cache: false

      # 1. Build your cross-platform artifacts into dist/
      # ... (e.g. goreleaser, cargo-dist, cmake, or wheels)

      # 2. Gate release baselines before uploading assets
      - name: Gate release runtime requirements
        uses: feiiiiii5/releasefloor@bf469b35a0fe4a1809b8073c9499bc464ec61419
        with:
          version: bf469b35a0fe4a1809b8073c9499bc464ec61419
          command: check
          args: --policy .github/releasefloor-policy.json dist/*.tar.gz dist/*.zip
```

---

## 🌐 Language & Ecosystem Support

ReleaseFloor reads ELF and Mach-O metadata independently of the source language:

* **C & C++**: Standalone binaries, shared libraries (`.so`, `.dylib`), and dynamic plugin architectures. Specifically gates `GLIBC`, `GLIBCXX`, and `CXXABI` version baselines.
* **Go & Rust**: Single-binary CLIs, static builds (`musl` / `CGO_ENABLED=0`), cross-compiled artifacts, and bundled system libraries.
* **Python (Native Wheels)**: Inspects native extensions (`.so`, `.dylib`) inside binary `.whl` packages (such as `manylinux` and `macosx` wheels) without extracting them.
* **Java & JVM (JNI & Native Image)**: Inspects bundled JNI shared libraries inside `.jar` archives, or ahead-of-time compiled native executables produced by GraalVM Native Image.
* **Zig & Swift**: Native CLI executables and dynamic frameworks on Linux and macOS.

Supported packaging formats: `.tar.gz`, `.tgz`, `.zip`, Python `.whl`, Java `.jar`, and raw standalone executables or libraries.

---

## 📊 Implemented Capabilities

| Capability | Scope in the documented source revision |
| --- | --- |
| Archive scanning | Raw ELF/Mach-O, `.tar.gz`, `.tgz`, `.zip`, `.whl` and `.jar`; no disk extraction |
| Architecture records | Separate records for every inspected universal Mach-O slice |
| Comparison | Tracks member, format, platform and architecture identities; reports requirement changes |
| Policy checks | JSON ceilings for GLIBC, GLIBCXX, CXXABI and minimum macOS; explicit required targets |
| Starter policy | `init-policy` derives ceilings from supplied artifacts for review |
| Agent integration | JSON reports and a stdio MCP server |
| Reports | Text, JSON and self-contained HTML |

For other tools and the workflow this project targets, see [product decisions](docs/PRODUCT.md).

---

## 🔒 Security & Scope Boundaries

* **No Target Execution**: ReleaseFloor **never** executes inspected binaries. Metadata is parsed statically with bounded memory readers.
* **Go Standard Library**: The scanner has no third-party Go dependencies and does not use CGo.
* **Never Overwrite**: `--output` strictly requires a new, non-existing file (`O_CREATE | O_EXCL`). Inputs and existing reports are never overwritten.
* **Archive Bounds**: Input size, member size, expanded bytes and file counts have explicit limits. Entries with unsafe paths are rejected; archive contents are not extracted to disk.

---

## 📖 CLI Help & Reference

```
Usage: releasefloor <command> [options] [arguments...]

Commands:
  scan         Inspect runtime requirements of an archive or binary
  compare      Compare runtime requirements between baseline and candidate builds
  check        Verify release artifacts against an explicit policy file
  init-policy  Synthesize a starter policy JSON from baseline artifacts
  mcp          Run as a Model Context Protocol (MCP) server over stdio
  version      Print version information
  help         Show help for ReleaseFloor or a specific command
```

To see detailed options and examples for any command:
```sh
releasefloor help scan
releasefloor help compare
releasefloor help check
releasefloor help init-policy
```

---

## License

ReleaseFloor is licensed under the [MIT License](LICENSE).
