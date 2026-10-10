#!/usr/bin/env bash
# =====================================================================
#  apt-deploy.sh — upload a .deb to the apt.thugs.red repository API.
#
#  Invoked by `make apt-thugs-red`. Reads the API key and endpoint from
#  the environment (populated from .apt-deploy.env, which is git-ignored).
#  The key is NEVER printed and NEVER committed.
#
#  Usage: scripts/apt-deploy.sh <path-to.deb>
#
#  Environment:
#    XXC_APT_KEY      (required)  API token for apt.thugs.red
#    APT_ENDPOINT     upload URL        (default https://apt.thugs.red/api/upload)
#    APT_DIST         distribution      (default stable)
#    APT_COMPONENT    component         (default main)
#    APT_AUTH_HEADER  auth header name  (default "Authorization: Bearer")
#
#  NOTE: adjust APT_ENDPOINT / fields below to match the actual
#  apt.thugs.red API contract if it differs.
# =====================================================================
set -euo pipefail

DEB="${1:-}"
if [ -z "$DEB" ] || [ ! -f "$DEB" ]; then
    echo "apt-deploy: missing or invalid .deb argument: '${DEB}'" >&2
    exit 2
fi

: "${XXC_APT_KEY:?apt-deploy: XXC_APT_KEY is required}"
APT_ENDPOINT="${APT_ENDPOINT:-https://apt.thugs.red/api/upload}"
APT_DIST="${APT_DIST:-stable}"
APT_COMPONENT="${APT_COMPONENT:-main}"
APT_AUTH_HEADER="${APT_AUTH_HEADER:-Authorization: Bearer}"

command -v curl >/dev/null 2>&1 || { echo "apt-deploy: curl not found" >&2; exit 3; }

echo "  -> ${DEB##*/}  =>  ${APT_ENDPOINT}  [${APT_DIST}/${APT_COMPONENT}]"

# Upload. The response body is captured; the HTTP code decides success.
http_code=$(curl -sS -o /tmp/xxc-apt-deploy.out -w '%{http_code}' \
    -X POST "${APT_ENDPOINT}" \
    -H "${APT_AUTH_HEADER} ${XXC_APT_KEY}" \
    -H "X-Api-Key: ${XXC_APT_KEY}" \
    -F "distribution=${APT_DIST}" \
    -F "component=${APT_COMPONENT}" \
    -F "file=@${DEB};type=application/vnd.debian.binary-package") || {
        echo "apt-deploy: upload failed (curl error)" >&2
        exit 4
    }

if [ "$http_code" -ge 200 ] && [ "$http_code" -lt 300 ]; then
    echo "  OK (HTTP $http_code)"
    rm -f /tmp/xxc-apt-deploy.out
    exit 0
fi

echo "apt-deploy: server returned HTTP $http_code" >&2
echo "----- response -----" >&2
cat /tmp/xxc-apt-deploy.out >&2 || true
echo >&2
echo "--------------------" >&2
rm -f /tmp/xxc-apt-deploy.out
exit 5
