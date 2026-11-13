"""A scratch present: the binary, its serve process, and pages created through `present mcp`.

Pages go in through the MCP's present_create, the path an agent uses, so the
tool's own checks (Doc validation, deck and terminal checks, image ingest) run.
"""

import json
import os
import re
import socket
import subprocess
import time
import urllib.request
from pathlib import Path

from common import HERE, file_sha256, log

ASSETS = Path("~/.config/present/assets").expanduser()
PAGE_ID = re.compile(r"/p/([0-9a-f]{8,})")
PRESENT_PKG = "./services/present/cmd/present"
BUILDINFO_PKG = "github.com/mad01/thismoon/buildinfo"
# What a present binary is built from, for the dirty check on a checkout build.
PRESENT_PATHS = ("services/present", "webkit", "buildinfo", "kit", "go.mod", "go.sum")


def repo_root():
    out = subprocess.run(["git", "-C", str(HERE), "rev-parse", "--show-toplevel"],
                         capture_output=True, text=True, check=True)
    return Path(out.stdout.strip())


def git(root, *args):
    return subprocess.run(["git", "-C", str(root), *args], capture_output=True, text=True,
                          check=True).stdout.rstrip()  # rstrip: porcelain lines start with a space


def ldflags(sha, version):
    stamp = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
    return (f"-X {BUILDINFO_PKG}.Version={version} -X {BUILDINFO_PKG}.Commit={sha} "
            f"-X {BUILDINFO_PKG}.BuildTime={stamp}")


def build_present(out_dir):
    """Build present from the checkout this harness sits in, uncommitted changes
    included; return (binary, source). A dirty build's version ends in -dirty."""
    out = Path(out_dir) / "bin" / "present"
    out.parent.mkdir(parents=True, exist_ok=True)
    root = repo_root()
    sha = git(root, "rev-parse", "HEAD")
    dirty = git(root, "status", "--porcelain", "--", *PRESENT_PATHS).splitlines()
    version = sha[:7] + ("-dirty" if dirty else "")
    log(f"building present from {root} ({version})")
    subprocess.run(["go", "build", "-ldflags", ldflags(sha, version), "-o", str(out), PRESENT_PKG],
                   cwd=root, check=True)
    return out, {"source": "checkout", "checkout": str(root), "commit": sha, "dirty": dirty}


def build_present_ref(ref, out_dir):
    """Build present from a git ref of this repo, through `git archive` into a
    scratch tree, so the checkout is never touched; return (binary, source).
    The binary is kept per commit and reused."""
    root = repo_root()
    sha = git(root, "rev-parse", "--verify", f"{ref}^{{commit}}")
    out = Path(out_dir) / "bin" / f"present-{sha[:12]}"
    source = {"source": "ref", "ref": ref, "commit": sha, "dirty": []}
    if out.exists():
        return out, source
    tree = Path(out_dir) / "src" / sha[:12]
    if not (tree / "go.mod").exists():
        tree.mkdir(parents=True, exist_ok=True)
        archive = subprocess.Popen(["git", "-C", str(root), "archive", sha], stdout=subprocess.PIPE)
        subprocess.run(["tar", "-x", "-C", str(tree)], stdin=archive.stdout, check=True)
        if archive.wait() != 0:
            raise SystemExit(f"git archive {sha} failed")
    log(f"building present from {ref} ({sha[:12]})")
    out.parent.mkdir(parents=True, exist_ok=True)
    subprocess.run(["go", "build", "-ldflags", ldflags(sha, sha[:7]), "-o", str(out), PRESENT_PKG],
                   cwd=tree, check=True)
    return out, source


def describe_binary(binary, source):
    """What the shots record about the renderer: path, version, file hash, and where it came from."""
    return {"binary": str(binary), "version": binary_version(binary), "sha256": file_sha256(binary), **source}


def binary_version(binary):
    out = subprocess.run([str(binary), "version", "-o", "json"], capture_output=True, text=True)
    try:
        return json.loads(out.stdout)
    except ValueError:
        return {"raw": out.stdout.strip()}


