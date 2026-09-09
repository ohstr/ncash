#!/usr/bin/env bash
# Ground truth for R1: did the token actually get redeemed server-side —
# checked directly against lokihub's admin API (the same one that minted
# it), not by trusting the agent's self-report or cashctl's own CLI output.
set -uo pipefail
cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
source bin/lib.sh
RUN_DIR="$1"
ROUND="r1-cash-lifecycle"

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

if [ ! -s fixtures/r1-hub-app-id.txt ]; then
  add_check "fixture_provisioned" false "fixtures/r1-hub-app-id.txt missing — prepare_r1 never ran or failed"
  write_verify "${ROUND}" "${RUN_DIR}"
  exit 0
fi
HUB_APP_ID="$(cat fixtures/r1-hub-app-id.txt)"

CLAIMS="$(admin_api GET "/api/apps/${HUB_APP_ID}/cash-wallets?limit=0" 2>/dev/null || echo '{}')"
if jq -e '.claims | length > 0' >/dev/null 2>&1 <<<"${CLAIMS}"; then
  add_check "cash_wallet_child_exists" true "mint_cash's own child is visible via the admin API"
  if jq -e '.claims[0].claimed == true' >/dev/null 2>&1 <<<"${CLAIMS}"; then
    add_check "token_actually_redeemed" true "the claim's server-side 'claimed' flag is true — the invoice was actually paid, not just reported"
  else
    add_check "token_actually_redeemed" false "the claim is still unclaimed server-side: ${CLAIMS}"
  fi
else
  add_check "cash_wallet_child_exists" false "no cash_wallet child found under hub app_id=${HUB_APP_ID}: ${CLAIMS}"
fi

write_verify "${ROUND}" "${RUN_DIR}"
