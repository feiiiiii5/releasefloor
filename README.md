# ReleaseFloor

[![CI](https://github.com/feiiiiii5/releasefloor/actions/workflows/ci.yml/badge.svg)](https://github.com/feiiiiii5/releasefloor/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/feiiiiii5/releasefloor?display_name=tag)](https://github.com/feiiiiii5/releasefloor/releases/latest)
[![Go Version](https://img.shields.io/badge/go-1.25+-00ADD8?logo=go)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey)](#)

[中文说明](README.zh-CN.md) · [Behavior Contract](docs/CONTRACT.md) · [AI Agent Guide](docs/AGENT.md) · [Product Decisions & Alternatives](docs/PRODUCT.md) · [Validation Record](docs/VALIDATION.md)

**Zero-overhead preflight release gate for compiled binaries (ELF & Mach-O).**  
Catch accidental GLIBC version creep, macOS deployment target bumps, unexpected dynamic libraries, and dropped architectures in binary release archives *before* you publish.

---

## 🎯 Target Audience (Who Needs ReleaseFloor?)

ReleaseFloor is built for anyone who publishes or automates the release of compiled native software:

| User Persona | Common Pain Point | How ReleaseFloor Solves It |
| :--- | :--- | :--- |
| **CLI & Systems Tool Authors**<br>*(Go, Rust, C/C++, Zig, Swift)* | Binaries compiled on newer CI machines fail on older user distros (e.g., CentOS 7, Debian 10, Ubuntu 20.04). | Catches GLIBC/GLIBCXX floor hikes and macOS target bumps before distribution. |
| **DevOps & Release Engineers**<br>*(CI/CD Pipeline Maintainers)* | Upgrades to GitHub Actions runners (`ubuntu-latest`) or Xcode silently break release matrices. | Provides a 3-line fail-fast CI gate with strict exit codes (0 = pass, 1 = blocked). |
| **Package & Wheel Maintainers**<br>*(Python Wheels & Java JNI)* | C-extensions in binary wheels (`.whl`) or native libraries in JARs (`.jar`) silently exceed platform ceilings. | Direct inspection of `.whl` and `.jar` archives without disk extraction or execution. |
| **Autonomous AI Coding Agents**<br>*(Claude Code, Antigravity, Cursor)* | Agents bump build toolchains or cross-compile flags without knowing they broke compatibility on older kernels. | Native Model Context Protocol (MCP) server & deterministic JSON for closed-loop remediation. |

---

## 💡 Real-World Scenarios (When to Use ReleaseFloor)

### 1. The Silent GLIBC Creep (CI Runner Upgrade)
* **The Situation**: GitHub Actions updates `ubuntu-latest` from Ubuntu 22.04 to 24.04.
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
* **The Disaster**: The agent has no way to verify compatibility without running on 10 different virtual machines.
* **The Fix**: The agent calls `releasefloor_check` via MCP, receives structured JSON diagnosis, automatically adds `-C target-feature=+crt-static` or adjusts `-mmacosx-version-min`, and verifies success deterministically.

---

## ⚡ 3-Minute Quickstart Tutorial

### Step 1: Install ReleaseFloor (30 Seconds)

**Option A — Using Go (1.25+):**
```sh
go install github.com/feiiiiii5/releasefloor/cmd/releasefloor@v0.1.0
```

**Option B — Download Standalone Binary (Linux / macOS / Windows):**
Download the prebuilt archive for your OS from [GitHub Releases](https://github.com/feiiiiii5/releasefloor/releases/latest), unpack, and place `releasefloor` in your `PATH`.

```sh
# Verify installation
releasefloor version
```

---

### Step 2: Inspect Any Release Artifact (`scan`)

Run `releasefloor scan` on any compiled executable, archive (`.tar.gz`, `.zip`), Python Wheel (`.whl`), or Java JAR (`.jar`):

```sh
releasefloor scan dist/myapp-linux-amd64.tar.gz
```

**Example Terminal Output:**
```
releasefloor 0.1.0 · pass

artifact: myapp-linux-amd64.tar.gz
sha256:   e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
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

* **Passes (Exit Code 0)**: Runtime requirements remain identical or lowered; new binaries added.
* **Blocks (Exit Code 1)**: Any GLIBC floor raised, macOS version bumped, slice dropped, or library added.

---

### Step 4: Auto-Generate Policy & Gate CI (`init-policy` + `check`)

Instead of writing release policies by hand, let ReleaseFloor inspect your current stable release and synthesize your contract:

```sh
# 1. Synthesize baseline policy from your current stable build
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
- `releasefloor_scan`: Inspect runtime requirements of any build archive.
- `releasefloor_compare`: Diff candidate builds against stable baselines.
- `releasefloor_check`: Validate full release matrices against policy ceilings.
- `releasefloor_init_policy`: Generate policy contracts automatically.

---

## 🚀 GitHub Actions CI/CD Integration

Integrate ReleaseFloor into your release workflow in 3 lines of YAML:

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

      # 1. Build your cross-platform artifacts into dist/
      # ... (e.g. goreleaser, cargo-dist, cmake, or wheels)

      # 2. Gate release baselines before uploading assets
      - name: Gate release runtime requirements
        uses: feiiiiii5/releasefloor@v0.1.0
        with:
          command: check
          args: --policy .github/releasefloor-policy.json dist/*.tar.gz dist/*.zip
```

---

## 🌐 Language & Ecosystem Support

ReleaseFloor is **completely language-agnostic**. It inspects compiled machine-code metadata (ELF and Mach-O) directly:

* **C & C++**: Standalone binaries, shared libraries (`.so`, `.dylib`), and dynamic plugin architectures. Specifically gates `GLIBC`, `GLIBCXX`, and `CXXABI` version baselines.
* **Go & Rust**: Single-binary CLIs, static builds (`musl` / `CGO_ENABLED=0`), cross-compiled artifacts, and bundled system libraries.
* **Python (Native Wheels)**: Inspects native extensions (`.so`, `.dylib`) inside binary `.whl` packages (such as `manylinux` and `macosx` wheels) without extracting them.
* **Java & JVM (JNI & Native Image)**: Inspects bundled JNI shared libraries inside `.jar` archives, or ahead-of-time compiled native executables produced by GraalVM Native Image.
* **Zig & Swift**: Native CLI executables and dynamic frameworks on Linux and macOS.

Supported packaging formats: `.tar.gz`, `.tgz`, `.zip`, Python `.whl`, Java `.jar`, and raw standalone executables or libraries.

---

## 📊 Feature & Competitor Matrix

| Capability | ReleaseFloor | abicheck | BLINT | cargo-dist | Custom bash scripts |
| :--- | :---: | :---: | :---: | :---: | :---: |
| **Mixed Archive Scanning** (`.tar.gz`, `.zip`, `.whl`, `.jar`) | **Native** (no disk extract) | Shared libs only | Single binary | During build only | Manual `tar`/`unzip` |
| **Multi-Architecture Universal Mach-O** | **Yes** (all slices) | Partial | No | Packaging only | Requires `otool`/`lipo` |
| **Zero Execution / Sandboxing Required** | **Yes** (pure static parse) | Yes | Yes | Yes | Depends on script |
| **Release Matrix Policy Gate** | **Yes** (Declarative JSON) | No | Diff only | Static config | Hard to maintain |
| **Auto Policy Synthesis (`init-policy`)** | **Yes** (one command) | No | No | No | No |
| **Native MCP Server for AI Agents** | **Yes** (Built-in stdio) | No | No | No | No |
| **Language & Ecosystem Agnostic** | **Any compiled binary** | C/C++ | Any binary | Rust-centric | Fragile across OS |
| **External Dependencies** | **None** (Go stdlib only) | Python / libs | Python / libs | Rust toolchain | `readelf`, `otool`, etc. |
| **Offline Self-Contained HTML Report** | **Yes** (Strict CSP) | No | HTML | No | No |

---

## 🔒 Security & Scope Boundaries

* **No Target Execution**: ReleaseFloor **never** executes inspected binaries. Metadata is parsed statically with bounded memory readers.
* **Pure Go Standard Library**: Zero external dependencies, zero CGo. Fast compilation, minimal attack surface.
* **Never Overwrite**: `--output` strictly requires a new, non-existing file (`O_CREATE | O_EXCL`). Inputs and existing reports are never overwritten.
* **Archive Safety**: Built-in limits protect against zip-slip directory traversal (`../`), decompression bombs, and malformed header attacks.

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
