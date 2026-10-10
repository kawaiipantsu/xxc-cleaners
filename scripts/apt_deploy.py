#!/usr/bin/env python3
"""Deploy .deb packages to the THUGS(red) APT repository (XXC-APTD).

Implements the documented administrative API v1 workflow:
    upload  ->  stage  ->  review diff  ->  publish  ->  poll job

See https://apt.thugs.red/api  (OpenAPI at /api/openapi.json).

The bearer token is read from the environment (XXC_APT_KEY), never from
argv, and is never printed. Only the Python standard library is used.

Environment:
    XXC_APT_KEY      (required)  bearer token (xxc_aptd_<64 hex>)
    XXC_API_BASE     admin API base   (default https://apt.thugs.red/admin/api/v1)
    XXC_SUITE        target suite     (default zerotrust)
    XXC_STAGE_ONLY   "1" to upload+stage but NOT publish (default off)
    XXC_ALLOW_REMOVALS "1" to allow a diff with removals/downgrades (default off)
    XXC_POLL_TIMEOUT seconds to wait for the publish job (default 900)

Usage:
    apt_deploy.py <pkg1.deb> [pkg2.deb ...]
"""
from __future__ import annotations

import json
import os
import sys
import time
import urllib.error
import urllib.request

API_BASE = os.environ.get("XXC_API_BASE", "https://apt.thugs.red/admin/api/v1").rstrip("/")
SUITE = os.environ.get("XXC_SUITE", "zerotrust")
STAGE_ONLY = os.environ.get("XXC_STAGE_ONLY", "") in ("1", "true", "yes")
ALLOW_REMOVALS = os.environ.get("XXC_ALLOW_REMOVALS", "") in ("1", "true", "yes")
POLL_TIMEOUT = int(os.environ.get("XXC_POLL_TIMEOUT", "900"))

DEB_CT = "application/vnd.debian.binary-package"


def die(msg: str, code: int = 1) -> "None":
    print(f"apt-deploy: {msg}", file=sys.stderr)
    sys.exit(code)


def load_token() -> str:
    tok = os.environ.get("XXC_APT_KEY", "").strip()
    if not tok:
        die("XXC_APT_KEY not set (export it or create .apt-deploy.env)", 2)
    # Validate shape per the API docs: xxc_aptd_ + 64 lowercase hex = 73 chars.
    if not (tok.startswith("xxc_aptd_") and len(tok) == 73 and all(c in "0123456789abcdef" for c in tok[9:])):
        die("XXC_APT_KEY does not look like a valid token (xxc_aptd_<64 hex>)", 2)
    return tok


def request(method: str, path: str, token: str, *, body: bytes | None = None,
            content_type: str | None = None, suite: bool = True):
    url = API_BASE + path
    if suite:
        url += ("&" if "?" in url else "?") + "suite=" + SUITE
    headers = {"Authorization": f"Bearer {token}", "Accept": "application/json"}
    if content_type:
        headers["Content-Type"] = content_type
    req = urllib.request.Request(url, data=body, method=method, headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=120) as resp:  # noqa: S310 (fixed https base)
            raw = resp.read()
            code = resp.status
    except urllib.error.HTTPError as e:
        raw = e.read()
        code = e.code
    except urllib.error.URLError as e:
        die(f"{method} {path}: connection failed: {e.reason}", 4)
    data = None
    if raw:
        try:
            data = json.loads(raw)
        except json.JSONDecodeError:
            data = {"_raw": raw[:500].decode("utf-8", "replace")}
    return code, data


def expect(code: int, data, want: int, what: str):
    if code != want:
        detail = ""
        if isinstance(data, dict):
            detail = data.get("error") or data.get("message") or data.get("_raw") or json.dumps(data)[:300]
        die(f"{what}: expected HTTP {want}, got {code}. {detail}", 5)


