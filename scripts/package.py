#!/usr/bin/env python3
"""Local consistency/reproducibility evidence, NOT independent attestation.

Only tracked + nonignored files enter snapshots. No git directory, ignored state,
submodules, symlinks or private-looking files. Release mode requires a clean
worktree and an existing exact-HEAD tag; publication is deliberately absent.
"""
import argparse
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys
import tarfile
import tempfile
import zlib

ROOT = Path(__file__).resolve().parents[1]
TARGETS = (("linux", "amd64"), ("darwin", "arm64"))
MODULES = {
    "github.com/BurntSushi/toml": "v1.5.0",
    "github.com/rivo/uniseg": "v0.4.7",
    "golang.org/x/sys": "v0.49.0",
}
LEGAL_MODULES = {
    "github.com/BurntSushi/toml": [("COPYING", "BurntSushi-TOML-MIT.txt")],
    "github.com/rivo/uniseg": [("LICENSE.txt", "Uniseg-MIT.txt")],
    "golang.org/x/sys": [("LICENSE", "Go-x-sys-BSD.txt"), ("PATENTS", "Go-x-sys-PATENTS.txt")],
}
VERSION_RE = r"v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9]+(?:[.-][A-Za-z0-9]+)*)?"
UNICODE_LEGAL = {
    "Unicode-15-data-license.txt": "68f5b9f5ea36881a0942ba02f558e9e1faf76cc09cb165ad801744c61b738844",
    "Unicode-17-data-license.txt": "68af68845f9302fdc7f4d903cbdb6f1d376cd97e9c753bff087c35917e7d8831",
    "Unicode-data-NOTICE.txt": "99d6e72fa8572a8cf43eecc2b68b1063faa283bba603adddd9f715112e527485",
}
UNISEG_TABLES = {"graphemeproperties.go", "wordproperties.go", "sentenceproperties.go",
                 "lineproperties.go", "eastasianwidth.go", "emojipresentation.go"}
GO_UNICODE_TABLES = {"src/unicode/tables.go", "src/vendor/golang.org/x/net/idna/tables17.0.0.go",
                     "src/vendor/golang.org/x/text/unicode/bidi/tables17.0.0.go",
                     "src/vendor/golang.org/x/text/unicode/norm/tables17.0.0.go"}


def sha(data):
    return hashlib.sha256(data).hexdigest()


def canonical(value):
    return (json.dumps(value, sort_keys=True, indent=2) + "\n").encode()


def run(args, cwd=ROOT, env=None):
    result = subprocess.run(args, cwd=cwd, env=env, stdout=subprocess.PIPE,
                            stderr=subprocess.PIPE, check=False)
    if result.returncode:
        raise RuntimeError(f"command failed ({result.returncode}): {args!r}\n"
                           + result.stderr.decode(errors="replace"))
    return result.stdout


