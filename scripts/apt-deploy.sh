#!/usr/bin/env bash
# =====================================================================
#  apt-deploy.sh — upload & publish .deb(s) to apt.thugs.red (XXC-APTD).
#
#  Thin wrapper around scripts/apt_deploy.py, which implements the real
#  administrative API v1 workflow (upload -> stage -> diff -> publish ->
#  poll). See https://apt.thugs.red/api
#
#  Invoked by `make apt-thugs-red`. The API token is read from the
#  environment (XXC_APT_KEY) or from .apt-deploy.env (git-ignored). It is
#  never placed in argv and never printed.
#
#  Usage: scripts/apt-deploy.sh <pkg1.deb> [pkg2.deb ...]
# =====================================================================
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
root="$(cd "$here/.." && pwd)"

# Load the token/env from .apt-deploy.env when not already in the environment.
if [ -z "${XXC_APT_KEY:-}" ] && [ -f "$root/.apt-deploy.env" ]; then
    set -a
    # shellcheck disable=SC1091
    . "$root/.apt-deploy.env"
    set +a
fi

if [ -z "${XXC_APT_KEY:-}" ]; then
    echo "apt-deploy: XXC_APT_KEY not set (export it or create .apt-deploy.env)" >&2
    echo "            cp .apt-deploy.env.example .apt-deploy.env  and add your token" >&2
    exit 2
fi

command -v python3 >/dev/null 2>&1 || { echo "apt-deploy: python3 is required" >&2; exit 3; }

# -I: isolated mode (ignore env PYTHONPATH and the script dir on sys.path).
exec python3 -I "$here/apt_deploy.py" "$@"
