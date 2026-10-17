#!/usr/bin/env python3
"""Offline installer/source/archive tests. --artifacts adds real shipping smoke.
No release network, live OAuth, Rust, Docker, oracle or private state access.
"""
import argparse
import hashlib
import http.client
import http.server
import importlib.util
import io
import json
import os
from pathlib import Path
import selectors
import shutil
import signal
import socket
import ssl
import stat
import sys
import subprocess
import tarfile
import tempfile
import threading
import unittest

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("packager", ROOT / "scripts/package.py")
pkg = importlib.util.module_from_spec(spec)
spec.loader.exec_module(pkg)
contract_spec = importlib.util.spec_from_file_location("packaging_contract", ROOT / "tests/packaging_contract.py")
contract = importlib.util.module_from_spec(contract_spec)
contract_spec.loader.exec_module(contract)
mutation_spec = importlib.util.spec_from_file_location("packaging_mutations", ROOT / "tests/packaging_mutations.py")
mutations = importlib.util.module_from_spec(mutation_spec)
mutation_spec.loader.exec_module(mutations)
TAG = "v0.0.0-test.1"


def executable(path, text):
    path.write_text(text)
    path.chmod(0o755)


def digest(data):
    return hashlib.sha256(data).hexdigest()


class Installer(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="installer-test-")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.home = self.root / "home"
        self.home.mkdir(mode=0o700)
        self.tools = self.root / "tools"
        self.tools.mkdir()
        self.downloads = self.root / "assets"
        self.downloads.mkdir()
        self.scratch = self.root / "scratch"
        self.scratch.mkdir(mode=0o700)
        self.dest = self.home / "bin with spaces"
        self.dest.mkdir(mode=0o750)
        self.old = self.dest / "atuin-ai-server"
        self.old.write_bytes(b"old executable: must survive all precommit failures")
        self.old.chmod(0o711)
        # Sentinels prove installer never edits config, credentials or profiles.
        for name in (".profile", ".zshrc", ".bashrc", ".config/atuin-ai/config.toml",
                     ".config/atuin-ai/credentials/credentials.json", "Library/Application Support/atuin-ai/config.toml"):
            path = self.home / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("private synthetic sentinel")
        self.before = self.private_inventory()
        executable(self.tools / "uname", "#!/bin/sh\ncase $1 in -s) printf '%s\\n' \"${TEST_OS:-Linux}\";; -m) printf '%s\\n' \"${TEST_ARCH:-x86_64}\";; *) exit 1;; esac\n")
        executable(self.tools / "curl", '''#!/usr/bin/python3
import json, os, pathlib, shutil, sys
args = sys.argv[1:]
assert args and args[0] == "-q", "curl must disable .curlrc with FIRST argument -q"
if args == ["-q", "--version"]:
    print("curl " + os.environ.get("TEST_CURL_VERSION", "8.4.0") + " (fixture)")
    sys.exit(0)
required = {"--proto": "=https", "--proto-redir": "=https", "--max-redirs": "3", "--tlsv1.2": None,
            "--connect-timeout": "15", "--max-time": "120", "--fail": None, "--location": None,
            "--silent": None, "--show-error": None, "--max-filesize": None}
for flag, value in required.items():
    assert flag in args, (flag, args)
    if value is not None: assert args[args.index(flag)+1] == value
assert not set(args).intersection({"-k", "--insecure"})
url = args[-1]
assert url.startswith("https://github.com/Jghh42/atuin-ai/releases/download/")
assert "/latest/" not in url
with open(os.environ["TEST_LOG"], "a") as f: f.write(json.dumps(args)+"\\n")
if os.environ.get("TEST_CURL_FAIL") or os.environ.get("TEST_UNSAFE_REDIRECT"):
    sys.exit(22 if os.environ.get("TEST_CURL_FAIL") else 1)
name = url.rsplit("/",1)[1]
# Every request must use the SAME explicit tag even if latest changed after bootstrap.
assert url.split("/")[-2] == os.environ["TEST_TAG"]
source = pathlib.Path(os.environ["TEST_ASSETS"]) / name
if not source.is_file(): sys.exit(22)
output = pathlib.Path(args[args.index("--output")+1])
assert output.parent.stat().st_mode & 0o777 == 0o700
shutil.copyfile(source, output)
''')
        self.env = {"PATH": str(self.tools) + os.pathsep + os.defpath, "HOME": str(self.home),
                    "TMPDIR": str(self.scratch), "TEST_ASSETS": str(self.downloads),
                    "TEST_LOG": str(self.root / "curl.jsonl"), "TEST_TAG": TAG}
        header = bytes.fromhex("7f454c4602010100000000000000000002003e00")
        self.binary = header + b"\0" * 5000
        self.binary_name = f"atuin-ai-server-{TAG}-linux-amd64"
        self.legal_name = f"atuin-ai-server-{TAG}-legal.txt"
        self.asset(self.binary_name, self.binary)
        self.asset(self.legal_name, b"Synthetic licenses\n")
        self.checksums()

    def private_inventory(self):
        return {str(p.relative_to(self.home)): (p.read_bytes(), stat.S_IMODE(p.stat().st_mode))
                for p in self.home.rglob("*") if p.is_file() and not p.is_relative_to(self.dest)}

    def asset(self, name, data):
        (self.downloads / name).write_bytes(data)

    def checksums(self):
        data = "".join(f"{digest(p.read_bytes())}  {p.name}\n" for p in sorted(self.downloads.iterdir()) if p.name != "SHA256SUMS")
        self.asset("SHA256SUMS", data.encode())

    def invoke(self, *args, ok=False, script=None):
        old_bytes = self.old.read_bytes() if not self.old.is_symlink() else None
        old_mode = self.old.stat().st_mode if old_bytes else None
        command = ["/bin/sh", str(script or ROOT / "scripts/install.sh"),
                   "--version", self.env["TEST_TAG"], "--bin-dir", str(self.dest), *args]
        child = subprocess.Popen(command, env=self.env, stdout=subprocess.PIPE,
                                 stderr=subprocess.PIPE, start_new_session=True)
        try:
            stdout, stderr = child.communicate(timeout=10)
        except subprocess.TimeoutExpired:
            os.killpg(child.pid, signal.SIGKILL)
            child.communicate()
            raise
        result = subprocess.CompletedProcess(command, child.returncode, stdout, stderr)
        if ok:
            self.assertEqual(result.returncode, 0, result.stderr.decode())
        else:
            self.assertNotEqual(result.returncode, 0, result.stdout.decode())
            if old_bytes is not None:
                self.assertEqual(self.old.read_bytes(), old_bytes, result.stderr.decode())
                self.assertEqual(self.old.stat().st_mode, old_mode)
        self.assertEqual(list(self.scratch.iterdir()), [], result.stderr.decode())
        self.assertEqual(list(self.dest.glob(".atuin-ai-*")), [], result.stderr.decode())
        self.assertEqual(self.private_inventory(), self.before)
        self.assertEqual(stat.S_IMODE(self.dest.stat().st_mode), 0o750)
        return result

    def test_install_spaces_modes_and_pinned_urls(self):
        self.invoke(ok=True)
        self.assertEqual(self.old.read_bytes(), self.binary)
        self.assertEqual(stat.S_IMODE(self.old.stat().st_mode), 0o755)
        legal = self.dest / f"atuin-ai-server.licenses-{TAG}.txt"
        self.assertEqual(legal.read_bytes(), b"Synthetic licenses\n")
        self.assertEqual(stat.S_IMODE(legal.stat().st_mode), 0o644)
        urls = [json.loads(line)[-1] for line in (self.root / "curl.jsonl").read_text().splitlines()]
        self.assertEqual(len(urls), 3)
        self.assertTrue(all(f"/download/{TAG}/" in u for u in urls))

    def test_generated_bootstrap_pins_once_no_latest_race(self):
        script = self.root / "bootstrap.sh"
        script.write_text((ROOT / "scripts/install.sh").read_text().replace("RELEASE_VERSION='UNPUBLISHED'", f"RELEASE_VERSION='{TAG}'"))
        result = subprocess.run(["sh", str(script), "--bin-dir", str(self.dest)], env=self.env, capture_output=True, timeout=10)
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        self.assertEqual(self.old.read_bytes(), self.binary)
        self.assertEqual(len((self.root / "curl.jsonl").read_text().splitlines()), 3)
        self.assertEqual(self.private_inventory(), self.before)

    def test_template_unavailable_without_version(self):
        result = subprocess.run(["sh", str(ROOT / "scripts/install.sh")], env=self.env, capture_output=True, timeout=10)
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "curl.jsonl").exists())

    def test_corrupt_binary(self):
        self.asset(self.binary_name, b"corrupt")
        self.assertIn(b"checksum mismatch", self.invoke().stderr)

    def test_corrupt_legal(self):
        self.asset(self.legal_name, b"corrupt legal")
        self.assertIn(b"legal checksum mismatch", self.invoke().stderr)

    def test_truncated_but_checksummed(self):
        self.asset(self.binary_name, self.binary[:20])
        self.checksums()
        self.assertIn(b"truncated binary", self.invoke().stderr)

    def test_wrong_arch_checksummed(self):
        self.asset(self.binary_name, self.binary[:18] + bytes.fromhex("b700") + self.binary[20:])
        self.checksums()
        self.assertIn(b"architecture mismatch", self.invoke().stderr)

    def test_malformed_missing_duplicate_checksums(self):
        original = (self.downloads / "SHA256SUMS").read_bytes()
        for data in (b"bad checksum", b"", original + original, original.upper(),
                     original + b"0" * 64 + b"  ../../danger\n"):
            with self.subTest(data=data[:30]):
                self.asset("SHA256SUMS", data)
                self.invoke()

    def test_platforms_and_arch_aliases(self):
        for osname, arch in (("Windows_NT", "x86_64"), ("Linux", "aarch64"), ("Darwin", "x86_64"), ("Linux", "i686")):
            with self.subTest(os=osname, arch=arch):
                self.env.update(TEST_OS=osname, TEST_ARCH=arch)
                self.invoke()
        self.assertFalse((self.root / "curl.jsonl").exists())
        self.env.update(TEST_OS="Linux", TEST_ARCH="amd64")
        self.invoke(ok=True)

    def test_darwin_arm64_aliases(self):
        self.binary_name = f"atuin-ai-server-{TAG}-darwin-arm64"
        self.binary = bytes.fromhex("cffaedfe0c000001000000000200000000000000") + b"\0" * 5000
        self.asset(self.binary_name, self.binary)
        self.checksums()
        for arch in ("arm64", "aarch64"):
            with self.subTest(arch=arch):
                self.env.update(TEST_OS="Darwin", TEST_ARCH=arch)
                self.invoke(ok=True)

    def test_curl_and_unsafe_redirect_fail_closed(self):
        for variable in ("TEST_CURL_FAIL", "TEST_UNSAFE_REDIRECT"):
            with self.subTest(variable=variable):
                self.env[variable] = "1"
                self.assertIn(b"HTTPS download failed", self.invoke().stderr)
                del self.env[variable]

    def test_oversize_download_even_with_lying_curl(self):
        self.asset("SHA256SUMS", b"x" * 65537)
        self.assertIn(b"size exceeded", self.invoke().stderr)

    def test_target_symlink(self):
        other = self.root / "untouched"
        other.write_bytes(b"not touched")
        self.old.unlink()
        self.old.symlink_to(other)
        self.assertIn(b"symlink target", self.invoke().stderr)
        self.assertEqual(other.read_bytes(), b"not touched")
        self.assertTrue(self.old.is_symlink())

    def test_ancestor_symlink(self):
        link = self.root / "link"
        link.symlink_to(self.dest, target_is_directory=True)
        result = self.invoke("--bin-dir", str(link))
        self.assertIn(b"symlink directory", result.stderr)

    def test_legal_symlink(self):
        (self.dest / f"atuin-ai-server.licenses-{TAG}.txt").symlink_to(self.old)
        self.assertIn(b"symlink target", self.invoke().stderr)

    def test_unsafe_versions_paths(self):
        for value in ("latest", "v1.2.3/../../x", "v1.2.3\n", "-v1.2.3", "v1.2.3+a"):
            with self.subTest(value=value):
                self.invoke("--version", value)
        for value in ("relative", str(self.dest) + "/../x", str(self.dest) + "\n", str(self.dest) + "/"):
            with self.subTest(value=value):
                self.invoke("--bin-dir", value)

    def test_new_directories_private_existing_unchanged(self):
        new = self.root / "new local" / "bin"
        self.invoke("--bin-dir", str(new), ok=True)
        self.assertEqual(stat.S_IMODE(new.stat().st_mode), 0o700)
        self.assertEqual(stat.S_IMODE(new.parent.stat().st_mode), 0o700)

    def failing_tool(self, name, action, marker=""):
        real = shutil.which(name, path=os.defpath)
        executable(self.tools / name, f'''#!/usr/bin/python3
import os, signal, sys
if {marker!r} in " ".join(sys.argv[1:]):
    {action}
os.execv({real!r}, [{real!r}] + sys.argv[1:])
''')

    def test_staging_failure_and_interrupt_cleanup(self):
        self.failing_tool("mktemp", "sys.exit(1)", ".atuin-ai-binary")
        self.assertIn(b"cannot stage binary", self.invoke().stderr)
        self.failing_tool("mktemp", "sys.exit(1)", ".atuin-ai-legal")
        self.assertIn(b"cannot stage legal", self.invoke().stderr)
        (self.tools / "mktemp").unlink()
        # Child reports SUCCESS: only the shell's signal trap can prevent commit.
        for caught in ("SIGTERM", "SIGHUP", "SIGINT"):
            with self.subTest(signal=caught):
                self.failing_tool("chmod", f"os.kill(os.getppid(), signal.{caught}); sys.exit(0)", ".atuin-ai-binary")
                self.invoke()

    def test_copy_chmod_legalrename_binaryrename_failure(self):
        for tool, marker in (("cat", ".txt"), ("chmod", ".atuin-ai-binary"),
                             ("mv", ".atuin-ai-legal"), ("mv", ".atuin-ai-binary")):
            with self.subTest(tool=tool, marker=marker):
                self.failing_tool(tool, "sys.exit(1)", marker)
                self.invoke()
                (self.tools / tool).unlink()

    def test_interrupted_download_cleanup(self):
        self.failing_tool("curl", "os.kill(os.getppid(), signal.SIGTERM); sys.exit(1)", "--output")
        self.invoke()

    def test_missing_legal_fails_before_binary_download(self):
        (self.downloads / self.legal_name).unlink()
        self.checksums()
        self.assertIn(b"legal checksum", self.invoke().stderr)
        self.assertEqual(len((self.root / "curl.jsonl").read_text().splitlines()), 1)

    def test_nonregular_target_refused(self):
        alternate = self.root / "alternate"
        alternate.mkdir()
        (alternate / "atuin-ai-server").mkdir()
        self.assertIn(b"nonregular target", self.invoke("--bin-dir", str(alternate)).stderr)

    def test_shasum_fallback(self):
        # A macOS-like PATH: no sha256sum, but normal shasum -a 256.
        for name in ("grep", "mkdir", "mktemp", "od", "awk", "wc", "tr", "chmod", "mv", "cat", "rm", "shasum"):
            (self.tools / name).symlink_to(shutil.which(name, path=os.defpath))
        self.env["PATH"] = str(self.tools)
        self.invoke(ok=True)

    def test_old_or_unparseable_curl_fails_before_fetch(self):
        for version in ("8.3.0", "7.88.1", "unknown", "8.4", "8.4.x", "8.4.0-DEV"):
            with self.subTest(version=version):
                self.env["TEST_CURL_VERSION"] = version
                self.assertIn(b"curl 8.4.0 or newer required", self.invoke().stderr)
                self.assertFalse((self.root / "curl.jsonl").exists())
        for version in ("8.4.0", "8.18.0", "9.0.0"):
            with self.subTest(version=version):
                self.env["TEST_CURL_VERSION"] = version
                self.invoke(ok=True)

    def test_darwin_checksummed_wrong_format_and_cpu(self):
        self.env.update(TEST_OS="Darwin", TEST_ARCH="arm64")
        self.binary_name = f"atuin-ai-server-{TAG}-darwin-arm64"
        for data in (self.binary, bytes.fromhex("cffaedfe07000001000000000200000000000000") + b"\\0" * 5000,
                     bytes.fromhex("cefaedfe0c000000000000000200000000000000") + b"\\0" * 5000):
            with self.subTest(header=data[:8].hex()):
                self.asset(self.binary_name, data)
                self.checksums()
                self.assertIn(b"format/architecture mismatch", self.invoke().stderr)

    def test_version_override_replaces_embedded_pin(self):
        script = self.root / "bootstrap.sh"
        script.write_text((ROOT / "scripts/install.sh").read_text().replace("RELEASE_VERSION='UNPUBLISHED'", "RELEASE_VERSION='v99.0.0'"))
        self.invoke(ok=True, script=script)


