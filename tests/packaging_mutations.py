"""Mandatory mutation regressions against freshly built real packaging assets.

Run by packaging_test.py --artifacts (required packaging CI step). No optional
skip: absent artifacts fail. Copies are owned temporary files; no original
artifact/source is modified and no mutation is executed as code.
"""
import gzip
import hashlib
import io
import json
from pathlib import Path
import shutil
import tarfile
import tempfile


def sha(data):
    return hashlib.sha256(data).hexdigest()


def canonical(value):
    return (json.dumps(value, sort_keys=True, indent=2) + "\n").encode()


def refresh(directory, manifest, assets=False, targets=False):
    if assets:
        manifest["assets"] = {name: {"size": (directory / name).stat().st_size,
                                     "sha256": sha((directory / name).read_bytes())}
                              for name in manifest["assets"]}
    if targets:
        for target in manifest["targets"]:
            target["binary_sha256"] = sha((directory / target["binary"]).read_bytes())
            target["archive_sha256"] = sha((directory / target["archive"]).read_bytes())
    (directory / "manifest.json").write_bytes(canonical(manifest))
    rows = [f"{sha(path.read_bytes())}  {path.name}\n" for path in sorted(directory.iterdir()) if path.name != "SHA256SUMS"]
    (directory / "SHA256SUMS").write_text("".join(rows))


def change_archive(directory, manifest, change, target_name="darwin/arm64"):
    target = next(t for t in manifest["targets"] if t["target"] == target_name)
    path = directory / target["archive"]
    with tarfile.open(path, "r:gz") as tar:
        records = [{"name": member.name, "data": tar.extractfile(member).read(),
                    "mode": member.mode, "type": member.type, "linkname": member.linkname} for member in tar]
    change(records)
    buffer = io.BytesIO()
    with gzip.GzipFile(fileobj=buffer, mode="wb", filename="", mtime=0, compresslevel=9) as gz:
        with tarfile.open(fileobj=gz, mode="w", format=tarfile.USTAR_FORMAT) as tar:
            for record in records:
                info = tarfile.TarInfo(record["name"])
                info.mode, info.type, info.linkname = record["mode"], record["type"], record["linkname"]
                info.size = len(record["data"])
                info.uid = info.gid = info.mtime = 0
                info.uname = info.gname = ""
                tar.addfile(info, io.BytesIO(record["data"]))
    path.write_bytes(buffer.getvalue())
    target["archive_inventory"] = [{"path": r["name"], "mode": oct(r["mode"]), "size": len(r["data"]),
                                     "sha256": sha(r["data"])} for r in records]
    refresh(directory, manifest, assets=True, targets=True)


