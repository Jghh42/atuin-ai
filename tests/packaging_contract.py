"""Independent consumer contract for Phase 9a assets, not producer attestation.

Do not import package.py: expected asset/member sets, source inspection and legal
pins must not come from the self-described manifest or producer's inventory code.
Go1.27.2-only legal/table pins are deliberate review gates on toolchain updates.
"""
import gzip
import hashlib
import io
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parents[1]
TARGETS = {"linux/amd64", "darwin/arm64"}
MODULES = {"github.com/BurntSushi/toml": "v1.5.0", "github.com/rivo/uniseg": "v0.4.7", "golang.org/x/sys": "v0.49.0"}
SOURCE_LEGAL = {"LICENSE", "NOTICE", *{"LICENSES/" + name for name in (
    "Atuin-MIT.txt", "BurntSushi-TOML-MIT.txt", "Go-x-sys-BSD.txt", "Go-x-sys-PATENTS.txt",
    "OpenAI-Codex-Apache-2.0.txt", "OpenAI-Codex-NOTICE.txt", "Pi-MIT.txt", "Uniseg-MIT.txt",
    "Unicode-15-data-license.txt", "Unicode-17-data-license.txt", "Unicode-data-NOTICE.txt")}}
BSD = "911f8f5782931320f5b8d1160a76365b83aea6447ee6c04fa6d5591467db9dad"
PATENTS = "96f408bfae65bf137fc2525d3ecb030271c50c1e90799f87abf8846d8dd505cc"
GO_LEGAL = {"LICENSES/Go-LICENSE.txt": BSD, "LICENSES/Go-PATENTS.txt": PATENTS,
            # Native init adds debug/buildinfo, os/exec and os/user standard-library
            # sources; reviewed additive notices include FreeBSD ELF's BSD-2 clause.
            "LICENSES/Go-stdlib-source-notices.txt": "99df598e700d9967c194f5993b5b1095c0f007a0a15d2c2dc07e89fffce9b265"}
for module in ("crypto", "net", "sys", "text"):
    GO_LEGAL[f"LICENSES/Go-stdlib-vendor-golang.org-x-{module}-LICENSE.txt"] = BSD
    GO_LEGAL[f"LICENSES/Go-stdlib-vendor-golang.org-x-{module}-PATENTS.txt"] = PATENTS
UNICODE_LEGAL = {
    "Unicode-15-data-license.txt": "68f5b9f5ea36881a0942ba02f558e9e1faf76cc09cb165ad801744c61b738844",
    "Unicode-17-data-license.txt": "68af68845f9302fdc7f4d903cbdb6f1d376cd97e9c753bff087c35917e7d8831",
    "Unicode-data-NOTICE.txt": "99d6e72fa8572a8cf43eecc2b68b1063faa283bba603adddd9f715112e527485",
}
TABLES = {
    "github.com/rivo/uniseg@v0.4.7/graphemeproperties.go": "321483625d9a4aa92e4a5730df51c59d5b49725cd20e3879749488e5628b3da8",
    "github.com/rivo/uniseg@v0.4.7/wordproperties.go": "10136e6c3c8ea619276428d59278e312aa4ce33b2e50d98f3a618f162b3a5864",
    "github.com/rivo/uniseg@v0.4.7/sentenceproperties.go": "272502600803b8e99721cb9d76002a6546ea2350056bf66bb7860c39a2032bd9",
    "github.com/rivo/uniseg@v0.4.7/lineproperties.go": "d73432ab989195312fdaa4f4dcea34c7bfb478160b98ad26fbc31df94f898b45",
    "github.com/rivo/uniseg@v0.4.7/eastasianwidth.go": "1cd2368ddedc56182d2f64576f2a713a2ad6234265d940575285f61006527c5b",
    "github.com/rivo/uniseg@v0.4.7/emojipresentation.go": "c4e1921aafedd787a29a0a6e329b85c6833eb1cfad0cfaf00450783f99e33b9c",
    "go1.27.2/src/unicode/tables.go": "ac0e5d5d1b58ed4b02d7cb4a0085bdefbdb4f7aae6fadddbc9477f8463da8bd8",
    "go1.27.2/src/vendor/golang.org/x/net/idna/tables17.0.0.go": "c1a0162f154cb63bab4de125817927c1d1d8a19326ad7c6adbb69d9d249b9cf3",
    "go1.27.2/src/vendor/golang.org/x/text/unicode/bidi/tables17.0.0.go": "a49eafd4eb11f7c8ae810ee5d69ca59a0cddd97dc644afbcb3e40563617088fc",
    "go1.27.2/src/vendor/golang.org/x/text/unicode/norm/tables17.0.0.go": "26784025eb1881a4452f101a7ef34b33c8ca02bd0603bfec41d2c823494ff7cb",
}