def present_env(workdir, port):
    env = {k: v for k, v in os.environ.items() if not k.startswith("PRESENT_")}
    env.update(PRESENT_WORKDIR=str(workdir), PRESENT_PORT=str(port), PRESENT_SPEAK_URL="")
    return env


def prepare_workdir(workdir):
    workdir = Path(workdir)
    workdir.mkdir(parents=True, exist_ok=True)
    link = workdir / "assets"
    if not link.exists():
        if not ASSETS.exists():
            raise SystemExit(f"{ASSETS} is missing: run `make cache` in services/present first")
        link.symlink_to(ASSETS)
    return workdir


def port_busy(port):
    with socket.socket() as sock:
        return sock.connect_ex(("127.0.0.1", port)) == 0


class Serve:
    """`present serve` on a scratch workdir and port, stopped on exit."""

    def __init__(self, binary, workdir, port, log_path):
        self.binary, self.workdir, self.port, self.log_path = binary, workdir, port, log_path
        self.proc = None

    def __enter__(self):
        if port_busy(self.port):
            raise SystemExit(f"port {self.port} is already in use; stop that process or pass --port")
        self._log = open(self.log_path, "a")
        self.proc = subprocess.Popen([str(self.binary), "serve", "--port", str(self.port)],
                                     env=present_env(self.workdir, self.port),
                                     stdout=self._log, stderr=subprocess.STDOUT)
        self._wait_up()
        log(f"present serve pid {self.proc.pid} on :{self.port}, workdir {self.workdir}")
        return self

    def _wait_up(self):
        deadline = time.monotonic() + 20
        while time.monotonic() < deadline:
            if self.proc.poll() is not None:
                raise SystemExit(f"present serve exited {self.proc.returncode}; see {self.log_path}")
            try:
                urllib.request.urlopen(f"http://localhost:{self.port}/version", timeout=2)
                return
            except OSError:
                time.sleep(0.3)
        raise SystemExit(f"present serve did not answer on :{self.port}; see {self.log_path}")

    def __exit__(self, *exc):
        if self.proc and self.proc.poll() is None:
            self.proc.terminate()
            try:
                self.proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                self.proc.kill()
        self._log.close()


class McpClient:
    """A `present mcp` stdio process spoken to in newline-delimited JSON-RPC."""

    def __init__(self, binary, workdir, port, stderr_path):
        self._err = open(stderr_path, "a")
        self.proc = subprocess.Popen([str(binary), "mcp"], env=present_env(workdir, port),
                                     stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                     stderr=self._err, text=True)
        self._id = 0
        self._call("initialize", {"protocolVersion": "2025-06-18", "capabilities": {},
                                  "clientInfo": {"name": "looks-eval", "version": "1"}})
        self._send({"jsonrpc": "2.0", "method": "notifications/initialized"})

    def _send(self, msg):
        self.proc.stdin.write(json.dumps(msg) + "\n")
        self.proc.stdin.flush()

    def _call(self, method, params):
        self._id += 1
        self._send({"jsonrpc": "2.0", "id": self._id, "method": method, "params": params})
        while True:
            line = self.proc.stdout.readline()
            if not line:
                raise RuntimeError(f"present mcp closed its output during {method}")
            msg = json.loads(line)
            if msg.get("id") == self._id:
                return msg

    def create(self, args):
        """Return (answer, error): answer is the tool's JSON reply (id, url, has_deck),
        error is the tool's refusal text, or ''."""
        msg = self._call("tools/call", {"name": "present_create", "arguments": args})
        if "error" in msg:
            return {}, f"JSON-RPC error: {msg['error']}"
        result = msg.get("result") or {}
        text = "\n".join(c.get("text", "") for c in result.get("content", []) if c.get("type") == "text")
        if result.get("isError"):
            return {}, text or "present_create refused the page"
        try:
            answer = json.loads(text)
        except ValueError:
            match = PAGE_ID.search(text)
            answer = {"id": match.group(1)} if match else {}
        if not answer.get("id"):
            return {}, f"present_create answered without a page id: {text[:300]}"
        return answer, ""

    def close(self):
        if self.proc.poll() is None:
            self.proc.stdin.close()
            try:
                self.proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                self.proc.kill()
        self._err.close()
