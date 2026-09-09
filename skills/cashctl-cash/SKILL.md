---
name: cashctl-cash
description: Receive a NIP-CASH token into your local wallet (`cashctl receive`), redeem a held token into a Lightning wallet or raw invoice (`cashctl redeem`), send a held token to someone else in full or split (`cashctl transfer`), merge several held tokens into one (`cashctl consolidate`), and inspect a token locally or against its Hub (`cashctl cash decode/verify-provenance/list-recipients`). Use whenever an agent is handed a lokicash1... (or other cash-token-family) string, needs to cash it out to Lightning, needs to forward it to another identity, or needs to combine multiple small tokens.
license: Unlicense
---

<!-- Mirrors ohstr/cashctl's cmd/cash_receive.go, cmd/cash_redeem.go,
cmd/cash_transfer.go, cmd/cash_consolidate.go, and cmd/cash_inspect.go as
of writing. Self-contained by design — update by hand if flags/schemas
change. -->

# cashctl receive / redeem / transfer / consolidate / cash ...

## `cashctl receive <token>` — cash it in

```sh
cashctl receive lokicash1... --json               # instant, local-only, marked unverified
cashctl receive lokicash1... --verify --json      # cross-checks against the Hub (list_recipients)
cashctl receive lokicash1... --secret <bearer_secret> --json   # bearer-mode tokens only
```

No network call unless `--verify` is passed — decoding and recording the
token locally is enough to hold it. Pasting a Circle Hub (`circlehub1...`)
or Cash Hub (`cashhub1...`) connection here instead of a token gets a
specific corrective error (`code: "invalid_input"`) naming the right
command (`cashctl join --hub ...`, or "cashctl can't mint").

**A bearer-mode token (`identity_required: false`) is two values, not
one.** The token string only lets you dial the wallet — it is *never* a
valid spending credential by itself, no matter how it looks. The real
credential, `bearer_secret`, is a separate value the Hub operator hands
out once, alongside the token, at mint time — pass it with `--secret` when
receiving. If you were only given the token and not its `bearer_secret`,
receiving still succeeds (the entry is saved, just not yet spendable) —
`redeem`/`transfer` will error asking for `--as bearer:<secret>` until one
is supplied, either that way or via a later `receive ... --secret`.

## `cashctl redeem` — cash it out

```sh
cashctl redeem --json                              # auto-picks your one held token + default wallet
cashctl redeem --token tok-a1b2 --to work --json
cashctl redeem --invoice lnbc1... --json           # bypasses both — any invoice, no cashctl wallet needed
```

If you hold more than one token, `--token <id>` is required (`cashctl wallet
show` lists held-token IDs) — auto-pick only applies when exactly one is
held. A **connection-key-bound** token needs an explicit `--as
connection-key:<privkey>,<platform>,<external-id>,<attestation-file>` —
cashctl does not auto-refresh a connection-key credential from a relay
(a deliberate scope limit: re-deriving a fresh live attestation isn't
automatic), so the error names exactly what to pass.

## `cashctl transfer` — send it to someone else

```sh
cashctl transfer --to pubkey:<hex> --json                        # send it all
cashctl transfer --to pubkey:<hex> --split 3000 --json            # keep the rest as a new held token
cashctl transfer --to bearer-target --json
cashctl transfer --to connection:<platform>:<external-id>:<ia-pubkey> --json
```

A successful split transfer's remainder is saved back into your own
ledger automatically — check the response's `remainder_entry` field
(`--json`) or the new `tok-...` ID printed in text mode.

## `cashctl consolidate` — merge several into one

```sh
cashctl consolidate --sources tok-a1b2,tok-c3d4 --json
cashctl consolidate --sources tok-a1b2,lokicash1...:5000:pubkey:<privkey> --to pubkey:<hex> --json
```

`--sources` is comma-separated: a bare ID already in your ledger (amount/
credential resolved automatically), or the verbose `<token>:<amount>:
<credential>` form for a source that isn't held locally — only `pubkey:`/
`bearer:` credentials work in that verbose form (a `connection-key:` value
has its own embedded commas, ambiguous in this shorthand — receive it into
your ledger first instead). Needs at least 2 sources. `--to` defaults to
your own identity.

## `cashctl cash list-recipients` / `decode` / `verify-provenance`

```sh
cashctl cash list-recipients --token tok-a1b2 --json  # network call — your allocation + co-recipients
cashctl cash decode lokicash1... --json                # local-only: {"hrp","wallet_pubkey","relays",...}
cashctl cash verify-provenance lokicash1... --json     # local-only: {"valid","minter_pubkey"}
```

`decode`/`verify-provenance` never touch the network — they work on any
token string, held or not, so they're the cheapest way to inspect one
before deciding whether to `receive` it at all.
