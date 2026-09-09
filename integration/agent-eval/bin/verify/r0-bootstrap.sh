#!/usr/bin/env bash
# Ground truth for R0: is cashctl *actually* installed and runnable, verified
# by the harness invoking it directly — not by trusting the agent's claim.
set -uo pipefail
cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source bin/lib.sh
RUN_DIR="$1"
ROUND="r0-bootstrap"

if self_report_exists "${RUN_DIR}" "${ROUND}"; then
  add_check "self_report_written" true "present"
else
  add_check "self_report_written" false "missing — agent never wrote it"
fi

VERSION_JSON="$(agent_exec 'cashctl version --json' 2>/dev/null)"
if [ -n "${VERSION_JSON}" ] && jq -e '.version' >/dev/null 2>&1 <<<"${VERSION_JSON}"; then
  add_check "cashctl_installed_and_runnable" true "cashctl version --json -> version=$(jq -r '.version' <<<"${VERSION_JSON}")"
else
  add_check "cashctl_installed_and_runnable" false "cashctl version --json did not return valid JSON with a version field"
fi

INIT_JSON="$(agent_exec 'cashctl init --json' 2>/dev/null)"
if jq -e '.npub | test("^npub1")' >/dev/null 2>&1 <<<"${INIT_JSON}"; then
  add_check "init_json_sane_shape" true "cashctl init --json returns a well-formed npub"
else
  add_check "init_json_sane_shape" false "cashctl init --json output missing/malformed npub: ${INIT_JSON}"
fi

write_verify "${ROUND}" "${RUN_DIR}"
