#!/usr/bin/env bash
# Orchestrates one full run of the ncash agent-capability eval: brings up
# the isolated stack, provisions each round's real lokihub fixtures (a
# minted cash token, a plain payer wallet, an ephemeral circle_hub) via
# the admin API named in ../config.local.yaml, drives each round as a
# fresh, non-interactive `claude -p` invocation inside the agent
# container, runs a harness-side deterministic verifier and an LLM-judge
# pass against each round's transcript, and assembles one report.
#
# Usage:
#   bin/run.sh                          # every round, in order
#   bin/run.sh r0-bootstrap r3-error-contract   # just these, in order given
#
# Requires: docker + docker compose, this host already logged into Claude
# Code (`claude` has worked here at least once), a Go toolchain (for
# decode-npub/mint-fixture), and ../config.local.yaml pointing at a real,
# already-running lokihub instance's admin API — see README.md.
set -euo pipefail
cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
source bin/lib.sh

ALL_ROUNDS=(r0-bootstrap r1-cash-lifecycle r2-circle-join r3-error-contract)
ROUNDS=("${@:-${ALL_ROUNDS[@]}}")

RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
RUN_DIR="report/${RUN_ID}"
mkdir -p "${RUN_DIR}" report fixtures
chmod 777 report fixtures

echo "==> run ${RUN_ID}: ${ROUNDS[*]}"

if ! load_admin_config; then
  echo "ERROR: ../config.local.yaml missing or incomplete — see README.md (r1-cash-lifecycle and r2-circle-join need it; r0-bootstrap and r3-error-contract don't need network fixtures at all)." >&2
fi

# --- credentials: copy, never bind-mount the host's real ~/.claude -------
HOST_CREDS="${HOME}/.claude/.credentials.json"
HOST_CLAUDE_JSON="${HOME}/.claude.json"
if [ ! -f "${HOST_CREDS}" ]; then
  echo "ERROR: ${HOST_CREDS} not found. Log into Claude Code on this host first (run 'claude' once) before running this suite." >&2
  exit 1
