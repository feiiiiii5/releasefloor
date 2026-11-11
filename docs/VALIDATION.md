# Validation record

This is the record for the original ReleaseFloor 0.1.0 implementation. It distinguishes metadata correctness, command behavior and target-system compatibility. Scanned third-party programs were not executed, and their binaries are not distributed with this project.

## Local verification

Host: macOS arm64; Go 1.27.3. The source declares Go 1.25 and is also checked on that toolchain by the public CI matrix.

- `go test -race -count=1 ./...` and `go vet ./...` exercise original fixtures, policy/identity comparisons, parser and archive bounds, escaping, immutable inputs and refusal to overwrite output.
- The original example generator creates format metadata, not runnable programs. Its old ZIP has four runtime records: an ELF executable, a shared library and two Mach-O universal slices. The new ZIP raises GLIBC, GLIBCXX and minimum macOS and removes arm64. Expected comparison findings are two `floor_raised`, one `minimum_os_raised` and one `removed_binary`.
- `scripts/verify_cli.py` runs the built command as a native process. Its 11 observations include gate exits 0/1/2, the complete archive, matrix ceilings and missing targets, HTML file output, refusal to overwrite a report or source, unsupported PE coverage, malformed archives and unchanged source hashes.
- Bounded fuzz sessions exercised `Inspect` for 10 seconds (535,857 executions) and policy parsing for approximately 6 seconds (258,266 executions), two workers. Both passed. These finite runs do not prove parser security.

Independent review found and prompted regression fixes for policy casing aliases overriding ceilings, same-provider Mach-O compatibility-version increases, weak imports becoming required, and avoiding a higher weak version masking an increased required version.

## Public artifact inspection

These exact files were acquired from the upstream projects' release pages, and their SHA-256 values matched GitHub release-asset digests. This is not an assertion about every version or target system.

