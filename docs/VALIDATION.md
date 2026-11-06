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
