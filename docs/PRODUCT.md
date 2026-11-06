# Product decision: a small release preflight experiment

Decision date: 2026-11-06. **GO for a bounded open-source experiment; commercial viability and adoption are unvalidated.**

ReleaseFloor checks a publisher's own release artifacts for recorded runtime-requirement changes and missing binary identities. The experiment asks whether directly inventorying mixed CLI archives and checking an explicit artifact matrix gives maintainers useful evidence that their current release process lacks.

The name was searched for an exact repository collision during discovery. That is not a trademark search or a guarantee of a unique brand. This project uses an original implementation, original synthetic fixtures, the Go standard library and the MIT license. It does not copy third-party source or branding.

## Candidate ranking

Three candidate workflows were compared before implementation. This ranking is a decision about where to run a small experiment, not a market-size ranking.

| Rank | Candidate | Evidence and decision |
| --- | --- | --- |
| 1 | Release archive inventory and artifact-matrix gate | Real release and installer maintenance reports show a need to manage runtime baselines. Existing tools overlap substantially. Try the narrower combination of direct mixed CLI archive inventory, member/platform/architecture comparison and explicit unknown/incomparable coverage. |
| 2 | Cross-evaluation result audit | Discovery found [errorbars](https://github.com/antonsoo/errorbars), a direct existing alternative. The research did not establish a sufficiently useful remaining gap. Do not build another generic evaluation-audit product. |
| 3 | Workbook privacy preflight before sharing | Real needs exist, including Mac users without the familiar Windows inspection workflow. Existing local open-source and free tools directly cover the proposed inspection. Do not start a separate workbook-inspection product. |

The workbook candidate was rejected after checking [Microsoft's built-in Document Inspector](https://support.microsoft.com/en-us/office/collab-files/remove-hidden-data-and-personal-information-by-inspecting-documents-presentations-or-workbooks), [Latenspect](https://github.com/xvorki/latenspect), [Office Privacy Inspector](https://github.com/ttomohisa/htmlapps-office-privacy-inspector) and the [free browser-local XLSX Deep Inspector](https://mlab.sh/tool/xlsx-inspector). The open-source Office Privacy Inspector already documents hidden/very-hidden sheets, hidden rows/columns, links, connections, Pivot/Slicer caches and embedded objects. Latenspect also provides local reports and explicit incomplete results. Absence of a familiar Mac menu command alone did not justify another product. Discovery reviewed public descriptions; it did not establish exhaustive runtime accuracy of those tools.

## Evidence for the release workflow

- [gnolang/gno #6284](https://github.com/gnolang/gno/issues/6284) was open in the discovery snapshot. It is a release-compatibility demand signal, not evidence that the newest artifact remains broken. No current target-system reproduction is claimed here.
- [taiki-e/install-action #998](https://github.com/taiki-e/install-action/issues/998) demonstrates recurring installer-maintenance work around release requirements. This is relevant to maintainers who consume upstream CLI artifacts, as well as publishers who build them.
- [MordechaiHadad/bob #344](https://github.com/MordechaiHadad/bob/issues/344) and [santhosh-tekuri/boon #29](https://github.com/santhosh-tekuri/boon/issues/29) are historical incidents. They support the existence of the failure mode; neither has been retested against its latest release for this experiment.

These are reports by their respective authors and maintainers. They are not findings discovered by ReleaseFloor, customer interviews, endorsements or evidence of adoption. Public issues do not authorize promotion in their threads.

The product hypothesis is an inference from these tasks: a maintainer may benefit from seeing the actual requirements and missing identities across the files they are about to ship. The evidence does not yet show how often maintainers need a separate tool, whether existing scripts are insufficient, or whether they will keep using this one.

## Alternatives

Use an existing release script or tool when it already gives adequate evidence. Language choice, offline execution and a single executable are implementation choices, not sufficient product differentiation.

| Alternative | Direct overlap | Consequence for this experiment |
| --- | --- | --- |
| [abicheck environment-drift analysis](https://github.com/abicheck/abicheck/blob/main/docs/learn/environment-drift.md) | Existing environment-drift and ABI analysis directly overlap with requirement changes. In the examined [package-extraction path](https://github.com/abicheck/abicheck/blob/main/abicheck/extract/packages.py#L1383), shared-library selection excludes PIE executables. | Do not claim that abicheck has no executable or archive support across the whole project. Test whether a direct inventory of CLI executables together with bundled libraries offers a practical gap for the specific release task. |
| [BLINT](https://github.com/owasp-dep-scan/blint) | Binary inspection and diff/baseline workflows already cover individual-binary comparisons. | ReleaseFloor must earn its place through archive/member coverage and an explicit multi-artifact gate. A single-binary floor diff alone is not a new product. |
| [Packslip host requirements](https://packslip.dev/docs/host-requirements/) | Reporting host requirements for shipped software overlaps with the underlying question. | Evaluate the user's existing packaging workflow first. Do not claim that runtime-baseline reporting itself is novel. |
| [cargo-dist `min-glibc-version`](https://axodotdev.github.io/cargo-dist/book/reference/config.html#min-glibc-version) | A release system already has GLIBC baseline configuration. | A cargo-dist user may be sufficiently served by that system. The experiment targets direct inspection of supplied mixed artifacts, without requiring adoption of a packaging system. |
| A project-specific script | A short script can inspect a known binary layout and enforce its intended versions. | If that script is reliable and the release shape is stable, a new tool may add little value. Prefer the script. |

The proposed distinction is narrow: examine supported runtime binaries throughout a supplied archive, preserve member/platform/architecture identity, compare the whole inventory, and check several artifacts against explicit ceilings. Unsupported, unknown, missing or incomparable coverage stays visible instead of becoming a passing result. This is a hypothesis to test against the alternatives, not a claim that none can be configured to accomplish the same task.

## Intended user and first useful result

The initial user is a CLI release maintainer or an installer-tool maintainer who already has build artifacts. The first useful result is one scan of an actual release archive showing the supported executables and accompanying libraries, their recorded requirements and any coverage gaps. A synthetic demonstration alone does not establish user value.

The next step is comparison with a trusted prior artifact, followed by an explicit policy if the report matches the maintainer's needs. There is no requirement to upload files, create an account, run a daemon, modify a source tree or install a new packaging system.

Expected use is tied to releases and changes in CI images, compilers, toolchains or dependencies. This is a proposed usage pattern, not measured frequency. The relevant cost is the maintainer's inspection and diagnosis time; no measured time saving or willingness to pay is claimed. Local execution avoids a per-scan cloud service cost but does not remove parser and support maintenance costs.

## Scope and maintenance responsibility

The [contract](CONTRACT.md) is the precise behavior boundary. Initial inputs are raw ELF/Mach-O, ZIP and tar.gz. ELF platform is reported as `elf`; its file format does not prove a Linux target. Mach-O platform comes from recorded metadata. The tool reads bytes and never executes them.

`scan` inventories supported runtime binaries. `compare` aligns explicit member/platform/architecture identities, with a user-selected path prefix stripping option. `check --policy` checks a supplied matrix against JSON rules. Every binary must match a rule and every applicable ceiling is enforced. A literal platform/architecture rule must match at least one binary in the full matrix; a wildcard rule limits requirements without guaranteeing a concrete target's presence. A missing literal target blocks with `expected_target_missing`.

Policy platform selectors are `elf`, `macos` and `*`; architecture selectors preserve the reported value or use `*`. Supported requirement families are `GLIBC`, `GLIBCXX` and `CXXABI`, so a misspelled family does not silently become an unused ceiling. A complete inventory with no recorded requirement for a family can satisfy that family's ceiling, including a static binary without GLIBC requirements. Unknown or incomplete required metadata still blocks. Text, JSON and self-contained HTML make the evidence inspectable. A policy is not a complete expected-member manifest; comparison detects per-member removals and lost member architectures.

The project must maintain bounded parsing, deterministic identities, version comparison, clear unknown states, useful source evidence and safe output handling. Universal slices, bundled libraries, malformed archives and absent metadata matter because skipping them can create false confidence. Unsupported inputs and parser failures must be explicit.

The tool does not establish complete ABI compatibility, transitive dependency resolution, dynamic loading behavior, CPU instruction support, kernel requirements or successful execution. Static linking is not universal portability. Archive hashes identify bytes rather than origin or safety. Target-system tests and trusted artifact acquisition remain the publisher's responsibility.

## Market and distribution unknowns

No defensible market headcount, SAM, adoption rate, usage frequency or paid demand has been measured. Repository stars and release downloads are not counts of relevant maintainers. Public incident counts are not a market denominator. The observed market evidence is limited to concrete maintenance tasks and substantial competing supply.

Initial distribution is the project's own public GitHub repository and a copyable CI guide in the README. No issue-thread advertising, unsolicited maintainer outreach, mailing campaign or automatic integration is part of this experiment. Documentation should let a maintainer reproduce a check on their own artifact without contacting the author.

Open-source operation still carries maintenance work: format changes, parsing corner cases, noisy unknown results, report stability and compatibility expectations. It has no established service-level promise or long-term support commitment. The experiment should not accumulate formats and integrations merely to appear more complete.

## Continue, stop or expand

Continue only long enough to establish whether the narrow check helps a real release decision. Verification must include behavior the tool can get wrong, particularly missed library members, lost architectures, raised requirements and incomplete metadata. A successful build or attractive report does not answer that question.

Stop or contribute to an existing tool if current scripts or the alternatives already handle the task adequately, if the remaining difference is mainly presentation, or if maintaining reliable coverage costs more than the demonstrated benefit. Do not create artificial shortcomings in competing tools to keep the project alive.

Expand investment only after **two independent external maintainers use the output in real release or installer-maintenance decisions**. This is an evidence target, not a forecast or a claim of current adoption. Record what the output changed, what the existing process missed and whether the tool was used again. Our own releases, implementation tests and validation runs do not meet that threshold.

## Verification status

This document records research, scope and a bounded decision to experiment. It does not claim completed tests, inspected real-world releases, external users or compatibility certification. The coordinating implementation work records actual commands, fixtures, results and limitations with the final reviewed revision; those results must not be inferred from this document.
