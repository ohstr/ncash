---
name: cashctl-wallet
description: Set up a cashctl identity and default wallet (`cashctl init`), manage registered NWC connections (`cashctl connect add/list/use/rm`, `cashctl wallet use`), inspect identity/wallets/history (`cashctl wallet show/history`), check a unified balance across every wallet and held cash token (`cashctl wallet balance`), and make ordinary NIP-47 calls against the current wallet (`cashctl wallet get-info/budget/invoice/pay/list-tx/sign-message`). Use when setting up cashctl for the first time, registering a plain Lightning wallet connection, switching the default wallet, checking balance/budget, or paying/creating a Lightning invoice.
license: Unlicense
---

<!-- Mirrors ohstr/cashctl's cmd/wallet_init.go, cmd/connect.go, cmd/wallet.go,
cmd/wallet_balance.go, and cmd/wallet_ops.go as of writing. Self-contained
by design — update by hand if flags/schemas change. -->

# cashctl init / wallet / connect

## `cashctl init` — first-time setup

```sh
cashctl init            # interactive: offers an existing ncli vault entry, or generates one
cashctl init --json      # scripted: always generates fresh, skips the wallet-connection offer
```

Idempotent: re-running it just reports `{"already_configured": true}`
(`--json`) or a one-line message (text mode) rather than erasing/
regenerating anything. There is no `--force`/reset flag — remove
`identity.json` under `cashctl`'s config dir (see `cashctl wallet show` for
where that is, or pass `--config-dir` to point cashctl at a fresh one) if you
genuinely want to start over.

Under `--json`, the wallet-connection offer is always skipped (there's no
way to paste a connection string non-interactively in that one prompt) —
register one afterward with `cashctl connect add` instead.

## `cashctl connect` / `cashctl wallet use` — managing wallet connections

```sh
cashctl connect add work nostr+walletconnect://...
cashctl connect list --json
cashctl wallet use work    # same as `cashctl connect use work`
cashctl connect rm work
```

`connect add`'s first-ever wallet is auto-set as default under `--json`
(and interactively offered under text mode); every subsequent one is only
offered, never auto-set. A wallet is looked up by name everywhere a
connection value is expected (`--to`, `-c/--connection`, `wallet use`) — if
the name isn't found, most of these accept the raw value directly instead
(lets a script pass an inline URI without a prior `connect add`).

## `cashctl wallet show` / `history`

```sh
cashctl wallet show --json     # {"npub", "identity_source", "wallets", "default_wallet", "held_tokens"}
cashctl wallet history --json  # {"history": [{"at","action","detail"}, ...]}
```

## `cashctl wallet balance` — the unified figure

```sh
cashctl wallet balance --json                # {"total_mloki", "stranded_mloki", "breakdown"}
cashctl wallet balance --breakdown --json    # same, always includes the itemized "breakdown" array
cashctl wallet balance --from work --json    # one wallet/token only: {"name","amount_mloki",...}
```

Sums every registered wallet's live `get_balance` result plus every
unredeemed held cash token's cached amount — the way a real wallet app
shows "your balance," not a protocol inventory. An unreachable wallet is
silently omitted from the sum (not fatal to the whole command) **except**
one declining with the NIP-47 `EXPIRED` code specifically, which falls
back to the last-known cached figure, marked `"stranded": true` — so an
expired wallet's balance is never just invisible, but is clearly flagged
as no longer money-moving.

## `cashctl wallet get-info` / `budget` / `invoice` / `pay` / `list-tx` / `sign-message`

Ordinary NIP-47 calls against whichever wallet is current (override with
`-c/--connection <name-or-raw-value>` for one call, without switching the
default):

```sh
cashctl wallet get-info --json
cashctl wallet budget --json
cashctl wallet invoice 5000 --desc "test" --json   # amount is mloki; prints {"invoice", "payment_hash", ...}
cashctl wallet pay lnbc1... --json
cashctl wallet list-tx --json
cashctl wallet sign-message "hello" --json
```

`cashctl invoice <amount>` / `cashctl pay <invoice>` are top-level shortcuts
for `wallet invoice`/`wallet pay` — identical flags and output, just
without the `wallet` prefix.

Any wallet decline surfaces as `code: "auth"` (restricted/unauthorized/
expired), `code: "conflict"` (rate-limited, retry), or `code: "internal"`
(insufficient balance, payment failed, or anything else) — check the
`nwc_code` field in `--json` output for the exact NIP-47 reason.
