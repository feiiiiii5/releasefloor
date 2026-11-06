# Contributing

ReleaseFloor is a small experimental preflight for a publisher's own release artifacts. Read [the product decision](docs/PRODUCT.md), [the behavior contract](docs/CONTRACT.md) and [AGENTS.md](AGENTS.md) before changing scope or report semantics.

## Useful reports

For a parser or comparison issue, include the tool version, host system, command, expected result and actual result. A minimal original synthetic fixture is preferred. Do not upload confidential release files, credentials, internal paths or customer data. If you can share an artifact publicly, link to its original release and identify its hash rather than committing a third-party binary to this repository.

Keep observations separate from assumptions. An older incident is not evidence that the latest release remains broken. A successful metadata check is not a successful target-system test.

## Development

Use the Go version declared in `go.mod` or a compatible newer version. The project uses only the Go standard library. Do not add runtime tools, network requests, telemetry or execution of inspected artifacts.

Before submitting a change, run the relevant checks from the repository root:

```sh
go test ./...
go vet ./...
```

Format changed Go files with `gofmt`. State which checks you actually ran and any failures or checks you could not run. These instructions do not imply that a particular revision has passed them.

Tests should expose behavior that could mislead a release maintainer: an increased floor, a missing architecture, a library inside an archive, incomplete metadata, malformed or unsupported input, resource limits, output overwrite protection, or imported text in reports. Use original synthetic fixture builders; do not depend on executing an inspected artifact or contacting a service.

## Keep the check honest

- Preserve original requirement nodes, provider names, hashes and member/platform/architecture identities as evidence.
- Report unknown, unsupported and incomplete coverage. Do not turn a parser error into a passing gate or fabricate a zero-version floor.
- Keep archive mapping explicit. Do not silently guess path renames or architecture exemptions.
- Keep reports deterministic where the contract promises it, with imported names escaped in HTML and no local absolute input paths.
- Validate policy fields and numeric versions rather than silently ignoring unsupported keys. Keep `GLIBC`, `GLIBCXX` and `CXXABI` family names explicit; literal platform/architecture rules require a matrix match, while wildcard rules only set ceilings. A complete inventory without a recorded family requirement can satisfy that ceiling; unknown metadata cannot.
- Keep changes focused. Update the contract and user documentation when externally visible behavior changes.

Changes to format support, policy interpretation or identity matching need a concrete release task and evidence that existing tools do not already solve it adequately. We may decline an expansion to keep the experiment maintainable.

## License and provenance

Submit only work you have the right to contribute under the [MIT license](LICENSE). Keep implementation and synthetic fixtures original; link to reference material rather than copying another project's source or branding. Identify external problem reports and acknowledge their reporters. No special account permissions or credentials are required to use the tool.

Follow [SECURITY.md](SECURITY.md) for security-sensitive findings.