def main(argv: list[str]) -> int:
    debs = [a for a in argv if a.endswith(".deb")]
    if not debs:
        die("no .deb files given", 2)
    for d in debs:
        if not os.path.isfile(d):
            die(f"not a file: {d}", 2)

    token = load_token()
    print(f"apt-deploy: {API_BASE}  suite={SUITE}")

    # 1. status / connectivity + auth check.
    code, data = request("GET", "/status", token)
    if code == 404:
        die("management API returned 404 — the admin proxy route is not enabled "
            "at this base. Ask the operator for the admin API base and set "
            "XXC_API_BASE (see https://apt.thugs.red/api).", 6)
    expect(code, data, 200, "GET /status")
    print(f"  server XXC-APTD {data.get('version')} · generation {data.get('generation')} "
          f"· {data.get('packages')} packages, {data.get('staged')} staged")

    # 2. suite authorization (best effort).
    code, sl = request("GET", "/suites", token, suite=False)
    if code == 200 and isinstance(sl, dict):
        allowed = sl.get("suites") or []
        if SUITE not in allowed:
            die(f"token is not authorized for suite '{SUITE}' (allowed: {', '.join(allowed) or 'none'})", 6)

    # 3. upload + stage each package.
    for deb in debs:
        with open(deb, "rb") as fh:
            payload = fh.read()
        print(f"  uploading {os.path.basename(deb)} ({len(payload)} bytes) ...")
        code, pkg = request("POST", "/uploads", token, body=payload, content_type=DEB_CT)
        expect(code, pkg, 200, f"POST /uploads ({deb})")
        pid = pkg.get("id")
        print(f"    -> {pkg.get('name')} {pkg.get('version')} {pkg.get('architecture')} "
              f"[{pkg.get('component')}]  id={pid}")
        code, st = request("POST", f"/uploads/{pid}/stage", token)
        expect(code, st, 200, f"POST /uploads/{pid}/stage")
        if not (isinstance(st, dict) and st.get("staged")):
            die(f"stage did not confirm for {pid}: {st}", 5)
        print("    staged.")

    # 4. review the diff for the suite.
    code, diff = request("GET", "/repository/diff", token)
    expect(code, diff, 200, "GET /repository/diff")
    added = diff.get("added") or []
    removed = diff.get("removed") or []
    downgrades = diff.get("downgrades") or []
    review_token = diff.get("token")
    print(f"  diff: +{len(added)} / -{len(removed)} / downgrades {len(downgrades)} "
          f"· size delta {diff.get('size_delta')} bytes")

    if not added and not removed and not downgrades:
        print("  nothing to publish (packages already current). Done.")
        return 0

    # 5. review policy: refuse removals/downgrades unless explicitly allowed.
    if (removed or downgrades) and not ALLOW_REMOVALS:
        die("diff contains removals/downgrades; refusing to publish. "
            "Set XXC_ALLOW_REMOVALS=1 to override after review.", 7)

    if STAGE_ONLY:
        print("  XXC_STAGE_ONLY set — staged but not published. "
              "Publish later with the review token from the diff.")
        return 0

    # 6. publish using the exact review token from the diff.
    code, acc = request("POST", "/repository/publish", token,
                        body=json.dumps({"review_token": review_token}).encode(),
                        content_type="application/json")
    expect(code, acc, 202, "POST /repository/publish")
    job_id = acc.get("job_id")
    print(f"  publish accepted · job {job_id}")

    # 7. poll the job.
    deadline = time.time() + POLL_TIMEOUT
    while time.time() < deadline:
        code, job = request("GET", f"/jobs/{job_id}", token)
        expect(code, job, 200, f"GET /jobs/{job_id}")
        state = job.get("state")
        if state == "succeeded":
            print(f"  published. {job.get('message', '')}")
            return 0
        if state == "failed":
            die(f"publish job failed: {job.get('message')}", 8)
        time.sleep(3)
    die("timed out waiting for the publish job", 8)
    return 8


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
