# Changelog

## Unreleased — 0.1.0 experiment

The initial scope is local inspection of raw ELF/Mach-O files and ZIP/tar.gz release archives, including executable members, accompanying dynamic libraries and universal Mach-O slices.

- Inventory recorded runtime requirements and preserve artifact/member hashes and identities.
- Compare releases by explicit member/platform/architecture identity, reporting floor increases, removals, dependency changes and incomplete coverage.
- Check multiple artifacts against a JSON policy with explicit ceilings and literal platform/architecture presence checks; wildcard rules set ceilings without requiring concrete targets.
- Produce text, JSON and self-contained HTML reports without executing inputs or using the network.
- Bound parsing and reject invalid archives, unsafe paths, duplicate identities and output overwrites.

This entry records the initial product scope. It does not assert a published release, completed validation, external adoption or complete runtime compatibility. Verification evidence belongs with the revision or release that was actually checked. The behavior contract is in [docs/CONTRACT.md](docs/CONTRACT.md).
