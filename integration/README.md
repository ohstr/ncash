# Integration tests

A black-box suite that drives the *compiled* `ncash` binary as a real user
would, against a real, already-running [lokihub](https://github.com/ohstr/lokihub)
instance — as opposed to the unit tests under `internal/`, which never touch
a network.

Excluded from normal `go test`/CI runs by the `integration` build tag:

```sh
go test -tags integration ./integration/...
```

## Setup

1. Copy `config.local.yaml.example` to `config.local.yaml` (gitignored —
   never committed).
2. Point `admin_api.base_url` at a running lokihub instance's admin HTTP API
   (the same one its frontend calls).
3. Mint a bearer token: `POST {base_url}/api/unlock` with the instance's
   unlock password, `"permission": "full"`, and an explicit
   `token_expiry_days`. Paste the resulting token into `admin_api.token`.

That's the *only* fixture this suite needs pre-provisioned. Every
cash_hub/circle_hub each test exercises is minted on demand through that
admin API and torn down again in its own `t.Cleanup` (see `admin_client.go`)
— there's no long-lived hub to hand-set-up first.

If `config.local.yaml` is missing, or `admin_api` is left blank, every test
skips cleanly (not a failure) — this suite is opt-in.

The token expires (30 days by default). An expired token makes tests fail
loudly, not skip — that's a config problem for the operator to refresh, not
a capability gap in ncash.

## What it proves

Each test drives the real compiled binary as a subprocess (`binary.go`
builds it once, cached for the whole run) with its own fully isolated
`--config-dir` and `XDG_CONFIG_HOME`, so runs never interact with each other
or with a real user's own ncash/ncli state:

- `TestCashLifecycle_MintReceiveRedeem` — mints a real cash token to a
  freshly generated local identity via a live cash_hub, `ncash receive
  --verify`s it, and `ncash redeem`s it into a real invoice from the same
  hub. The full mint → receive → redeem round trip, proven live.
- `TestCashInspect_DecodeAndListRecipients` — mints a bearer token, then
  exercises `ncash cash decode` (local-only) and `ncash cash
  list-recipients` (network) against it.
- `TestCircleJoin_CreateWalletAndGetInfo` — provisions an allowlist-policy
  circle_hub, authorizes ncash's local identity under it, joins via `ncash
  join --hub <circlehub1...>` (the real bech32 Circle Hub connection format,
  end to end), and confirms the resulting personal wallet is live via
  `ncash wallet get-info`.

Everything else — the CLI's error contract, confirmation flows, flag
parsing, and cross-command Sniff-based routing — is covered by `internal/`'s
unit tests and the `agent-eval/` suite instead; this suite exists
specifically to prove the network-touching command bodies work against a
real server, not to re-litigate logic already covered elsewhere.
