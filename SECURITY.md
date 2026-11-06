# Security

ReleaseFloor reads the publisher's own build artifacts. It never executes them, loads their libraries, follows archive symlinks, extracts members to disk or contacts a network service. This reduces some exposure; it does not make the parser an isolation environment for malware or arbitrary hostile inputs.

## Boundaries

- Parsing is bounded by input, member, expanded-byte and member-count limits. Invalid archives, unsafe paths, duplicate identities and exceeded limits are failures.
- Unsupported executable formats, nested archives and missing required metadata must remain visible. An incomplete scan must not be reported as a complete passing gate.
- Artifact and member hashes identify supplied bytes. They do not verify signatures, origin or trustworthiness.
- Reports contain archive member paths, dependency names and other supplied metadata. Review them before publishing. Avoid embedding confidential paths or identifiers in artifacts you intend to share.
- HTML reports escape imported content and contain no scripts or remote assets. Output files must be new; inputs and existing outputs must not be overwritten.
- A report is not a vulnerability scan, a safety certificate or a complete compatibility guarantee. Keep independent artifact verification and target-system tests.
- A policy checks ceilings and the presence of literal platform/architecture targets. Wildcards do not guarantee concrete target presence, and no rule is a complete expected-member manifest. Compare a trusted prior release to detect per-member removals.

## Reporting a concern

For a non-sensitive defect, open a GitHub issue with a minimal original fixture and the observed behavior. Do not include credentials, private artifacts, personal data or internal infrastructure details.

If GitHub's **Report a vulnerability** option is available on this repository's Security page, use it for a sensitive parser, output-handling or disclosure issue. If that option is unavailable, open a minimal issue requesting a private reporting channel without publishing exploit details or confidential material. Do not assume a private reporting channel or response deadline has been established.

Include the revision or version, host system, command, expected boundary and observed result. Do not execute a suspicious artifact to demonstrate the issue. Security reports and fixes are handled on a best-effort basis while the project is experimental; no response-time or long-term maintenance guarantee is offered.