fi
rm -rf .creds-seed
mkdir -p .creds-seed
cp "${HOST_CREDS}" .creds-seed/credentials.json
[ -f "${HOST_CLAUDE_JSON}" ] && cp "${HOST_CLAUDE_JSON}" .creds-seed/claude.json || true
# uid 10001 == evaluser inside agent/Dockerfile.
chown -R 10001:10001 .creds-seed
chmod 700 .creds-seed
chmod 600 .creds-seed/*.json

FIXTURE_HUB_APP_IDS=()
cleanup() {
  echo "==> tearing down"
  for id in "${FIXTURE_HUB_APP_IDS[@]:-}"; do
    [ -n "${id}" ] || continue
    GOWORK=off go run ./mint-fixture cleanup "${ADMIN_BASE_URL}" "${ADMIN_TOKEN}" "${id}" 2>/dev/null || true
  done
  docker compose down -v >/dev/null 2>&1 || true
  rm -rf .creds-seed fixtures/*
}
trap cleanup EXIT

echo "==> ensuring a clean slate (tearing down any leftover stack from a prior run)"
docker compose down -v >/dev/null 2>&1 || true

echo "==> building agent image"
docker compose build agent

echo "==> starting stack"
docker compose up -d

echo "==> waiting for the agent container to be responsive"
for _ in $(seq 1 30); do
  docker compose exec -T agent true >/dev/null 2>&1 && break
  sleep 1
done

run_round() {
  local round="$1"
  local prompt
  prompt="$(cat rounds/_preamble.md; echo; cat "rounds/${round}.md")"
  echo "==> [${round}] running agent"
  docker compose exec -T agent claude -p "${prompt}" \
    --output-format stream-json --verbose \
    --permission-mode bypassPermissions \
    > "${RUN_DIR}/${round}.transcript.jsonl" 2> "${RUN_DIR}/${round}.stderr.log"
  local status=$?
  cp "report/${round}.self-report.json" "${RUN_DIR}/${round}.self-report.json" 2>/dev/null \
    || echo "WARNING: [${round}] agent did not write a self-report" >&2
  return ${status}
}

# prepare_r1 mints a real bearer cash token and a plain payer wallet
# before the round starts — see rounds/r1-cash-lifecycle.md.
prepare_r1() {
  echo "==> [r1-cash-lifecycle] minting a bearer cash token"
  local mint_out hub_id
  mint_out="$(GOWORK=off go run ./mint-fixture mint "${ADMIN_BASE_URL}" "${ADMIN_TOKEN}" 250000)"
  echo "${mint_out}" | jq -r '.cash_token' > fixtures/r1-cash-token.txt
  hub_id="$(echo "${mint_out}" | jq -r '.hub_app_id')"
  echo "${hub_id}" > fixtures/r1-hub-app-id.txt
  FIXTURE_HUB_APP_IDS+=("${hub_id}")

  echo "==> [r1-cash-lifecycle] provisioning a plain payer wallet"
  local wallet
  wallet="$(admin_api POST /api/apps '{"name":"ncash agent-eval r1-cash-lifecycle payout","scopes":["make_invoice","get_balance"]}')"
  echo "${wallet}" | jq -r '.pairingUri' > fixtures/r1-wallet-uri.txt
  FIXTURE_HUB_APP_IDS+=("$(echo "${wallet}" | jq -r '.id')")
}

# prepare_r2 provisions an ephemeral allowlist circle_hub ahead of time
# (creation itself needs no pubkey) — the pubkey authorization happens
# mid-round, in run_r2 below, once the agent has generated its identity.
prepare_r2() {
  echo "==> [r2-circle-join] provisioning an ephemeral circle_hub"
  local hub
  hub="$(admin_api POST /api/apps '{"name":"ncash agent-eval r2-circle-join","kind":"circle_hub","scopes":["circle_wallet"],"circleIdentityName":"ncash agent-eval r2 identity","circlePolicy":"allowlist","circleMaxExpSecs":86400,"circlePerWalletMaxMloki":1000000}')"
  R2_HUB_APP_ID="$(echo "${hub}" | jq -r '.id')"
  R2_HUB_TOKEN="$(echo "${hub}" | jq -r '.circleHubToken')"
  echo "${R2_HUB_APP_ID}" > fixtures/r2-hub-app-id.txt
  FIXTURE_HUB_APP_IDS+=("${R2_HUB_APP_ID}")
  admin_api POST /api/transfers "{\"toAppId\":${R2_HUB_APP_ID},\"amountLoki\":100}" >/dev/null
}

# run_r2 runs the round in the background, polls report/r2-npub.txt for
# the agent's freshly generated identity, authorizes it under the
# circle_hub prepare_r2 provisioned, and only then drops
# fixtures/r2-hub.txt for the round to pick up — mirrors ncli's own
# run_r6's "external mid-round provisioning" shape.
run_r2() {
  rm -f "report/r2-npub.txt" "fixtures/r2-hub.txt"
  run_round r2-circle-join &
  local claude_pid=$!
  local authorized=0
  for _ in $(seq 1 60); do
    if [ -s "report/r2-npub.txt" ]; then
      local npub pubkey_hex
      npub="$(tr -d '[:space:]' < report/r2-npub.txt)"
      echo "==> [r2-circle-join] agent identity: ${npub}"
      pubkey_hex="$(decode_npub_hex "${npub}")"
      admin_api PUT "/api/apps/${R2_HUB_APP_ID}/circle/allowlist" "{\"pubkeys\":[\"${pubkey_hex}\"]}" >/dev/null
      echo "${R2_HUB_TOKEN}" > fixtures/r2-hub.txt
      authorized=1
      break
    fi
    sleep 3
  done
  [ "${authorized}" -eq 1 ] || echo "WARNING: [r2-circle-join] agent never wrote report/r2-npub.txt" >&2
  wait "${claude_pid}" || true
}

for round in "${ROUNDS[@]}"; do
  case "${round}" in
    r1-cash-lifecycle)
      load_admin_config && prepare_r1 || echo "WARNING: [${round}] admin_api not configured — skipping fixture provisioning; round will fail" >&2
      run_round "${round}" || echo "WARNING: [${round}] claude invocation exited non-zero" >&2
      ;;
    r2-circle-join)
      load_admin_config && prepare_r2 || echo "WARNING: [${round}] admin_api not configured — skipping fixture provisioning; round will fail" >&2
      run_r2
      ;;
    *)
      run_round "${round}" || echo "WARNING: [${round}] claude invocation exited non-zero" >&2
      ;;
  esac
  bin/verify/"${round}.sh" "${RUN_DIR}" || echo "WARNING: [${round}] verifier reported problems" >&2
done

echo "==> judging transcripts"
bin/judge.sh "${RUN_DIR}" "${ROUNDS[@]}"

echo "==> assembling report"
bin/report.sh "${RUN_DIR}" "${ROUNDS[@]}"

echo "==> done: ${RUN_DIR}/report.md"
