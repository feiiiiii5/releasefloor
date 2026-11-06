"""Build reproducible, dependency-free release archives from the current commit."""
from __future__ import annotations

import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tarfile
import zipfile


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--out", default="dist")
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[1]
    out = (root / args.out).resolve()
    if out.exists():
        raise SystemExit("output directory must be new")
    out.mkdir(parents=True)
    env = {k: v for k, v in os.environ.items() if k not in {"SSH_AUTH_SOCK", "GH_TOKEN", "GITHUB_TOKEN"}}
    env.update({"CGO_ENABLED": "0", "GOTOOLCHAIN": "local"})
    commit = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=root, text=True, timeout=10).strip()
    version = subprocess.check_output(["go", "run", "./cmd/releasefloor", "version"], cwd=root, env=env, text=True, timeout=120).strip()
    build_go = subprocess.check_output(["go", "version"], env=env, text=True, timeout=10).strip()
    metadata = {"version": version, "commit": commit, "go": build_go, "cgo_enabled": False, "artifacts": []}
    for platform in ("linux", "darwin", "windows"):
        for arch in ("amd64", "arm64"):
            name = "releasefloor.exe" if platform == "windows" else "releasefloor"
            raw = out / name
            subprocess.run(
                ["go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w -buildid=", "-o", str(raw), "./cmd/releasefloor"],
                cwd=root, env={**env, "GOOS": platform, "GOARCH": arch}, check=True, timeout=240,
            )
            payload = raw.read_bytes()
            base = f"releasefloor-{version}-{platform}-{arch}"
            members = ((name, payload), ("LICENSE", (root / "LICENSE").read_bytes()), ("THIRD_PARTY_NOTICES.txt", (root / "THIRD_PARTY_NOTICES.txt").read_bytes()))
            if platform == "windows":
                dest = out / (base + ".zip")
                with zipfile.ZipFile(dest, "x", zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
                    for member, content in members:
                        info = zipfile.ZipInfo(member, date_time=(1980, 1, 1, 0, 0, 0))
                        info.compress_type = zipfile.ZIP_DEFLATED
                        info.external_attr = (0o100755 if member == name else 0o100644) << 16
                        archive.writestr(info, content)
            else:
                dest = out / (base + ".tar.gz")
                with dest.open("xb") as stream, gzip.GzipFile(filename="", fileobj=stream, mode="wb", mtime=0) as gz, tarfile.open(fileobj=gz, mode="w") as archive:
                    for member, content in members:
                        info = tarfile.TarInfo(member)
                        info.size, info.mode, info.mtime = len(content), (0o755 if member == name else 0o644), 0
                        archive.addfile(info, io.BytesIO(content))
            raw.unlink()
            metadata["artifacts"].append({"name": dest.name, "sha256": hashlib.sha256(dest.read_bytes()).hexdigest(), "os": platform, "arch": arch})
    manifest = out / "build.json"
    manifest.write_text(json.dumps(metadata, indent=2) + "\n", encoding="utf-8")
    files = sorted(p for p in out.iterdir() if p.is_file())
    (out / "SHA256SUMS").write_text("".join(f"{hashlib.sha256(p.read_bytes()).hexdigest()}  {p.name}\n" for p in files), encoding="ascii")
    print(json.dumps({"commit": commit, "version": version, "artifacts": len(metadata["artifacts"]), "directory": str(out)}))


if __name__ == "__main__":
    main()
