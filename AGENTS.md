# ReleaseFloor

Original, MIT-licensed experimental release-archive preflight. Read `docs/PRODUCT.md` before expanding scope. No target binary is executed. No network service, telemetry, model, credential, installation or source-tree modification is part of the product.

Report recorded runtime requirements, never complete compatibility or safety. Missing, unsupported and unreadable runtime binaries must not produce a successful gate. Preserve artifact hashes, member identities, architecture and source evidence. Static linkage is not a proof of universal portability. Keep field names and exit codes consistent with `docs/CONTRACT.md`.

Use Go standard library only. Inputs are the publisher's own build artifacts, with bounded parsing; do not claim malware-analysis isolation. Keep original synthetic fixtures. External public artifacts may be inspected for validation but are not executed or committed. Workers change only assigned files and never perform Git writes, publication, credential access or delegation. The coordinator handles commits, release and evidence review.
