# ReleaseFloor

Catch a release archive whose recorded runtime requirements changed before you publish it.

ReleaseFloor is an experimental local command-line tool for release maintainers and installer maintainers. It reads ELF and Mach-O executables and accompanying dynamic libraries from a raw binary, ZIP, or tar.gz archive. It inventories their recorded requirements, compares releases by member, platform and architecture, and checks several artifacts against an explicit JSON policy.

For example, a rebuilt CLI may start requiring a newer GLIBC version, a bundled library may raise its minimum macOS version, or an archive may lose an architecture. ReleaseFloor makes those changes visible without running the binaries.

**This is a small open-source experiment.** Its reports describe metadata found in the supplied bytes. They do not certify that an application will run on a target system. Keep target-system smoke tests and your existing release checks.

[中文说明](README.zh-CN.md) · [Exact contract](docs/CONTRACT.md) · [Product decision and alternatives](docs/PRODUCT.md)

[Download a prebuilt command](https://github.com/feiiiiii5/releasefloor/releases/tag/v0.1.0) for Linux, macOS or Windows, or install from source with `go install github.com/feiiiiii5/releasefloor/cmd/releasefloor@v0.1.0`. Windows is supported as a host; PE input inspection is outside 0.1. Extract the release archive and, from that directory, run `./releasefloor version` on Linux/macOS or `.\releasefloor.exe version` in Windows PowerShell. Verify the archive against the release's `SHA256SUMS` through your normal trusted download process. Add its directory to your PATH if you want to invoke the command elsewhere.

The prebuilt macOS commands record a minimum OS of 13.0. [Validation details](docs/VALIDATION.md) separate source CI, inspected public artifacts and execution of packaged commands. Distributions include [Go's license notice](THIRD_PARTY_NOTICES.txt) alongside the project license.

Independent project developed with AI coding assistance. The implementation and synthetic fixtures are original; format references and competing tools are documented in the product notes.

## Reproduce the example

For a prebuilt command, download and unpack `releasefloor-examples.zip` from the same release. In the examples directory, run `/path/to/releasefloor compare old.zip new.zip` (replace `/path/to/releasefloor` with the extracted command's actual path). Expected exit: **1**. `check --policy policy.json old.zip` passes; `new.zip` blocks. The accompanying `releasefloor-demo.html` is a ready-made report of those synthetic files.

To regenerate the examples from source, first clone the repository and enter its root. The generator creates synthetic object-format headers; they are **not runnable programs** and contain no third-party binaries.

```sh
git clone https://github.com/feiiiiii5/releasefloor.git
cd releasefloor
go run ./examples/generate --out output/demo
go build -o releasefloor ./cmd/releasefloor
./releasefloor compare --format html --output output/comparison.html output/demo/old.zip output/demo/new.zip
```

The last command intentionally exits **1**: the candidate raises GLIBC, GLIBCXX and a macOS minimum version, and drops an arm64 universal slice. Open `output/comparison.html` locally to review the exact member evidence. `check --policy output/demo/policy.json output/demo/old.zip` passes; checking `new.zip` blocks. Choose a new output directory/file when repeating the example.

## Build and try it

Build from a checkout with Go 1.25 or newer. The implementation uses only the Go standard library; the built tool needs no Go installation or external commands at runtime.

```sh
go build -o releasefloor ./cmd/releasefloor
./releasefloor scan app-linux-amd64.tar.gz
./releasefloor compare previous.zip candidate.zip
```

These are usage examples, not a record of tests performed on those filenames. Inspect your own release artifact first, then compare it with the previous release.

```sh
./releasefloor scan --format json app-linux-amd64.tar.gz
./releasefloor compare --format html --output comparison.html previous.zip candidate.zip
```

Output goes to stdout unless `--output` names a **new** file. Existing files and inputs are never overwritten. HTML reports are self-contained and contain no remote assets or scripts.

## Check a release matrix

Save a policy such as this as `policy.json`. Set the limits to the requirements you intend to support; the example values are illustrative.

```json
{
  "schema": "releasefloor.policy.v1",
  "rules": [
    {"platform": "elf", "arch": "amd64", "max_floors": {"GLIBC": "2.28"}},
    {"platform": "elf", "arch": "arm64", "max_floors": {"GLIBC": "2.28"}},
    {"platform": "macos", "arch": "amd64", "max_os": "12.0"},
    {"platform": "macos", "arch": "arm64", "max_os": "12.0"}
  ]
}
```

```sh
./releasefloor check --policy policy.json --format json --output matrix.json app-linux-amd64.tar.gz app-linux-arm64.tar.gz app-macos.zip
```

Every inspected binary must match at least one rule, and all matching ceilings apply. A rule with a literal platform and architecture also requires at least one matching binary across the supplied artifacts. The four example rules therefore require all four target dimensions. A `*` selector supplies a ceiling without requiring a particular target to exist.

Platform selectors are `elf`, `macos` or `*`; architecture selectors use the exact reported value or `*`. Supported floor families are `GLIBC`, `GLIBCXX` and `CXXABI`. Unknown policy fields and unsupported family names are errors. If a complete inventory records no requirement for a family, that family does not exceed a ceiling; the tool does not invent a zero-version floor. Unknown or incomplete metadata still blocks.

A policy caps requirements and checks literal target presence; it is not a complete list of expected archive members. Compare against the previous release to detect a missing member or a missing architecture of a particular member.

`elf` identifies the file format, not a proven Linux target. Mach-O platform identifiers come from recorded build metadata. Use the platform and architecture values reported by `scan` when writing a policy.

## What comparison means

Comparison aligns exact archive member path, platform and architecture identities. It blocks removed binaries, increased or newly introduced numeric requirement floors or minimum OS, and incomplete or unknown coverage. New binaries are reported without claiming a previous floor comparison; added direct dependencies request review.

For an existing Mach-O library provider, a raised recorded compatibility version or a weak import becoming required also blocks for review. Weak and required imports are compared separately. This does not establish whether a library's actual ABI is compatible.

Archive paths are significant. If releases contain an outer version directory such as `app-1.0/bin/app` and `app-1.1/bin/app`, explicitly use `--strip-components 1` when both layouts have one wrapper directory:

```sh
./releasefloor compare --strip-components 1 previous.tar.gz candidate.tar.gz
```

ReleaseFloor does not guess renamed paths. Raw single-binary inputs use the stable member name `binary`, so changing the outer filename does not make the binary appear removed.

## Coverage and limits

- Reads supported ELF and Mach-O metadata, including universal Mach-O slices, from raw files, ZIP, tar.gz and `.tgz`.
- Scans regular members by magic rather than filename or executable permission. Accompanying shared libraries are included.
- Does not extract archives, follow symlinks, recurse into nested archives, execute input, fetch dependencies, or contact a service.
- Reports unsupported runtime formats, nested archives, unknown required metadata and incomplete scans explicitly. A failed parser is never a clean result.
- Rejects invalid archives, duplicate identities, unsafe member paths and resource-limit violations. Defaults are 128 MiB input, 64 MiB per member, 256 MiB total regular bytes and 4,096 members.
- Preserves artifact and member hashes, identities and source metadata. Hashes identify bytes; they do not establish authenticity.

Numeric ELF floor gates cover GLIBC, GLIBCXX and CXXABI. Other recorded version nodes remain evidence; their numeric changes are not a general ABI gate. FAT64, sectionless/extended-numbering ELF, compressed runtime metadata and unsupported machine formats fail explicitly. See the [validation record](docs/VALIDATION.md) for exercised environments and remaining limits.

Dynamic loading, transitive dependencies, CPU instruction requirements, kernel requirements and actual runtime behavior are outside this check. Static linkage does not prove universal portability. The tool is intended for the publisher's own build artifacts, with bounded parsing; it is not a malware-analysis isolation environment.

## Reports and CI

`scan`, `compare` and `check` support `--format text|json|html`. JSON reports use `releasefloor.report.v1`. Reports include artifact basenames and archive member names, not local absolute input paths. Member paths, dependency names and other metadata may still contain information you should review before sharing a report.

| Exit | Meaning |
| --- | --- |
| `0` | Implemented checks completed with no blocking finding. |
| `1` | The report contains blocking changes, policy findings or unknown/incomplete coverage. |
| `2` | Invalid invocation, input, policy or an I/O failure. |

A minimal CI step, after building and collecting your release artifacts:

```sh
./releasefloor check --policy policy.json --format json --output releasefloor-matrix.json dist/app-linux-amd64.tar.gz dist/app-linux-arm64.tar.gz dist/app-macos.zip
```

Keep the exit status as part of the job result. Use a fresh report filename for each run. Add `compare` against a trusted prior artifact if the job must detect removals. Acquire and verify prior releases through your existing release process; ReleaseFloor does not download them.

## Alternatives and project status

Start with your existing release script if it already answers this question reliably. [abicheck](docs/PRODUCT.md#alternatives), BLINT, Packslip and cargo-dist cover adjacent or overlapping workflows. ReleaseFloor's experiment is a direct check of mixed CLI archives and an explicit artifact matrix, with missing architectures and incomparable coverage kept visible. It does not replace ABI analysis or packaging systems.

There is no claim of external adoption, commercial validation or comprehensive compatibility. See the [product decision](docs/PRODUCT.md) for the evidence, unknowns and conditions for stopping or expanding the experiment. Changes are recorded in [CHANGELOG.md](CHANGELOG.md).

Contributions follow [CONTRIBUTING.md](CONTRIBUTING.md). Security boundaries and reporting guidance are in [SECURITY.md](SECURITY.md). Original project code is available under the [MIT license](LICENSE).