def digest(data):
    return hashlib.sha256(data).hexdigest()


def canonical(value):
    return (json.dumps(value, sort_keys=True, indent=2) + "\n").encode()


def require(condition, message):
    if not condition:
        raise AssertionError(message)


def inspect_source(root):
    """Independently compare actual files/normalized modes with FULL HEAD blobs.

    Git hash-object --no-filters hashes the actual files; neither status/index
    hints nor the producer's reported inventory/hash determine source truth.
    """
    def git(*args):
        return subprocess.check_output(["git", *args], cwd=root, env={"PATH": os.defpath,
            "HOME": "/nonexistent", "LC_ALL": "C", "GIT_CONFIG_NOSYSTEM": "1", "GIT_CONFIG_GLOBAL": os.devnull})
    flags = git("ls-files", "-v", "-z").decode().split("\0")[:-1]
    require(all(row[0] == row[0].upper() and row[0] != "S" for row in flags), "hidden index flags refused")
    for item in git("config", "--null", "--list").decode().split("\0"):
        key, _, value = item.partition("\n")
        require(not (key.lower() in {"core.sparsecheckout", "core.sparsecheckoutcone", "index.sparse"}
            and value.lower() in {"true", "yes", "on", "1", ""}), "sparse source refused")
    tracked = set(git("ls-files", "-z").decode().split("\0")[:-1])
    extra = sorted(git("ls-files", "--others", "--exclude-standard", "-z").decode().split("\0")[:-1])
    names = sorted(n for n in tracked | set(extra) if (root / n).exists() or (root / n).is_symlink())
    inventory = []
    for name in names:
        require(not Path(name).is_absolute() and not re.search(r"[\x00-\x1f\x7f]", name)
                and all(p not in {".", ".."} for p in Path(name).parts), "unsafe source name")
        require(not any(p in {".git", "dist", "bin", "data", "secrets", "__pycache__"}
                        or p.startswith((".env", ".atuin-ai-", ".record-")) for p in Path(name).parts), "private source path")
        require(Path(name).name not in {"credentials.json", "auth.json", "config.toml", "config.local.toml"}
                and not name.endswith((".token", ".log", ".test", ".prof", ".pyc")), "private source file")
        path = root / name
        require(not path.is_symlink() and stat.S_ISREG(path.lstat().st_mode) and path.stat().st_nlink == 1, "source type")
        require(not any(parent.is_symlink() for parent in path.parents if parent != root), "source ancestor symlink")
        data = path.read_bytes()
        inventory.append({"path": name, "mode": "0755" if path.stat().st_mode & 0o111 else "0644",
                          "size": len(data), "sha256": digest(data)})
    objects = git("hash-object", "--no-filters", "--", *names).decode().splitlines() if names else []
    actual_tree = [{"path": entry["path"], "mode": entry["mode"], "oid": oid} for entry, oid in zip(inventory, objects)]
    head = []
    for row in git("ls-tree", "--full-tree", "-r", "-z", "HEAD").split(b"\0")[:-1]:
        metadata, name = row.split(b"\t", 1)
        mode, kind, oid = metadata.decode().split()
        require(kind == "blob" and mode in {"100644", "100755"}, "unsupported HEAD type")
        head.append({"path": name.decode(), "mode": "0755" if mode == "100755" else "0644", "oid": oid})
    head.sort(key=lambda e: e["path"])
    status = git("status", "--porcelain=v1", "-z", "--untracked-files=all")
    diff = git("diff", "--no-ext-diff", "--no-textconv", "--binary", "HEAD", "--")
    identity = {"revision": git("rev-parse", "HEAD").decode().strip(),
                "git_tree": git("rev-parse", "HEAD^{tree}").decode().strip(),
                "inventory_sha256": digest(canonical(inventory)), "dirty": bool(status) or actual_tree != head,
                "head_snapshot_match": actual_tree == head, "head_inventory_sha256": digest(canonical(head)),
                "tracked_diff": bool(diff), "tracked_diff_sha256": digest(diff),
                "status_sha256": digest(status), "untracked": extra}
    return inventory, identity