class RealCurlTLS(unittest.TestCase):
    """Actual curl, isolated fake HOME/CA and forced loopback HTTPS only.

    No GitHub/public network: --connect-to maps BOTH possible schemes to the
    owned TLS server. The installer flags, not a stub response, must enforce TLS,
    HTTPS-only redirects and curl's in-flight chunked transfer/disk limits.
    """
    @classmethod
    def setUpClass(cls):
        cls.real_curl = shutil.which("curl", path=os.defpath)
        if not cls.real_curl or not shutil.which("openssl", path=os.defpath):
            raise AssertionError("real curl >=8.4 and openssl required; no green skip")
        cls.cert_tmp = tempfile.TemporaryDirectory(prefix="installer-tls-ca-")
        cls.cert_root = Path(cls.cert_tmp.name)
        cls.addClassCleanup(cls.cert_tmp.cleanup)
        for name in ("trusted", "unrelated"):
            subprocess.run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes",
                            "-config", os.devnull, "-keyout", str(cls.cert_root / (name + ".key")),
                            "-out", str(cls.cert_root / (name + ".crt")), "-days", "1",
                            "-subj", "/CN=github.com", "-addext", "subjectAltName=DNS:github.com",
                            "-addext", "basicConstraints=critical,CA:TRUE"],
                           env={"PATH": os.defpath, "HOME": str(cls.cert_root)},
                           capture_output=True, check=True, timeout=10)

    def setUp(self):
        self.fixture = Installer("test_install_spaces_modes_and_pinned_urls")
        self.fixture.setUp()
        self.addCleanup(self.fixture.doCleanups)
        self.behavior = "chunked"
        self.oversize = None
        self.requests = []
        self.expect_certificate_rejection = False
        self.expected_peer_aborts = []
        self.server_errors = []
        self.limits = {"SHA256SUMS": 65536, self.fixture.binary_name: 67108864,
                       self.fixture.legal_name: 1048576}
        owner = self
        class Handler(http.server.BaseHTTPRequestHandler):
            protocol_version = "HTTP/1.1"
            def log_message(self, *args):
                pass
            def handle(self):
                try:
                    super().handle()
                except (BrokenPipeError, ConnectionResetError, ssl.SSLError) as error:
                    if not owner.expect_certificate_rejection:
                        raise
                    owner.expected_peer_aborts.append(type(error).__name__)
            def do_GET(self):
                self.close_connection = True
                owner.requests.append((self.path, self.headers.get("Authorization")))
                name = self.path.rsplit("/", 1)[-1]
                if owner.behavior in {"redirect-https", "redirect-http"} and "/redirected/" not in self.path:
                    scheme = "https" if owner.behavior == "redirect-https" else "http"
                    self.send_response(302)
                    self.send_header("Location", scheme + "://github.com/redirected/" + name)
                    self.send_header("Content-Length", "0")
                    self.send_header("Connection", "close")
                    self.end_headers()
                    return
                data = (owner.fixture.downloads / name).read_bytes() if (owner.fixture.downloads / name).is_file() else b"control"
                self.send_response(200)
                self.send_header("Transfer-Encoding", "chunked")
                self.send_header("Connection", "close")
                self.end_headers()
                try:
                    # Every response lacks Content-Length, including ALL THREE
                    # oversize assets. Never allocate a 64MiB fixture in memory.
                    if name == owner.oversize:
                        remaining = owner.limits[name] + 65536
                        while remaining:
                            chunk = b"x" * min(65536, remaining)
                            self.wfile.write(f"{len(chunk):x}\r\n".encode() + chunk + b"\r\n")
                            self.wfile.flush()
                            remaining -= len(chunk)
                    else:
                        self.wfile.write(f"{len(data):x}\r\n".encode() + data + b"\r\n")
                    self.wfile.write(b"0\r\n\r\n")
                    self.wfile.flush()
                except (BrokenPipeError, ConnectionResetError, ssl.SSLError):
                    if name != owner.oversize:
                        raise
                    owner.expected_peer_aborts.append("bounded transfer stopped")
        class FixtureServer(http.server.ThreadingHTTPServer):
            def handle_error(self, request, client_address):
                owner.server_errors.append(repr(sys.exc_info()[1]))
        self.server = FixtureServer(("127.0.0.1", 0), Handler)
        self.server.daemon_threads = True
        context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
        context.load_cert_chain(self.cert_root / "trusted.crt", self.cert_root / "trusted.key")
        self.server.socket = context.wrap_socket(self.server.socket, server_side=True)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.addCleanup(self.close_server)
        port = self.server.server_address[1]
        self.mapping = ["--connect-to", f"github.com:443:127.0.0.1:{port}", "--connect-to",
                        f"github.com:80:127.0.0.1:{port}", "--noproxy", "*"]
        self.fixture.env.update(TEST_REAL_CURL=self.real_curl, TEST_CONNECT=json.dumps(self.mapping),
                                TEST_CA=str(self.cert_root / "trusted.crt"))
        executable(self.fixture.tools / "curl", '''#!/usr/bin/python3
import json, os, pathlib, subprocess, sys
args = sys.argv[1:]
assert args[0] == "-q", "-q must be first, even for the version probe"
real = os.environ["TEST_REAL_CURL"]
if args == ["-q", "--version"]:
    result = subprocess.run([real, *args], capture_output=True)
    sys.stdout.buffer.write(result.stdout); sys.stderr.buffer.write(result.stderr)
    sys.exit(result.returncode)
assert args[-1].startswith("https://github.com/Jghh42/atuin-ai/releases/download/")
extra = json.loads(os.environ["TEST_CONNECT"]) + ["--cacert", os.environ["TEST_CA"]]
result = subprocess.run([real, "-q", *extra, *args[1:]], capture_output=True)
output = pathlib.Path(args[args.index("--output")+1])
record = {"args": args, "code": result.returncode, "size": output.stat().st_size if output.exists() else 0,
          "stderr": result.stderr.decode(errors="replace")}
with open(os.environ["TEST_LOG"], "a") as f: f.write(json.dumps(record)+"\\n")
sys.stdout.buffer.write(result.stdout); sys.stderr.buffer.write(result.stderr)
sys.exit(result.returncode)
''')

    def close_server(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(5)
        self.assertFalse(self.thread.is_alive(), "owned TLS fixture did not stop")
        self.assertEqual(self.server_errors, [], "unhandled real-TLS fixture failure")

    def receipts(self):
        return [json.loads(line) for line in (self.fixture.root / "curl.jsonl").read_text().splitlines()]

    def test_real_curlrc_contamination_ignored(self):
        home = self.fixture.home
        control = home / "curlrc-output"
        netrc = home / "synthetic.netrc"
        netrc.write_text("machine github.com login netrc-fixture password netrc-fixture-password\n")
        netrc.chmod(0o600)
        (home / ".curlrc").write_text(f'insecure\nuser = "synthetic-user:synthetic-password"\nnetrc\nnetrc-file = "{netrc}"\noutput = "{control}"\n')
        # Controlled negative witness: without -q this exact synthetic rc really
        # disables trust, sends fixture auth and writes outside curl's scratch.
        result = subprocess.run([self.real_curl, *self.mapping, "--silent", "--show-error",
                                 "--max-time", "5", "https://github.com/control"],
                                env=self.fixture.env, capture_output=True, timeout=10)
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        self.assertEqual(control.read_bytes(), b"control")
        self.assertIsNotNone(self.requests[-1][1])
        self.requests.clear()
        self.fixture.before = self.fixture.private_inventory()
        self.fixture.invoke(ok=True)
        self.assertEqual(len(self.receipts()), 3)
        self.assertTrue(all(auth is None for _, auth in self.requests))
        self.assertTrue(all(row["code"] == 0 for row in self.receipts()))

    def test_real_tls_does_not_honor_insecure_curlrc(self):
        (self.fixture.home / ".curlrc").write_text("insecure\n")
        self.fixture.before = self.fixture.private_inventory()
        self.expect_certificate_rejection = True
        self.fixture.env["TEST_CA"] = str(self.cert_root / "unrelated.crt")
        self.fixture.invoke()
        self.assertEqual(self.receipts()[0]["code"], 60)
        self.assertEqual(self.requests, [])

    def test_real_https_redirect_and_chunked_success(self):
        self.behavior = "redirect-https"
        self.fixture.invoke(ok=True)
        self.assertEqual(len(self.requests), 6)
        self.assertTrue(all(row["code"] == 0 for row in self.receipts()))

    def test_real_http_redirect_refused(self):
        self.behavior = "redirect-http"
        self.fixture.invoke()
        self.assertEqual(len(self.requests), 1)
        self.assertEqual(self.receipts()[0]["code"], 1)
        self.assertIn("disabled", self.receipts()[0]["stderr"])

    def test_real_unknown_length_limits_all_three_assets(self):
        for name, limit in self.limits.items():
            with self.subTest(asset=name, limit=limit):
                self.oversize = name
                log = self.fixture.root / "curl.jsonl"
                log.unlink(missing_ok=True)
                self.fixture.invoke()
                record = self.receipts()[-1]
                self.assertEqual(record["args"][-1].rsplit("/", 1)[-1], name)
                self.assertEqual(int(record["args"][record["args"].index("--max-filesize") + 1]), limit)
                self.assertEqual(record["code"], 63, record)
                self.assertLessEqual(record["size"], limit, "in-flight disk bound was not enforced")


class Packaging(unittest.TestCase):
    def test_independent_consumer_source_inspection(self):
        self.assertEqual(contract.inspect_source(ROOT), pkg.source())

    def test_version_and_archive_normalization(self):
        files = {"atuin-ai-server": (b"binary", 0o755), "LICENSE": (b"legal", 0o644)}
        data = pkg.archive(files)
        self.assertEqual(data, pkg.archive(dict(reversed(list(files.items())))))
        self.assertEqual(data[4:8], b"\0" * 4)
        self.assertEqual(data[3], 0)  # gzip no filename/comment
        with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as tar:
            self.assertEqual(tar.getnames(), sorted(files))
            for member in tar:
                self.assertTrue(member.isfile())
                self.assertEqual((member.uid, member.gid, member.mtime, member.uname, member.gname), (0, 0, 0, "", ""))
                self.assertEqual(member.mode, files[member.name][1])

    def test_complete_snapshot_and_private_exclusion(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            subprocess.run(["git", "init", "-q", str(root)], check=True)
            (root / ".gitignore").write_text("secrets/\nignored-binary\ncredentials.json\n")
            (root / "source.go").write_text("package sample\n")
            subprocess.run(["git", "-C", str(root), "add", ".gitignore", "source.go"], check=True)
            subprocess.run(["git", "-C", str(root), "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "fixture"], check=True)
            (root / "extra.go").write_text("package sample\n")
            (root / "secrets").mkdir()
            (root / "secrets/private").write_text("NEVER SHIP SYNTHETIC")
            (root / "credentials.json").write_text("NEVER SHIP SYNTHETIC")
            (root / "ignored-binary").write_bytes(b"NEVER SHIP SYNTHETIC")
            inv, identity = pkg.source(root)
            self.assertEqual({e["path"] for e in inv}, {".gitignore", "source.go", "extra.go"})
            self.assertTrue(identity["dirty"])
            self.assertFalse(identity["tracked_diff"])
            self.assertEqual(identity["untracked"], ["extra.go"])
            snap = root.parent / (root.name + "-snapshot")
            self.addCleanup(lambda: shutil.rmtree(snap, ignore_errors=True))
            pkg.snapshot(root, snap, inv)
            (snap / "extra-directory").mkdir()
            with self.assertRaisesRegex(RuntimeError, "complete inventory"):
                pkg.verify_snapshot(snap, inv)
            (snap / "extra-directory").rmdir()
            for mode in (0o755, 0o600):
                (snap / "source.go").chmod(mode)
                with self.assertRaisesRegex(RuntimeError, "complete inventory"):
                    pkg.verify_snapshot(snap, inv)
            (snap / "source.go").chmod(0o644)
            snap.chmod(0o755)
            with self.assertRaisesRegex(RuntimeError, "complete inventory"):
                pkg.verify_snapshot(snap, inv)
            snap.chmod(0o700)
            (snap / "unexpected.go").write_text("package injected")
            with self.assertRaisesRegex(RuntimeError, "complete inventory"):
                pkg.verify_snapshot(snap, inv)
            (root / "auth.json").write_text("private-looking unignored")
            with self.assertRaisesRegex(RuntimeError, "private"):
                pkg.source(root)
            (root / "auth.json").unlink()
            (root / "link.go").symlink_to(root / "source.go")
            with self.assertRaisesRegex(RuntimeError, "symlink"):
                pkg.source(root)

    def test_hidden_index_flags_missing_files_and_sparse_checkout(self):
        with tempfile.TemporaryDirectory(prefix="index-fixture-") as tmp:
            root = Path(tmp)
            def git(*args):
                return subprocess.check_output(["git", "-C", str(root), *args])
            git("init", "-q")
            for folder in ("keep", "omit"):
                (root / folder).mkdir()
                (root / folder / "source.go").write_text("package sample\\n")
            git("add", ".")
            git("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "fixture")
            self.assertFalse(pkg.source(root)[1]["dirty"])
            self.assertTrue(pkg.source(root)[1]["head_snapshot_match"])
            path = root / "keep/source.go"
            for flag, clear in (("--assume-unchanged", "--no-assume-unchanged"),
                                ("--skip-worktree", "--no-skip-worktree")):
                git("update-index", flag, "keep/source.go")
                path.write_text("package hidden_byte_change\\n")
                self.assertEqual(git("status", "--porcelain"), b"")
                with self.assertRaisesRegex(RuntimeError, "index flags"):
                    pkg.source(root)
                with self.assertRaisesRegex(AssertionError, "hidden index flags"):
                    contract.inspect_source(root)
                git("update-index", clear, "keep/source.go")
                identity = pkg.source(root)[1]
                self.assertEqual(contract.inspect_source(root)[1], identity)
                self.assertTrue(identity["dirty"])
                self.assertFalse(identity["head_snapshot_match"])
                path.write_text("package sample\\n")
            # core.filemode=false can hide executable-mode changes from status.
            git("config", "core.filemode", "false")
            path.chmod(0o755)
            self.assertEqual(git("status", "--porcelain"), b"")
            self.assertTrue(pkg.source(root)[1]["dirty"])
            self.assertFalse(pkg.source(root)[1]["head_snapshot_match"])
            self.assertEqual(contract.inspect_source(root), pkg.source(root))
            path.chmod(0o644)
            path.unlink()
            missing = pkg.source(root)[1]
            self.assertTrue(missing["dirty"])
            self.assertFalse(missing["head_snapshot_match"])
            self.assertEqual(contract.inspect_source(root)[1], missing)
            git("restore", "keep/source.go")
            git("sparse-checkout", "init", "--cone")
            git("sparse-checkout", "set", "keep")
            self.assertFalse((root / "omit/source.go").exists())
            with self.assertRaisesRegex(RuntimeError, "index flags|sparse"):
                pkg.source(root)
            with self.assertRaisesRegex(AssertionError, "hidden index flags|sparse"):
                contract.inspect_source(root)
            git("sparse-checkout", "disable")
            self.assertFalse(pkg.source(root)[1]["dirty"])

    def test_release_mode_rejects_dirty_and_unsafe_version(self):
        # Test actual preconditions without compilation or publication.
        from unittest.mock import patch
        with patch.object(pkg, "source", return_value=([], {"dirty": True})):
            with self.assertRaisesRegex(RuntimeError, "CLEAN"):
                pkg.package(argparse.Namespace(version="v1.2.3", development=False, out="unused"))
        with self.assertRaisesRegex(RuntimeError, "unsafe version"):
            pkg.package(argparse.Namespace(version="v1.2.3/evil", development=True, out="unused"))
        with patch.object(pkg, "source", return_value=([], {"dirty": False, "revision": "head"})), patch.object(pkg, "git", return_value=b"other-commit\n"):
            with self.assertRaisesRegex(RuntimeError, "exact HEAD"):
                pkg.package(argparse.Namespace(version="v1.2.3", development=False, out="unused"))


def shipping_smoke(directory):
    manifest = contract.verify(directory)
    metadata_env = {"PATH": os.defpath, "HOME": "/nonexistent", "GOENV": "off", "GOWORK": "off",
                    "GOTOOLCHAIN": "go1.27.2", "GOPATH": str(Path.home() / "go"),
                    "GOMODCACHE": str(Path.home() / "go/pkg/mod")}
    go = shutil.which("go")
    for target in manifest["targets"]:
        binary = directory / target["binary"]
        metadata = subprocess.check_output([go, "version", "-m", str(binary)], env=metadata_env).decode()
        assert metadata.replace(str(binary), "atuin-ai-server", 1) == target["go_version_m"]
        assert "CGO_ENABLED=0" in metadata
        if target["target"] == "linux/amd64":
            assert "GOAMD64=v1" in metadata
    mutations.run(directory, contract)
    linux = next(t for t in manifest["targets"] if t["target"] == "linux/amd64")
    with tempfile.TemporaryDirectory(prefix="shipping-relocated-") as tmp:
        root = Path(tmp)
        home, config = root / "home", root / "config"
        home.mkdir(mode=0o700)
        config.mkdir(mode=0o700)
        binary = root / "relocated executable"
        shutil.copyfile(directory / linux["binary"], binary)
        binary.chmod(0o755)
        env = {"PATH": os.defpath, "HOME": str(home), "XDG_CONFIG_HOME": str(config),
               "CHAT_CONFIG": str(root / "missing")}
        for arg in ("version", "--version"):
            result = subprocess.run([str(binary), arg], env=env, capture_output=True, check=True, timeout=5)
            assert result.stderr == b""
            expected_version = (f"atuin-ai-server version={manifest['version']} revision={manifest['source']['revision']} "
                                f"source_sha256={manifest['source']['inventory_sha256']} dirty={str(manifest['source']['dirty']).lower()} "
                                "target=linux/amd64 go=go1.27.2\n").encode()
            assert result.stdout == expected_version
            assert list(home.iterdir()) == [] and list(config.iterdir()) == []
        env.pop("CHAT_CONFIG")
        # Select one available port; no retries or external network. A collision fails.
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            port = sock.getsockname()[1]
        explicit = root / "config.toml"
        explicit.write_text(f"port = {port}\n")
        explicit.chmod(0o600)
        for stop_signal in (signal.SIGINT, signal.SIGTERM):
            child = subprocess.Popen([str(binary), "serve", "--config", str(explicit)], env=env,
                                     stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            try:
                with selectors.DefaultSelector() as selector:
                    selector.register(child.stderr, selectors.EVENT_READ)
                    assert selector.select(5), "no startup outcome"
                    log = json.loads(child.stderr.readline())
                assert log["msg"] == "listening" and log["address"] == f"127.0.0.1:{port}", log
                conn = http.client.HTTPConnection("127.0.0.1", port, timeout=5)
                conn.request("GET", "/api/cli/models")
                response = conn.getresponse()
                assert response.status == 503
                body = json.loads(response.read())
                assert body == {"error": "models_unavailable", "message": "Valid application login required for bundled models; check default_model override"}, body
                conn.close()
                # Lock remains held even for safe auth status, but version is independent.
                locked = subprocess.run([str(binary), "auth", "status"], env=env, capture_output=True, timeout=5)
                assert locked.returncode != 0
                subprocess.run([str(binary), "version"], env=env, capture_output=True, timeout=5, check=True)
                child.send_signal(stop_signal)
                stdout, stderr = child.communicate(timeout=10)
                assert child.returncode == 0, (stdout, stderr)
            finally:
                if child.poll() is None:
                    child.kill()
                    child.communicate()
        status = subprocess.run([str(binary), "auth", "status"], env=env, capture_output=True, check=True, timeout=5)
        assert status.stdout == b"" and status.stderr == b"Logged out\n"
        for path in config.rglob("*"):
            assert stat.S_IMODE(path.stat().st_mode) == (0o700 if path.is_dir() else 0o600), path
        assert list(home.iterdir()) == []
    # Install the actual shipping raw binary/legal bundle through fake HTTPS,
    # then execute only the explicitly tested local version command.
    fixture = Installer("test_install_spaces_modes_and_pinned_urls")
    fixture.setUp()
    try:
        fixture.env["TEST_TAG"] = manifest["version"]
        for path in fixture.downloads.iterdir():
            path.unlink()
        for name in (linux["binary"], f"atuin-ai-server-{manifest['version']}-legal.txt", "SHA256SUMS"):
            shutil.copyfile(directory / name, fixture.downloads / name)
        fixture.invoke(ok=True, script=directory / "install.sh")
        assert fixture.old.read_bytes() == (directory / linux["binary"]).read_bytes()
        installed = subprocess.run([str(fixture.old), "version"], env=fixture.env, capture_output=True, check=True, timeout=5)
        assert manifest["source"]["inventory_sha256"].encode() in installed.stdout
    finally:
        fixture.doCleanups()
    print("PASS: checksum/archive/legal inventories + actual raw-asset install + relocated Linux version/503/lock/SIGINT/SIGTERM/restart (no upstream auth/network)")


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--artifacts", type=Path)
    args, unittest_args = parser.parse_known_args()
    result = unittest.main(argv=[__file__, *unittest_args], exit=False)
    if not result.result.wasSuccessful():
        raise SystemExit(1)
    if args.artifacts:
        shipping_smoke(args.artifacts.resolve())
