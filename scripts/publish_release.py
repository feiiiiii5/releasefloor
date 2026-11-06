"""Finish the explicitly selected draft after verifying a fresh CI build."""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
from urllib.parse import quote
from urllib.error import HTTPError
from urllib.request import Request, urlopen
import zipfile


def require_matching_tag(request, repo: str, tag: str, expected: str, *, allow_missing: bool) -> None:
    try:
        value = request(f"https://api.github.com/repos/{repo}/git/ref/tags/{quote(tag, safe='')}")
    except HTTPError as error:
        error.close()
        if error.code == 404 and allow_missing:
            return
        raise
    obj = value["object"]
    for _ in range(8):
        if obj["type"] == "commit":
            if obj["sha"] != expected:
                raise ValueError("existing release tag points at a different source commit")
            return
        if obj["type"] != "tag":
            raise ValueError("release tag does not resolve to a commit")
        obj = request(f"https://api.github.com/repos/{repo}/git/tags/{obj['sha']}")["object"]
    raise ValueError("release tag indirection exceeds limit")


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--release-id", required=True, type=int)
    parser.add_argument("--directory", required=True, type=Path)
    parser.add_argument("--examples", required=True, type=Path)
    parser.add_argument("--tool", required=True, type=Path)
    args = parser.parse_args()
    repo = os.environ.get("GITHUB_REPOSITORY")
    if repo != "feiiiiii5/releasefloor" or not os.environ.get("GH_TOKEN"):
        raise SystemExit("publishing is restricted to this repository's authenticated CI")
    token = os.environ["GH_TOKEN"]
    headers = {"Authorization": "Bearer " + token, "Accept": "application/vnd.github+json", "User-Agent": "ReleaseFloor-publisher"}

    def request(url: str, method: str = "GET", data: bytes | None = None, content_type: str = "application/json"):
        if not (url.startswith("https://api.github.com/repos/" + repo + "/") or url.startswith("https://uploads.github.com/repos/" + repo + "/")):
            raise ValueError("unexpected publishing destination")
        with urlopen(Request(url, data=data, headers={**headers, "Content-Type": content_type}, method=method), timeout=55) as response:
            return json.load(response) if response.status != 204 else None

    root = args.directory.resolve()
    build = json.loads((root / "build.json").read_text())
    if build["commit"] != os.environ.get("GITHUB_SHA") or build["version"] != "0.1.0":
        raise SystemExit("build source/version does not match this CI run")
    platforms = {(platform, arch) for platform in ("linux", "darwin", "windows") for arch in ("amd64", "arm64")}
    if len(build["artifacts"]) != 6 or {(a["os"], a["arch"]) for a in build["artifacts"]} != platforms:
        raise SystemExit("expected all six distinct host targets")
    archives = {f"releasefloor-0.1.0-{platform}-{arch}." + ("zip" if platform == "windows" else "tar.gz") for platform, arch in platforms}
    if {a["name"] for a in build["artifacts"]} != archives:
        raise SystemExit("unexpected host archive names")
    clean_env = {k: v for k, v in os.environ.items() if k not in {"GH_TOKEN", "GITHUB_TOKEN", "SSH_AUTH_SOCK"}}
    for asset in build["artifacts"]:
        path = root / asset["name"]
        if hashlib.sha256(path.read_bytes()).hexdigest() != asset["sha256"]:
            raise SystemExit("archive digest changed")
        if asset["os"] != "windows":
            check = subprocess.run([str(args.tool.resolve()), "scan", "--format", "json", str(path)], capture_output=True, text=True, env=clean_env, timeout=30)
            if check.returncode or len(json.loads(check.stdout)["inventories"][0]["binaries"]) != 1:
                raise SystemExit("built archive did not pass metadata inspection")
    with tempfile.TemporaryDirectory(prefix="releasefloor-package-") as directory:
        binary = Path(directory) / "releasefloor"
        with tarfile.open(root / "releasefloor-0.1.0-linux-amd64.tar.gz", "r:gz") as archive:
            if set(archive.getnames()) != {"releasefloor", "LICENSE", "THIRD_PARTY_NOTICES.txt"}:
                raise SystemExit("unexpected package members")
            binary.write_bytes(archive.extractfile("releasefloor").read())
        binary.chmod(0o700)
        subprocess.run(["python3", "scripts/verify_cli.py", "--binary", str(binary), "--demo", str(args.examples)], check=True, env=clean_env, timeout=90)
    report = root / "releasefloor-demo.html"
    result = subprocess.run([str(args.tool.resolve()), "compare", "--format", "html", "--output", str(report), str(args.examples / "old.zip"), str(args.examples / "new.zip")], capture_output=True, text=True, env=clean_env, timeout=30)
    if result.returncode != 1:
        raise SystemExit("example must produce a blocked report")
    with zipfile.ZipFile(root / "releasefloor-examples.zip", "x", zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
        for path in sorted(args.examples.iterdir()):
            if path.is_file():
                info = zipfile.ZipInfo(path.name, (1980, 1, 1, 0, 0, 0))
                info.compress_type = zipfile.ZIP_DEFLATED
                archive.writestr(info, path.read_bytes())
        archive.writestr("QUICKSTART.txt", "Unpack these original synthetic metadata fixtures; they are NOT runnable programs. Run releasefloor compare old.zip new.zip: expected exit 1. Check old.zip with policy.json: exit 0; check new.zip: exit 1. Use --format html --output NEW_REPORT.html before input paths.\n")
    files = sorted(p for p in root.iterdir() if p.is_file() and p.name != "SHA256SUMS")
    (root / "SHA256SUMS").write_text("".join(hashlib.sha256(p.read_bytes()).hexdigest() + "  " + p.name + "\n" for p in files), encoding="ascii")
    files = sorted(p for p in root.iterdir() if p.is_file())
    if {p.name for p in files} != archives | {"build.json", "SHA256SUMS", "releasefloor-demo.html", "releasefloor-examples.zip"}:
        raise SystemExit("unexpected release asset set")
    api = f"https://api.github.com/repos/{repo}/releases/{args.release_id}"
    def require_draft():
        value = request(api)
        if not value["draft"] or value["tag_name"] != "v0.1.0":
            raise SystemExit("only the selected unpublished v0.1.0 draft may be changed")
        return value
    release = require_draft()
    require_matching_tag(request, repo, "v0.1.0", build["commit"], allow_missing=True)
    names = {p.name for p in files}
    existing = {a["name"]: a for a in release["assets"]}
    if set(existing) - names:
        raise SystemExit("draft contains unexpected assets; refusing to remove them")
    for path in files:
        require_draft()
        digest = "sha256:" + hashlib.sha256(path.read_bytes()).hexdigest()
        prior = existing.get(path.name)
        if prior and prior.get("digest") == digest and prior.get("state") == "uploaded":
            continue
        if prior:
            request(f"https://api.github.com/repos/{repo}/releases/assets/{prior['id']}", "DELETE")
        asset = request(f"https://uploads.github.com/repos/{repo}/releases/{args.release_id}/assets?name={quote(path.name)}", "POST", path.read_bytes(), "application/octet-stream")
        if asset.get("digest") != digest or asset["size"] != path.stat().st_size:
            raise SystemExit("uploaded asset digest mismatch")
        print(json.dumps({"uploaded": path.name, "bytes": asset["size"]}), flush=True)
    release = require_draft()
    server = {a["name"]: a for a in release["assets"]}
    if set(server) != names or any(server[p.name].get("digest") != "sha256:" + hashlib.sha256(p.read_bytes()).hexdigest() for p in files):
        raise SystemExit("draft readback mismatch")
    require_matching_tag(request, repo, "v0.1.0", build["commit"], allow_missing=True)
    release = request(api, "PATCH", json.dumps({"draft": False, "prerelease": True, "target_commitish": build["commit"]}).encode())
    require_matching_tag(request, repo, "v0.1.0", build["commit"], allow_missing=False)
    print(json.dumps({"published": release["html_url"], "source": build["commit"], "verified_assets": len(files)}), flush=True)


if __name__ == "__main__":
    main()
