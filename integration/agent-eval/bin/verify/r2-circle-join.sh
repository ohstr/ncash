#!/usr/bin/env bash
# Ground truth for R2: did `ncash join` actually create a circle_wallet
# child under the ephemeral circle_hub — checked directly against
# lokihub's admin API, not by trusting the agent's self-report.
set -uo pipefail
cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source bin/lib.sh
RUN_DIR="$1"
ROUND="r2-circle-join"

if self_report_exists "${RUN_DIR}" "${ROUND}"; then
  add_check "self_report_written" true "present"
else
  add_check "self_report_written" false "missing — agent never wrote it"
fi

if ! load_admin_config; then
  add_check "admin_api_configured" false "../config.local.yaml missing/incomplete — cannot verify ground truth"
  write_verify "${ROUND}" "${RUN_DIR}"
  exit 0
fi

if [ ! -s fixtures/r2-hub-app-id.txt ]; then
  add_check "fixture_provisioned" false "fixtures/r2-hub-app-id.txt missing — prepare_r2 never ran or failed"
  write_verify "${ROUND}" "${RUN_DIR}"
  exit 0
fi
HUB_APP_ID="$(cat fixtures/r2-hub-app-id.txt)"

if [ ! -s "report/r2-npub.txt" ]; then
  add_check "agent_wrote_npub" false "report/r2-npub.txt missing — agent never completed step 1, so allowlisting/join couldn't have happened"
  write_verify "${ROUND}" "${RUN_DIR}"
  exit 0
fi
add_check "agent_wrote_npub" true "$(cat report/r2-npub.txt)"

CHILDREN="$(admin_api GET "/api/apps/${HUB_APP_ID}/circle/children?limit=0" 2>/dev/null || echo '{}')"
if jq -e '.children | length > 0' >/dev/null 2>&1 <<<"${CHILDREN}"; then
  add_check "circle_wallet_child_exists" true "create_circle_wallet actually landed server-side: $(jq -c '.children' <<<"${CHILDREN}")"
else
  add_check "circle_wallet_child_exists" false "no circle_wallet child found under hub app_id=${HUB_APP_ID}: ${CHILDREN}"
fi

write_verify "${ROUND}" "${RUN_DIR}"