def run(original, contract):
    original_manifest = json.loads((original / "manifest.json").read_text())
    darwin = next(t for t in original_manifest["targets"] if t["target"] == "darwin/arm64")["binary"]
    legal_name = "atuin-ai-server-" + original_manifest["version"] + "-legal.txt"
    unicode_name = "LICENSES/Unicode-15-data-license.txt"
    cases = []
    def case(name, reason):
        def register(function):
            cases.append((name, reason, function))
            return function
        return register

    @case("missing Darwin raw checksum row", "exact checksum row set")
    def missing_checksum(directory, manifest):
        path = directory / "SHA256SUMS"
        path.write_text("".join(line for line in path.read_text().splitlines(keepends=True) if not line.endswith("  " + darwin + "\n")))

    @case("duplicated checksum row", "checksum row format/duplicate")
    def duplicated_checksum(directory, manifest):
        path = directory / "SHA256SUMS"
        path.write_text(path.read_text() + path.read_text().splitlines(keepends=True)[0])

    @case("unexpected checksum row", "exact checksum row set")
    def extra_checksum(directory, manifest):
        path = directory / "SHA256SUMS"
        path.write_text(path.read_text() + "0" * 64 + "  unexpected\n")

    @case("truncated checksum row", "checksum row format/duplicate")
    def truncated_checksum(directory, manifest):
        path = directory / "SHA256SUMS"
        path.write_bytes(path.read_bytes()[:-1])

    @case("missing target output file", "exact output file set")
    def missing_output(directory, manifest):
        (directory / darwin).unlink()

    @case("unexpected output file", "exact output file set")
    def extra_output(directory, manifest):
        (directory / "unexpected").write_text("not allowed")

    @case("symlink target output", "output file type")
    def symlink_output(directory, manifest):
        (directory / darwin).unlink()
        (directory / darwin).symlink_to(original / darwin)

    @case("wrong raw output mode", "output file mode")
    def wrong_mode(directory, manifest):
        (directory / darwin).chmod(0o644)

    @case("changed binary without matching checksum", "checksum bytes")
    def changed_binary(directory, manifest):
        path = directory / darwin
        path.write_bytes(path.read_bytes() + b"changed")

    @case("missing manifest asset", "exact manifest assets/sizes/hashes")
    def missing_manifest_asset(directory, manifest):
        manifest["assets"].pop(darwin)
        refresh(directory, manifest)

    @case("wrong manifest asset size", "exact manifest assets/sizes/hashes")
    def wrong_asset_size(directory, manifest):
        manifest["assets"][darwin]["size"] += 1
        refresh(directory, manifest)

    @case("wrong manifest asset hash", "exact manifest assets/sizes/hashes")
    def wrong_asset_hash(directory, manifest):
        manifest["assets"][darwin]["sha256"] = "0" * 64
        refresh(directory, manifest)

    @case("recursive manifest asset schema", "exact manifest assets/sizes/hashes")
    def recursive_assets(directory, manifest):
        manifest["assets"]["manifest.json"] = {"size": 0, "sha256": "0" * 64}
        refresh(directory, manifest)

    @case("missing minimum target", "exact minimum target set")
    def missing_target(directory, manifest):
        manifest["targets"] = [t for t in manifest["targets"] if t["target"] != "darwin/arm64"]
        refresh(directory, manifest)

    @case("duplicated target", "exact minimum target set")
    def duplicate_target(directory, manifest):
        manifest["targets"][1] = manifest["targets"][0]
        refresh(directory, manifest)

    @case("missing Unicode legal inventory", "exact reviewed legal inventory")
    def missing_unicode(directory, manifest):
        manifest["legal_inventory"].pop(unicode_name)
        refresh(directory, manifest)

    @case("false self-described legal hash", "exact reviewed legal inventory")
    def false_legal_hash(directory, manifest):
        manifest["legal_inventory"][unicode_name] = "0" * 64
        refresh(directory, manifest)

    @case("incomplete Unicode table attribution", "Unicode table/notice completeness")
    def false_unicode_attribution(directory, manifest):
        manifest["unicode_attribution"]["table_sources_sha256"].pop(next(iter(manifest["unicode_attribution"]["table_sources_sha256"])))
        refresh(directory, manifest)

    @case("wrong archive legal bytes with producer-rehashed assets", "archive legal content")
    def archive_legal(directory, manifest):
        def mutate(records):
            record = next(r for r in records if r["name"] == unicode_name)
            record["data"] = b"X" + record["data"][1:]
        change_archive(directory, manifest, mutate)

    @case("self-consistent wrong legal bytes across BOTH archives/bundle/producer hashes", "exact reviewed legal inventory")
    def all_rehashed_legal(directory, manifest):
        original_grant = (contract.ROOT / unicode_name).read_bytes()
        changed_grant = b"X" + original_grant[1:]
        manifest["legal_inventory"][unicode_name] = sha(changed_grant)
        manifest["unicode_attribution"]["reviewed_legal_sha256"]["Unicode-15-data-license.txt"] = sha(changed_grant)
        def mutate(records):
            next(r for r in records if r["name"] == unicode_name)["data"] = changed_grant
        for target_name in ("darwin/arm64", "linux/amd64"):
            change_archive(directory, manifest, mutate, target_name)
        path = directory / legal_name
        data = path.read_bytes()
        assert data.count(original_grant) == 1
        path.write_bytes(data.replace(original_grant, changed_grant, 1))
        refresh(directory, manifest, assets=True, targets=True)

    @case("wrong archive member mode", "archive member type/mode/metadata")
    def archive_mode(directory, manifest):
        def mutate(records):
            next(r for r in records if r["name"] == "atuin-ai-server")["mode"] = 0o644
        change_archive(directory, manifest, mutate)

    @case("wrong archive member type", "archive member")
    def archive_type(directory, manifest):
        def mutate(records):
            record = next(r for r in records if r["name"] == unicode_name)
            record["type"], record["linkname"] = tarfile.SYMTYPE, "atuin-ai-server"
        change_archive(directory, manifest, mutate)

    @case("extra archive member", "exact archive member set/order")
    def archive_extra(directory, manifest):
        change_archive(directory, manifest, lambda records: records.append({"name": "unexpected", "data": b"extra", "mode": 0o644, "type": tarfile.REGTYPE, "linkname": ""}))

    @case("duplicate archive member", "exact archive member set/order")
    def archive_duplicate(directory, manifest):
        change_archive(directory, manifest, lambda records: records.append(records[0].copy()))

    @case("raw/archive executable disagreement with matching outer hashes", "archive member size|archive executable bytes")
    def archive_binary_mismatch(directory, manifest):
        path = directory / darwin
        data = path.read_bytes()
        path.write_bytes(data[:-1] + bytes([data[-1] ^ 1]))
        refresh(directory, manifest, assets=True, targets=True)

    @case("wrong manifest archive inventory", "manifest archive inventory")
    def archive_inventory(directory, manifest):
        manifest["targets"][0]["archive_inventory"][0]["mode"] = "0o600"
        refresh(directory, manifest)

    @case("truncated installer legal bundle with matching asset hash", "installer legal content/completeness")
    def legal_bundle(directory, manifest):
        path = directory / legal_name
        path.write_bytes(path.read_bytes()[:-10])
        refresh(directory, manifest, assets=True)

    @case("source inventory content misrepresentation", "source inventory/identity misrepresentation")
    def source_bytes(directory, manifest):
        manifest["source_inventory"][0]["sha256"] = "0" * 64
        manifest["source"]["inventory_sha256"] = sha(canonical(manifest["source_inventory"]))
        refresh(directory, manifest)

    @case("missing source inventory entry", "source inventory/identity misrepresentation")
    def source_missing(directory, manifest):
        manifest["source_inventory"].pop()
        manifest["source"]["inventory_sha256"] = sha(canonical(manifest["source_inventory"]))
        refresh(directory, manifest)

    @case("source inventory normalized mode misrepresentation", "source inventory/identity misrepresentation")
    def source_mode(directory, manifest):
        manifest["source_inventory"][0]["mode"] = "0755" if manifest["source_inventory"][0]["mode"] != "0755" else "0644"
        manifest["source"]["inventory_sha256"] = sha(canonical(manifest["source_inventory"]))
        refresh(directory, manifest)

    @case("false clean source identity", "source inventory/identity misrepresentation")
    def source_clean(directory, manifest):
        manifest["source"]["dirty"] = not manifest["source"]["dirty"]
        refresh(directory, manifest)

    @case("modified pinned installer with matching outer hash", "pinned installer source bytes")
    def changed_installer(directory, manifest):
        path = directory / "install.sh"
        path.write_bytes(path.read_bytes().replace(b"curl -q --proto", b"curl    --proto"))
        refresh(directory, manifest, assets=True)

    for name, reason, mutate in cases:
        with tempfile.TemporaryDirectory(prefix="packaging-mutation-") as tmp:
            directory = Path(tmp) / "assets"
            directory.mkdir(mode=0o700)
            for path in original.iterdir():
                shutil.copyfile(path, directory / path.name)
                (directory / path.name).chmod(path.stat().st_mode & 0o777)
            manifest = json.loads((directory / "manifest.json").read_text())
            mutate(directory, manifest)
            try:
                contract.verify(directory)
            except AssertionError as error:
                import re
                if not re.search(reason, str(error)):
                    raise AssertionError(f"mutation {name!r} failed for unexpected reason: {error}") from error
            else:
                raise AssertionError("mutation was not rejected: " + name)
            print("REJECTED mutation: " + name)
    print(f"PASS: {len(cases)} mandatory independent artifact-contract mutation regressions")
