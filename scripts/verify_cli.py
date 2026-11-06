"""Exercise the built CLI process; never execute a scanned artifact."""
from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import tempfile


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--binary", required=True)
    parser.add_argument("--demo", required=True)
    args = parser.parse_args()
    binary = Path(args.binary).resolve()
    if not binary.exists() and binary.with_suffix(".exe").exists():
        binary = binary.with_suffix(".exe")
    demo = Path(args.demo).resolve()
    before = {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in demo.iterdir() if p.is_file()}
    observations = []

    def run(name: str, arguments: list[str], expected: int, *, read_json: bool = True):
        p = subprocess.run([str(binary), *arguments], text=True, capture_output=True, timeout=30)
        assert p.returncode == expected, (name, p.returncode, p.stderr[:1000])
        observations.append({"case": name, "exit": p.returncode})
        return json.loads(p.stdout) if read_json else p

    old, new, policy = (str(demo / n) for n in ("old.zip", "new.zip", "policy.json"))
    good = run("scan_complete_original_archive", ["scan", "--format", "json", old], 0)
    assert good["status"] == "pass" and len(good["inventories"][0]["binaries"]) == 4
    regression = run("compare_raises_and_removed_slice", ["compare", "--format", "json", old, new], 1)
    codes = [x["code"] for x in regression["issues"]]
    assert codes.count("floor_raised") == 2 and "minimum_os_raised" in codes and "removed_binary" in codes, codes
    run("check_baseline_matrix", ["check", "--format", "json", "--policy", policy, old], 0)
    violation = run("check_missing_target_and_ceilings", ["check", "--format", "json", "--policy", policy, new], 1)
    assert {"expected_target_missing", "policy_floor_exceeded", "policy_os_exceeded"} <= {x["code"] for x in violation["issues"]}
    with tempfile.TemporaryDirectory(prefix="releasefloor-cli-") as directory:
        root = Path(directory)
        report = root / "report.html"
        run("write_html_report", ["compare", "--format", "html", "--output", str(report), old, new], 1, read_json=False)
        original = report.read_bytes()
        assert b"Content-Security-Policy" in original and b"<script" not in original
        run("refuse_existing_report", ["scan", "--output", str(report), old], 2, read_json=False)
        assert report.read_bytes() == original
        run("refuse_source_overwrite", ["scan", "--output", old, old], 2, read_json=False)
        unsupported = root / "program.exe"
        unsupported.write_bytes(b"MZ" + b"\0" * 128)
        partial = run("unsupported_binary_blocks", ["scan", "--format", "json", str(unsupported)], 1)
        assert not partial["inventories"][0]["complete"]
        broken = root / "broken.zip"
        broken.write_bytes(b"PK\3\4truncated")
        run("malformed_archive_fails", ["scan", str(broken)], 2, read_json=False)
        run("no_artifacts_fails", ["check", "--policy", policy], 2, read_json=False)
    after = {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in demo.iterdir() if p.is_file()}
    assert before == after, "source artifact bytes changed"
    observations.append({"case": "all_source_hashes_unchanged", "passed": True})
    print(json.dumps({"platform": __import__("platform").platform(), "checks": observations}, indent=2))


if __name__ == "__main__":
    main()