def git(*args, root=ROOT):
    # No inherited alternate index/worktree, config, external diff or textconv.
    env = {"PATH": os.defpath, "LC_ALL": "C", "HOME": "/nonexistent",
           "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull}
    return run(["git", *args], root, env)


def private_path(name):
    parts = Path(name).parts
    return (any(p in {".git", "bin", "dist", "secrets", "data", "__pycache__"}
                or p.startswith((".env", ".atuin-ai-", ".record-")) for p in parts)
            or Path(name).name in {"credentials.json", "auth.json", "config.toml", "config.local.toml"}
            or name.endswith((".token", ".log", ".test", ".prof", ".pyc")))


def inventory(root, names):
    result = []
    for name in sorted(names):
        if (not name or name.startswith("/") or any(p in {".", ".."} for p in name.split("/"))
                or re.search(r"[\x00-\x1f\x7f]", name) or private_path(name)):
            raise RuntimeError(f"unsafe/private source name refused: {name!r}")
        path = root / name
        # Reject symlinked ancestors as well as leaf files.
        for parent in (path, *path.parents):
            if parent == root:
                break
            if parent.is_symlink():
                raise RuntimeError(f"source symlink refused: {name}")
        mode = path.lstat().st_mode
        if not stat.S_ISREG(mode) or path.stat().st_nlink != 1:
            raise RuntimeError(f"nonregular/hardlinked source refused: {name}")
        result.append({"path": name, "mode": "0755" if mode & 0o111 else "0644",
                       "size": path.stat().st_size, "sha256": sha(path.read_bytes())})
    return result


def source(root=ROOT):
    flags = git("ls-files", "-v", "-z", root=root).decode().split("\0")[:-1]
    if any(row[0].islower() or row[0] == "S" for row in flags):
        raise RuntimeError("unsupported assume-unchanged/skip-worktree index flags; clear them before packaging")
    configuration = git("config", "--null", "--list", root=root).decode().split("\0")
    for item in configuration:
        key, _, value = item.partition("\n")
        if key.lower() in {"core.sparsecheckout", "core.sparsecheckoutcone", "index.sparse"} and value.lower() in {"true", "1", "yes", "on", ""}:
            raise RuntimeError("unsupported sparse checkout/index; use a complete worktree")
    tracked = git("ls-files", "-z", root=root).decode().split("\0")[:-1]
    extra = git("ls-files", "--others", "--exclude-standard", "-z", root=root).decode().split("\0")[:-1]
    # Deleted tracked files are honest dirty evidence, not missing inventory entries.
    names = sorted(set(n for n in tracked + extra if (root / n).exists() or (root / n).is_symlink()))
    inv = inventory(root, names)
    status = git("status", "--porcelain=v1", "-z", "--untracked-files=all", root=root)
    diff = git("diff", "--no-ext-diff", "--no-textconv", "--binary", "HEAD", "--", root=root)
    # Status/index hints are NOT proof of actual bytes matching HEAD. Compare
    # every full HEAD blob (including missing tracked files) with the snapshot.
    head = []
    for record in git("ls-tree", "-r", "-z", "--full-tree", "HEAD", root=root).split(b"\0")[:-1]:
        metadata, name = record.split(b"\t", 1)
        mode, kind, oid = metadata.decode().split()
        if kind != "blob" or mode not in {"100644", "100755"}:
            raise RuntimeError("unsupported nonregular HEAD tree entry")
        head.append({"path": name.decode(), "mode": "0" + mode[3:], "oid": oid})
    algorithm = git("rev-parse", "--show-object-format", root=root).decode().strip()
    actual = []
    for entry in inv:
        data = (root / entry["path"]).read_bytes()
        oid = hashlib.new(algorithm, b"blob " + str(len(data)).encode() + b"\0" + data).hexdigest()
        actual.append({"path": entry["path"], "mode": entry["mode"], "oid": oid})
    head = sorted(head, key=lambda row: row["path"])
    matches_head = actual == head
    return inv, {"revision": git("rev-parse", "HEAD", root=root).decode().strip(),
                 "git_tree": git("rev-parse", "HEAD^{tree}", root=root).decode().strip(),
                 "inventory_sha256": sha(canonical(inv)), "dirty": bool(status) or not matches_head,
                 "head_snapshot_match": matches_head, "head_inventory_sha256": sha(canonical(head)),
                 "tracked_diff": bool(diff), "tracked_diff_sha256": sha(diff),
                 "status_sha256": sha(status), "untracked": sorted(extra)}


def snapshot(root, destination, inv):
    destination.mkdir(mode=0o700)
    directories = {p for entry in inv for p in Path(entry["path"]).parents if str(p) != "."}
    for directory in sorted(directories, key=lambda p: (len(p.parts), str(p))):
        (destination / directory).mkdir(mode=0o700)
    for entry in inv:
        target = destination / entry["path"]
        data = (root / entry["path"]).read_bytes()
        if sha(data) != entry["sha256"]:
            raise RuntimeError("source changed while snapshotting")
        target.write_bytes(data)
        target.chmod(int(entry["mode"], 8))
    verify_snapshot(destination, inv)


def verify_snapshot(root, inv):
    # Include unexpected files, directories and symlinks (not just expected hashes).
    expected_files = {e["path"] for e in inv}
    expected_dirs = {str(p) for n in expected_files for p in Path(n).parents if str(p) != "."}
    actual_files, actual_dirs = set(), set()
    for path in root.rglob("*"):
        name = path.relative_to(root).as_posix()
        if path.is_symlink():
            raise RuntimeError("snapshot symlink appeared")
        (actual_dirs if path.is_dir() else actual_files).add(name)
    if (actual_files != expected_files or actual_dirs != expected_dirs
            or any(stat.S_IMODE((root / d).stat().st_mode) != 0o700 for d in actual_dirs | {"."})
            or any(stat.S_IMODE((root / e["path"]).stat().st_mode) != int(e["mode"], 8) for e in inv)
            or inventory(root, expected_files) != inv):
        raise RuntimeError("snapshot complete inventory changed")


def archive(files):
    buf = io.BytesIO()
    with gzip.GzipFile(filename="", mode="wb", fileobj=buf, mtime=0, compresslevel=9) as gz:
        with tarfile.open(fileobj=gz, mode="w", format=tarfile.USTAR_FORMAT) as tar:
            for name, (data, mode) in sorted(files.items()):
                info = tarfile.TarInfo(name)
                info.size, info.mode, info.mtime = len(data), mode, 0
                info.uid = info.gid = 0
                info.uname = info.gname = ""
                tar.addfile(info, io.BytesIO(data))
    return buf.getvalue()


def json_stream(data):
    decoder = json.JSONDecoder()
    text = data.decode().strip()
    result = []
    while text:
        value, end = decoder.raw_decode(text)
        result.append(value)
        text = text[end:].lstrip()
    return result


def build(snapshot_dir, work, env, go, version, identity):
    work.mkdir(mode=0o700)
    build_env = dict(env, GOCACHE=str(work / "cache"))
    # Verify all module contents against go.sum, including the separate proof module.
    run([go, "mod", "download"], snapshot_dir, build_env)
    verification = run([go, "mod", "verify"], snapshot_dir, build_env).decode().strip()
    if verification != "all modules verified":
        raise RuntimeError("module verification did not succeed")
    modules = json_stream(run([go, "list", "-m", "-json", "all"], snapshot_dir, build_env))
    module_dirs = {m["Path"]: Path(m["Dir"]) for m in modules if "Dir" in m}
    legal = {"LICENSE": (snapshot_dir / "LICENSE").read_bytes(),
             "NOTICE": (snapshot_dir / "NOTICE").read_bytes()}
    # Retain applicable protocol/source attribution, not the proof-only JWT text
    # or any external test tooling. Root NOTICE retains its historical scope.
    retained_legal = {name for pairs in LEGAL_MODULES.values() for _, name in pairs}
    retained_legal.update({"Atuin-MIT.txt", "Pi-MIT.txt", "OpenAI-Codex-Apache-2.0.txt", "OpenAI-Codex-NOTICE.txt", *UNICODE_LEGAL})
    for name in sorted(retained_legal):
        legal["LICENSES/" + name] = (snapshot_dir / "LICENSES" / name).read_bytes()
    for name, checksum in UNICODE_LEGAL.items():
        if sha(legal["LICENSES/" + name]) != checksum:
            raise RuntimeError("reviewed Unicode license/attribution is missing or changed: " + name)
    for module, pairs in LEGAL_MODULES.items():
        for upstream, retained in pairs:
            if (module_dirs[module] / upstream).read_bytes() != legal["LICENSES/" + retained]:
                raise RuntimeError("linked module legal text differs: " + module)
    goroot = Path(run([go, "env", "GOROOT"], snapshot_dir, build_env).decode().strip())
    for name in ("LICENSE", "PATENTS"):
        legal["LICENSES/Go-" + name + ".txt"] = (goroot / name).read_bytes()
    # Go's runtime/stdlib vendored libraries have separate required license grants.
    # Include all four stdlib-vendor legal pairs, not compiler/test-tool vendor trees.
    for path in sorted((goroot / "src/vendor").rglob("*")):
        if path.name in {"LICENSE", "PATENTS", "NOTICE"} and path.is_file():
            legal["LICENSES/Go-stdlib-vendor-" + str(path.relative_to(goroot / "src/vendor")).replace("/", "-") + ".txt"] = path.read_bytes()
    graphs = {}
    stdlib_notices = {}
    for goos, arch in TARGETS:
        target_env = dict(build_env, GOOS=goos, GOARCH=arch)
        if goos == "linux":
            target_env["GOAMD64"] = "v1"
        deps = json_stream(run([go, "list", "-mod=readonly", "-deps", "-json", "./cmd/atuin-ai-server"], snapshot_dir, target_env))
        graphs[(goos, arch)] = (target_env, deps)
        # Go also retains permissive third-party notices IN source comments
        # (e.g. Sun fdlibm and Moshier/Cephes math). Preserve notice-bearing
        # comment blocks from every selected stdlib Go/assembly/header file.
        for dep in deps:
            if not dep.get("Standard"):
                continue
            for field in ("GoFiles", "CgoFiles", "CFiles", "HFiles", "SFiles"):
                for name in dep.get(field, []):
                    path = Path(dep["Dir"]) / name
                    text = path.read_text()
                    blocks = re.findall(r"(?m)(?:^[ \t]*//[^\n]*(?:\n|$))+|/\*.*?\*/", text, re.S)
                    notices = [b for b in blocks if re.search(r"copyright|licen[sc]e|permission to", b, re.I)]
                    if notices:
                        stdlib_notices[str(path.relative_to(goroot))] = "\n".join(notices)
    unicode_tables = {}
    for _, deps in graphs.values():
        uniseg_tables, go_tables = set(), set()
        for dep in deps:
            for name in dep.get("GoFiles", []):
                path = Path(dep["Dir"]) / name
                if dep["ImportPath"] == "github.com/rivo/uniseg":
                    text = path.read_text()
                    if "unicode.org/Public/" in text and "Code generated" in text:
                        if "unicode.org/Public/15.0.0/" not in text:
                            raise RuntimeError("unreviewed uniseg Unicode version")
                        uniseg_tables.add(name)
                        unicode_tables["github.com/rivo/uniseg@v0.4.7/" + name] = sha(path.read_bytes())
                elif dep.get("Standard"):
                    relative = str(path.relative_to(goroot))
                    if relative in GO_UNICODE_TABLES:
                        if not re.search(r'(?:UnicodeVersion|Version)\s*=\s*"17\.0\.0"', path.read_text()):
                            raise RuntimeError("unreviewed Go Unicode version")
                        go_tables.add(relative)
                        unicode_tables["go1.27.2/" + relative] = sha(path.read_bytes())
                    elif "tables" in name and any(x in dep["ImportPath"] for x in ("/idna", "/unicode/bidi", "/unicode/norm")):
                        raise RuntimeError("unreviewed Go Unicode table selection")
        if uniseg_tables != UNISEG_TABLES or go_tables != GO_UNICODE_TABLES:
            raise RuntimeError("Unicode attribution completeness check failed")
    legal["LICENSES/Go-stdlib-source-notices.txt"] = b"".join(
        ("===== " + name + " =====\n" + text + "\n").encode()
        for name, text in sorted(stdlib_notices.items()))
    legal_text = b"".join(("===== " + name + " =====\n").encode() + data + b"\n" for name, data in sorted(legal.items()))
    if len(legal_text) > 1048576:
        raise RuntimeError("legal bundle exceeds installer bound; review notices before changing bound")
    assets = {f"atuin-ai-server-{version}-legal.txt": legal_text}
    reports = []
    for goos, arch in TARGETS:
        target_env, deps = graphs[(goos, arch)]
        linked = {d["Module"]["Path"]: d["Module"].get("Version", "") for d in deps
                  if "Module" in d and not d["Module"].get("Main")}
        if linked != MODULES or any("proof" in d["ImportPath"] for d in deps):
            raise RuntimeError("unexpected shipping dependency graph")
        ldflags = (f"-X main.version={version} -X main.revision={identity['revision']} "
                   f"-X main.sourceSHA256={identity['inventory_sha256']} "
                   f"-X main.sourceDirty={str(identity['dirty']).lower()}")
        name = f"atuin-ai-server-{version}-{goos}-{arch}"
        binary = work / name
        flags = ["-mod=readonly", "-trimpath", "-buildvcs=false", "-buildmode=exe", "-ldflags", ldflags]
        run([go, "build", *flags, "-o", str(binary), "./cmd/atuin-ai-server"], snapshot_dir, target_env)
        data = binary.read_bytes()
        metadata = run([go, "version", "-m", str(binary)], snapshot_dir, build_env).decode()
        # Remove only the physical output filename; retain all embedded build settings.
        metadata = metadata.replace(str(binary), "atuin-ai-server", 1)
        if "CGO_ENABLED=0" not in metadata or "-trimpath=true" not in metadata:
            raise RuntimeError("missing controlled build settings")
        files = {"atuin-ai-server": (data, 0o755), **{n: (b, 0o644) for n, b in legal.items()}}
        packed = archive(files)
        assets[name] = data
        assets[name + ".tar.gz"] = packed
        reports.append({"target": f"{goos}/{arch}", "flags": flags, "binary": name,
                        "binary_sha256": sha(data), "archive": name + ".tar.gz",
                        "archive_sha256": sha(packed), "go_version_m": metadata,
                        "linked_modules": linked, "packages": sorted(d["ImportPath"] for d in deps),
                        "archive_inventory": [{"path": n, "mode": oct(m), "size": len(b), "sha256": sha(b)}
                                              for n, (b, m) in sorted(files.items())]})
    if run([go, "mod", "verify"], snapshot_dir, build_env).decode().strip() != verification:
        raise RuntimeError("modules changed during build")
    host = run([go, "env", "GOHOSTOS", "GOHOSTARCH"], snapshot_dir, build_env).decode().splitlines()
    tool_prefix = "pkg/tool/" + "_".join(host) + "/"
    return assets, reports, {"module_verification": verification, "build_host": "/".join(host),
                            "unicode_attribution": {"versions": {"uniseg": "15.0.0", "go": "17.0.0"},
                                                    "table_sources_sha256": dict(sorted(unicode_tables.items())),
                                                    "reviewed_legal_sha256": UNICODE_LEGAL},
                            "module_integrity": [{k: m[k] for k in ("Path", "Version", "Sum", "GoModSum") if k in m}
                                                 for m in modules if not m.get("Main")],
                            "toolchain_executables_sha256": {name: sha((goroot / name).read_bytes()) for name in
                                ("bin/go", tool_prefix + "compile", tool_prefix + "link")},
                            "legal_inventory": {n: sha(b) for n, b in sorted(legal.items())}}


def package(args):
    if not re.fullmatch(VERSION_RE, args.version) or len(args.version) > 100:
        raise RuntimeError("unsafe version tag")
    inv, identity = source()
    if not args.development:
        if identity["dirty"]:
            raise RuntimeError("release blocked: source must be reviewed and CLEAN; use --development only for validation")
        tag_revision = git("rev-parse", "--verify", "refs/tags/" + args.version + "^{commit}").decode().strip()
        if tag_revision != identity["revision"]:
            raise RuntimeError("release tag must already identify exact HEAD")
    out = Path(os.path.abspath(args.out))
    # Owned fresh output only, with no symlink ancestry; never replace old evidence.
    for p in (out, *out.parents):
        if p.is_symlink():
            raise RuntimeError("symlink output path refused")
    if out.exists():
        raise RuntimeError("output must not exist (preserve earlier failures/evidence)")
    if out.is_relative_to(ROOT):
        relative = str(out.relative_to(ROOT))
        try:
            git("check-ignore", "-q", "--", relative)
        except RuntimeError as error:
            raise RuntimeError("in-repository output must be ignored") from error
    os.umask(0o077)
    out.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
    out.mkdir(mode=0o700)
    try:
        go = shutil.which("go")
        if not go:
            raise RuntimeError("go launcher missing")
        pin = re.search(r"^go ([0-9.]+)$", (ROOT / "go.mod").read_text(), re.M).group(1)
        if pin != "1.27.2":
            raise RuntimeError("review exact toolchain change before packaging")
        with tempfile.TemporaryDirectory(prefix="packaging-", dir=out) as scratch:
            scratch = Path(scratch)
            home = scratch / "empty-home"
            home.mkdir(mode=0o700)
            env = {"PATH": os.defpath, "HOME": str(home), "LANG": "C", "LC_ALL": "C", "TZ": "UTC",
                   "GOENV": "off", "GOWORK": "off", "GOTOOLCHAIN": "go" + pin,
                   "GOPATH": str(Path.home() / "go"), "GOMODCACHE": str(Path.home() / "go/pkg/mod"),
                   "GOPROXY": "https://proxy.golang.org", "GOSUMDB": "sum.golang.org",
                   "CGO_ENABLED": "0", "GOFLAGS": "", "GOEXPERIMENT": "", "GOFIPS140": "off"}
            if run([go, "env", "GOVERSION"], env=env).decode().strip() != "go" + pin:
                raise RuntimeError("incorrect toolchain")
            first = None
            paths = []
            for label in ("first-short", "second-different-absolute-path"):
                container = scratch / label
                container.mkdir(mode=0o700)
                tree = container / "source"
                snapshot(ROOT, tree, inv)
                paths.append(str(tree))
                result = build(tree, container / "work", env, go, args.version, identity)
                verify_snapshot(tree, inv)
                if first is None:
                    first = result
                elif result != first:
                    raise RuntimeError("two-path same-target binary/archive/legal/metadata reproducibility FAILED")
            # Never label a build with an earlier source identity after a concurrent edit.
            if source() != (inv, identity):
                raise RuntimeError("original source changed during build")
            assets, targets, legal = first
            installer = (ROOT / "scripts/install.sh").read_bytes()
            if installer.count(b"RELEASE_VERSION='UNPUBLISHED'") != 1:
                raise RuntimeError("installer tag sentinel missing")
            assets["install.sh"] = installer.replace(b"RELEASE_VERSION='UNPUBLISHED'", f"RELEASE_VERSION='{args.version}'".encode())
            manifest = {"schema": 1, "version": args.version, "mode": "DEVELOPMENT; RELEASE BLOCKED" if args.development else "CLEAN TAGGED; publication requires separate approval",
                        "source": identity, "source_inventory": inv, "toolchain": "go" + pin,
                        "packaging_toolchain": {"python": sys.version.split()[0], "zlib": zlib.ZLIB_VERSION,
                                                "zlib_runtime": zlib.ZLIB_RUNTIME_VERSION},
                        "environment": {k: v for k, v in env.items() if k not in {"HOME", "GOPATH", "GOMODCACHE"}},
                        "targets": targets, **legal,
                        "reproducibility": {"same_target_two_absolute_paths": True, "paths": paths,
                                            "independent_attestation": False},
                        "assets": {n: {"size": len(b), "sha256": sha(b)} for n, b in sorted(assets.items())}}
            assets["manifest.json"] = canonical(manifest)
            assets["SHA256SUMS"] = b"".join(f"{sha(b)}  {n}\n".encode() for n, b in sorted(assets.items()))
            for name, data in assets.items():
                (out / name).write_bytes(data)
                (out / name).chmod(0o755 if name == "install.sh" or name in {r["binary"] for r in targets} else 0o644)
        print(f"Packaged {out}\n{manifest['mode']}\nTwo-path local consistency only; NOT independent attestation.")
    except BaseException:
        (out / "FAILED").write_text("Packaging failed; no release usable. Preserve diagnostic logs.\n")
        raise


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--version", required=True)
    parser.add_argument("--out", required=True)
    parser.add_argument("--development", action="store_true", help="explicit DEVELOPMENT-only snapshot, RELEASE BLOCKED even if clean")
    package(parser.parse_args())