| Input | Observed result | Independent check |
| --- | --- | --- |
| [jq 1.8.1 Linux amd64](https://github.com/jqlang/jq/releases/tag/jq-1.8.1), SHA-256 `020468de7539ce70ef1bceaf7cde2e8c4f2ca6c3afb84642aabc5c97d9fc2a0d` | Complete ELF scan, no recorded version-family imports | This does not prove universal portability or successful execution. |
| jq 1.8.1 macOS arm64, SHA-256 `a9fe3ea2f86dfc72f6728417521ec9067b343277152b114f4e98d8cb0e263603` | Minimum macOS `14.0.0`, SDK `14.5.0`, direct libSystem dependency | Apple's `otool -l` independently reports minos 14.0, SDK 14.5. |
| [fd 10.3.0 GNU/Linux amd64 tar.gz](https://github.com/sharkdp/fd/releases/tag/v10.3.0), SHA-256 `c3c2bc79f838e780173fc8f18b337ec273e7ba17c7ff8f551be29fc3c19b7916` | Archive coverage complete; GLIBC floor `2.18`; one runtime binary, nine ignored regular files | All 20 version-provider nodes matched an independent read with Go's `debug/elf.DynamicVersionNeeds`. The extracted member hash was `16791ab3d5c2f7ce04dc994d175dab75aa704038d769ebe497ae5d59cb2c5ec9`. |

No model calls, production credentials, user libraries or private files were involved.

## Limits

The implementation gates numeric GLIBC/GLIBCXX/CXXABI floors, recorded macOS targets and the documented comparison fields. It does not check every ABI family. Unsupported/unknown coverage blocks; an absent family in a complete scan means no recorded requirement, not a zero-version guarantee.

Native CLI checks on a host establish that the scanner command runs there, not that a scanned program runs on a different target. ELF/Mach-O static inspection on Windows similarly does not provide Windows PE support. Cross-compiled arm64 release binaries are not all executed on physical target devices.

HTML escaping and a no-network CSP are tested. A desktop browser preview was attempted but its transport was unavailable; visual layout was not independently inspected. Generated reports remain reproducible from the supplied examples.

CI conclusions and release hashes are recorded with the final release evidence. Historical findings do not certify future formats, operating systems, artifacts or compatibility.

The first CI run's Linux and Windows jobs passed; its `macos-14` job had no runner or steps and was cancelled with `The configuration 'macos-14-arm64-us-default' is not supported`. [GitHub retired those images on 2026-11-02](https://github.com/actions/runner-images/issues/13518). The workflow was corrected to `macos-26`; this cancellation is not counted as a passed macOS test.

The corrected [three-system CI run](https://github.com/feiiiiii5/releasefloor/actions/runs/42235356532) passed on source commit `2bc6ebd493f4b894d28c81f07a7288a78e1748d2`, with Go 1.25.x: Ubuntu 24.04, Windows Server 2022 and macOS 26. Each native command completed all 11 process observations. Subsequent packaging-only changes include the Go runtime's license; they do not change the scanner implementation.

Release commands are cross-compiled with Go 1.27.3 and `CGO_ENABLED=0`, for amd64 and arm64 on Linux, macOS and Windows. The packaged macOS arm64 command was itself extracted and passed its version check plus all 11 process observations. Prebuilt commands for other targets were cross-compiled, not all executed on physical devices. The macOS packages record a minimum OS of 13.0; the Linux packages have no recorded versioned GLIBC imports. Packaging includes the project MIT license and Go's BSD license. `build.json` identifies the exact build source and toolchain; `SHA256SUMS` identifies distributed bytes.

## Published 0.1.0 evidence

The [experimental v0.1.0 release](https://github.com/feiiiiii5/releasefloor/releases/tag/v0.1.0) was published at 2026-11-06 15:10:54 UTC. Its lightweight tag and `build.json` both identify source commit `405ab62c7fd75d7c04a1f47959d93829df58f3be`.

- The [source CI for that exact commit](https://github.com/feiiiiii5/releasefloor/actions/runs/42239730214) passed on Ubuntu 24.04, Windows Server 2022 and macOS 26. Each host ran the 11 native command observations with Go 1.25.x, in addition to source tests and vet.
- The [manual publishing run](https://github.com/feiiiiii5/releasefloor/actions/runs/42240395309) built all six archives with Go 1.27.3 on Linux amd64. It ran the extracted Linux amd64 package through the same 11 observations, checked the four ELF/Mach-O archives, verified all ten server asset digests, then published the draft. Tag/source mismatches, unexpected assets and concurrent publishing runs are guarded. An independent review's two publishing blockers were fixed before dispatch.
- All ten files were then independently downloaded through their public release URLs. Their lengths and SHA-256 values matched the GitHub asset metadata. All nine entries in `SHA256SUMS` matched the downloaded bytes. The checksum file itself has SHA-256 `235c71791056f5be41561247941cf571a6e2c880e723b47cda56180076ee485c`.
- The downloaded macOS arm64 package passed its version check and all 11 process observations using the downloaded examples. All six archive member sets included the proper executable and both license notices; four non-Windows archives passed metadata inspection, and both Windows executable headers matched their advertised architectures. Header checks do not establish Windows package execution.
- `go install github.com/feiiiiii5/releasefloor/cmd/releasefloor@v0.1.0` succeeded using the public Go module proxy and isolated local install/cache directories. That installed macOS arm64 command passed its version check and all 11 process observations, too.

The initial local multi-asset upload timed out and left an incomplete unpublished draft. It was not counted as delivery. The successful manual workflow rebuilt and reconciled those draft assets before publication. This explains why the final public manifest identifies the Linux CI build rather than the earlier local packaging attempt.

Subsequent documentation-only commits clarify installation, record these results and require external maintainer evidence before expanding investment. They do not move the published tag or change its binaries. There are still no verified external users, paid demand or target-program compatibility results.

## Real-World Failure Case Studies & Edge-Case Regression Fixes

### 1. Edge-Case Regression Fixes (Preventing False Passes)

An independent code review and fuzzing inspection identified two critical edge-case boundaries where tampered or non-standard inputs could previously produce a false pass:

1. **ELF PT_LOAD Segment vs. Section Header Consistency**:
   - *Vulnerability*: Previously, `inspectELF` verified that dynamic symbol tags (`DT_STRTAB`, `DT_VERNEED`, `DT_VERSYM`) matched the virtual address (`sh_addr`) in their respective section headers, but did not cross-reference that virtual address against the file offsets defined in the loader's `PT_LOAD` program headers. A modified binary with tampered section file offsets (`sh_offset`) could cause the scanner to read decoupled string tables while the real system dynamic linker mapped the unmodified segment, leading to false passes against ceilings.
   - *Fix*: `inspectELF` now records all `PT_LOAD` segments, maps each metadata section's `sh_addr` through the segment table, and strictly asserts `mappedOffset == sh_offset`. Any discrepancy immediately returns `ErrUnsupported`, rendering inventory coverage incomplete and blocking the gate.
   - *Regression Test*: Added `TestELFSectionSegmentConsistencyAndMachOMandatoryCommands` in `floor/inspect_test.go`.

2. **Mach-O Mandatory Load Commands (`LC_REQ_DYLD`)**:
   - *Vulnerability*: Mach-O load commands with the high bit set (`cmd & 0x80000000 != 0`, i.e., `LC_REQ_DYLD`) are mandatory for `dyld`. Unhandled commands with this flag were previously skipped without error, allowing binaries requiring uninspected dynamic linker features to be reported as complete.
   - *Fix*: `inspectMach` explicitly enumerates all known benign mandatory commands (e.g. `LC_RPATH`, `LC_DYLD_INFO_ONLY`, `LC_MAIN`, `LC_DYLD_EXPORTS_TRIE`, `LC_DYLD_CHAINED_FIXUPS`, `LC_FILESET_ENTRY`). Any unknown mandatory command is appended to `Binary.Unknown`, causing `Complete: false` and blocking the release gate.
   - *Regression Test*: Tested via synthetic Mach-O load command `0x80000099` in `floor/inspect_test.go`.

### 2. Real-World Case Studies

ReleaseFloor directly addresses silent compatibility regressions documented in high-profile open-source releases:

| Project Incident | Root Cause | Impact | How ReleaseFloor Gating Intercepts |
| :--- | :--- | :--- | :--- |
| [rust-analyzer #19215](https://github.com/rust-lang/rust-analyzer/issues/19215) | ARM64 build runner environment changed from Ubuntu 20.04 to Ubuntu 22.04 without oldsysroot isolation. | `rust-analyzer-aarch64-unknown-linux-gnu` silently bumped minimum GLIBC requirement, failing on Debian 11 and enterprise LTS distros. | `releasefloor compare` diffs the candidate release against the previous tag, detects `GLIBC` floor raised (e.g. `2.31 -> 2.35`), and exits with code 1. |
| [Go-Ethereum #32102](https://github.com/ethereum/go-ethereum/issues/32102) | Linux release binaries compiled with CGO linked against a newer host `libc.so.6`. | Geth executable failed to launch on Debian 12 with `version 'GLIBC_2.38' not found`. | `releasefloor check --policy policy.json` enforces `max_floors: {"GLIBC": "2.28"}` across all Linux assets and fails fast before publication. |
| [DuckDB #17632](https://github.com/duckdb/duckdb/issues/17632) | Bundled precompiled dynamic library (`libduckdb.so`) for Go bindings required both newer `GLIBC` and `GLIBCXX_3.4.29`. | Users on supported enterprise distributions encountered unresolved C++ ABI symbol errors. | ReleaseFloor parses both `GLIBC` and `GLIBCXX` version nodes simultaneously from ELF `.gnu.version_r` tables, identifying multi-family floor violations. |

### 3. Comparison with Existing Local Tooling

ReleaseFloor does not attempt to replace ecosystem-specific packaging compilers (like `auditwheel`, `delocate`, or `cargo-dist`). Instead, it serves as a **unified, build-system-agnostic preflight gate** on the final release bundle:

- **vs. `auditwheel` / `delocate`**: Ecosystem tools modify and bundle shared libraries during wheel building. ReleaseFloor inspects the final mixed `.tar.gz`, `.zip`, `.whl`, or `.jar` archive without extraction to ensure no post-packaging drift occurred.
- **vs. `abicheck`**: `abicheck` focuses on library ABI evolution and diffing shared library interfaces. ReleaseFloor focuses on executable and library runtime floor requirements (GLIBC/macOS SDK) and multi-architecture slice consistency across release matrices.
- **vs. Custom Shell Scripts**: ReleaseFloor eliminates fragile `readelf` / `otool` / `lipo` host tool dependencies with zero runtime execution risk and memory-bounded parsing.