def normalized_archive(files):
    """Consumer reconstruction enforces the whole normalized archive byte form."""
    buffer = io.BytesIO()
    with gzip.GzipFile(fileobj=buffer, mode="wb", filename="", mtime=0, compresslevel=9) as compressed:
        with tarfile.open(fileobj=compressed, mode="w", format=tarfile.USTAR_FORMAT) as tar:
            for name in sorted(files):
                data, mode = files[name]
                info = tarfile.TarInfo(name)
                info.size, info.mode, info.uid, info.gid, info.mtime = len(data), mode, 0, 0, 0
                info.uname = info.gname = ""
                tar.addfile(info, io.BytesIO(data))
    return buffer.getvalue()


def verify(directory, source_root=ROOT):
    require(directory.is_dir() and not directory.is_symlink(), "output directory type")
    require(stat.S_IMODE(directory.stat().st_mode) == 0o700, "output directory mode")
    def unique_object(pairs):
        result = {}
        for key, value in pairs:
            require(key not in result, "duplicate manifest member")
            result[key] = value
        return result
    manifest_stat = (directory / "manifest.json").lstat()
    require(stat.S_ISREG(manifest_stat.st_mode) and manifest_stat.st_nlink == 1, "manifest file type")
    require(manifest_stat.st_size <= 1048576, "manifest size")
    manifest = json.loads((directory / "manifest.json").read_text(), object_pairs_hook=unique_object)
    version = manifest["version"]
    require(isinstance(version, str) and len(version) <= 100 and re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9]+(?:[.-][A-Za-z0-9]+)*)?", version), "version")
    base = "atuin-ai-server-" + version
    binaries = {target: base + "-" + target.replace("/", "-") for target in TARGETS}
    expected_assets = {*binaries.values(), *(name + ".tar.gz" for name in binaries.values()), base + "-legal.txt", "install.sh"}
    expected_files = expected_assets | {"manifest.json", "SHA256SUMS"}
    require({path.name for path in directory.iterdir()} == expected_files, "exact output file set")
    blobs = {}
    for name in expected_files:
        path = directory / name
        require(stat.S_ISREG(path.lstat().st_mode) and not path.is_symlink() and path.stat().st_nlink == 1, "output file type: " + name)
        expected_mode = 0o755 if name in {*binaries.values(), "install.sh"} else 0o644
        require(stat.S_IMODE(path.stat().st_mode) == expected_mode, "output file mode: " + name)
        maximum = (67108864 if name in binaries.values() else 68157440 if name.endswith(".tar.gz")
                   else 65536 if name in {"install.sh", "SHA256SUMS"} else 1048576)
        require(path.stat().st_size <= maximum, "output file size bound: " + name)
        blobs[name] = path.read_bytes()
    rows = {}
    for line in blobs["SHA256SUMS"].decode("ascii").splitlines(keepends=True):
        match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9._-]+)\n", line)
        require(match is not None and match[2] not in rows, "checksum row format/duplicate")
        rows[match[2]] = match[1]
    require(set(rows) == expected_files - {"SHA256SUMS"}, "exact checksum row set")
    require(rows == {name: digest(data) for name, data in blobs.items() if name != "SHA256SUMS"}, "checksum bytes")
    require(manifest["assets"] == {name: {"size": len(blobs[name]), "sha256": digest(blobs[name])}
                                   for name in expected_assets}, "exact manifest assets/sizes/hashes")
    require(manifest["schema"] == 1 and manifest["toolchain"] == "go1.27.2", "schema/toolchain")
    actual_inventory, actual_source = inspect_source(source_root)
    require(manifest["source_inventory"] == actual_inventory and manifest["source"] == actual_source, "source inventory/identity misrepresentation")
    require(manifest["mode"] in {"DEVELOPMENT; RELEASE BLOCKED", "CLEAN TAGGED; publication requires separate approval"}, "mode")
    if manifest["mode"].startswith("CLEAN"):
        require(not actual_source["dirty"] and actual_source["head_snapshot_match"], "false clean source")
        result = subprocess.check_output(["git", "rev-parse", "--verify", "refs/tags/" + version + "^{commit}"], cwd=source_root).decode().strip()
        require(result == actual_source["revision"], "tag identity")
    reproduction = manifest["reproducibility"]
    paths = reproduction["paths"]
    require(reproduction["same_target_two_absolute_paths"] is True and reproduction["independent_attestation"] is False
            and len(paths) == 2 and len(set(paths)) == 2 and all(Path(path).is_absolute() for path in paths), "two-path evidence")
    require(len(manifest["targets"]) == 2 and {target["target"] for target in manifest["targets"]} == TARGETS, "exact minimum target set")
    expected_legal_hashes = {name: digest((source_root / name).read_bytes()) for name in SOURCE_LEGAL}
    expected_legal_hashes.update(GO_LEGAL)
    require(manifest["legal_inventory"] == expected_legal_hashes, "exact reviewed legal inventory")
    require({name: expected_legal_hashes["LICENSES/" + name] for name in UNICODE_LEGAL} == UNICODE_LEGAL, "reviewed Unicode grants")
    require(manifest["unicode_attribution"] == {"versions": {"uniseg": "15.0.0", "go": "17.0.0"},
            "table_sources_sha256": TABLES, "reviewed_legal_sha256": UNICODE_LEGAL}, "Unicode table/notice completeness")
    legal_sizes = {name: (source_root / name).stat().st_size for name in SOURCE_LEGAL}
    legal_sizes.update({name: 335056 if "source-notices" in name else 1303 if "PATENTS" in name else 1453 for name in GO_LEGAL})
    legal = None
    for target in manifest["targets"]:
        binary_name = binaries[target["target"]]
        archive_name = binary_name + ".tar.gz"
        binary = blobs[binary_name]
        require(4096 <= len(binary) <= 67108864, "binary size")
        if target["target"] == "linux/amd64":
            require(binary[:8] == bytes.fromhex("7f454c4602010100") and binary[16:20] == bytes.fromhex("02003e00"), "Linux binary format/CPU")
        else:
            require(binary[:8] == bytes.fromhex("cffaedfe0c000001") and binary[12:16] == bytes.fromhex("02000000"), "Darwin binary format/CPU")
        require(all(path.encode() not in binary for path in paths), "source build path leaked")
        require(target["binary"] == binary_name and target["archive"] == archive_name
                and target["binary_sha256"] == digest(binary) and target["archive_sha256"] == digest(blobs[archive_name]), "target asset identity")
        require(target["linked_modules"] == MODULES, "shipping module graph")
        flags = ["-mod=readonly", "-trimpath", "-buildvcs=false", "-buildmode=exe", "-ldflags",
                 f"-X main.version={version} -X main.revision={actual_source['revision']} -X main.sourceSHA256={actual_source['inventory_sha256']} -X main.sourceDirty={str(actual_source['dirty']).lower()}"]
        require(target["flags"] == flags, "target build flags")
        with tarfile.open(fileobj=io.BytesIO(blobs[archive_name]), mode="r|gz") as tar:
            files = {}
            archive_inventory = []
            for member in tar:
                require(member.name in {"atuin-ai-server", *expected_legal_hashes} and member.name not in files, "exact archive member set/order")
                require(member.size == (len(binary) if member.name == "atuin-ai-server" else legal_sizes[member.name]), "archive member size")
                mode = 0o755 if member.name == "atuin-ai-server" else 0o644
                require(member.type == tarfile.REGTYPE and not member.pax_headers
                        and (member.uid, member.gid, member.mtime, member.uname, member.gname, member.mode) == (0, 0, 0, "", "", mode), "archive member type/mode/metadata")
                data = tar.extractfile(member).read()
                require(len(data) == member.size, "archive member size")
                if member.name == "atuin-ai-server":
                    require(data == binary, "archive executable bytes")
                else:
                    require(digest(data) == expected_legal_hashes[member.name], "archive legal content")
                files[member.name] = (data, mode)
                archive_inventory.append({"path": member.name, "mode": oct(mode), "size": len(data), "sha256": digest(data)})
            require(list(files) == sorted({"atuin-ai-server", *expected_legal_hashes}), "exact archive member set/order")
            require(target["archive_inventory"] == archive_inventory, "manifest archive inventory")
        require(blobs[archive_name] == normalized_archive(files), "normalized archive bytes")
        target_legal = {name: data for name, (data, _) in files.items() if name != "atuin-ai-server"}
        require(legal is None or legal == target_legal, "same-source target legal bytes")
        legal = target_legal
    expected_text = b"".join(("===== " + name + " =====\n").encode() + data + b"\n" for name, data in sorted(legal.items()))
    require(0 < len(expected_text) <= 1048576 and blobs[base + "-legal.txt"] == expected_text, "installer legal content/completeness")
    template = (source_root / "scripts/install.sh").read_bytes()
    require(template.count(b"RELEASE_VERSION='UNPUBLISHED'") == 1, "source installer sentinel")
    expected_installer = template.replace(b"RELEASE_VERSION='UNPUBLISHED'", f"RELEASE_VERSION='{version}'".encode())
    require(blobs["install.sh"] == expected_installer, "pinned installer source bytes")
    return manifest
