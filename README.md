# >_ cashctl

[![Release](https://img.shields.io/github/v/release/ohstr/cashctl)](https://github.com/ohstr/cashctl/releases/latest)
[![CI](https://github.com/ohstr/cashctl/actions/workflows/ci.yml/badge.svg)](https://github.com/ohstr/cashctl/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/ohstr/cashctl.svg)](https://pkg.go.dev/github.com/ohstr/cashctl)
[![License: Unlicense](https://img.shields.io/badge/license-Unlicense-blue.svg)](LICENSE)

**A wallet CLI for [cash](https://github.com/flokiorg/lokihub/blob/main/docs/nips/NIP-CASH.md)
tokens on energy-backed coins with Lightning support: receive, hold, spend,
and consolidate them.**

**Join a [circle](https://github.com/flokiorg/lokihub/blob/main/docs/nips/NIP-CW.md)
to get a personal Lightning wallet.**

## Features

- [`cashctl init`](#cashctl-init) — Set up your identity
- [`cashctl join`](#cashctl-join) — Join a circle to get a personal Lightning wallet
- [`cashctl wallet show/history/use`](#cashctl-wallet-show) — Your identity, registered wallets, and local action history
- [`cashctl wallet balance`](#cashctl-wallet-balance) — Every wallet's live balance plus every held cash token's value, summed into one figure
- [`cashctl wallet <op>`](#cashctl-wallet-op) — get-info, budget, invoice, pay, list-tx, sign-message: ordinary NWC operations against whichever wallet is current
- [`cashctl connect add/list/use/rm`](#cashctl-connect-addlistuserm) — Register an NWC connection: a plain Lightning wallet you already have
- [`cashctl receive`](#cashctl-receive) — "Cash-in" a token: decode it and add it to your wallet
- [`cashctl redeem`](#cashctl-redeem) — Redeem a held token into a Lightning wallet
- [`cashctl transfer`](#cashctl-transfer) — Send a held token to someone else, in full or split
- [`cashctl consolidate`](#cashctl-consolidate) — Merge several held tokens into one
- [`cashctl cash list-recipients/decode/verify-provenance`](#cashctl-cash-list-recipientsdecodeverify-provenance) — Inspect a token, locally or against its Hub

`cashctl` mints nothing itself — minting is the Hub operator's own tooling.

## Installation

**AI agents:**

```
Fetch https://ohstr.github.io/cashctl/PROMPT.md
```

See [Agent skills](#agent-skills) below for what that does.

**macOS / Linux:**

```sh
curl -fsSL https://ohstr.github.io/cashctl/install.sh | sh
```

Detects your OS and CPU (amd64/arm64) and installs to `/usr/local/bin`.
Falls back to `~/.local/bin` if that's not writable.

**Homebrew** (macOS/Linux):

```sh
brew install ohstr/tap/cashctl
```

**Windows (PowerShell):**

```powershell
irm https://ohstr.github.io/cashctl/install.ps1 | iex
```

**go install**:

```sh
go install github.com/ohstr/cashctl@latest
```

**Docker** — no toolchain required:

```sh
# :latest tracks the newest release, :edge tracks main — see Docker section below.
docker run --rm ghcr.io/ohstr/cashctl:latest --help
```

**From source** — see [Development](#development).

## `cashctl init`

Set up your identity and first wallet.

```sh
cashctl init
```

Reuses your [ncli](https://github.com/ohstr/ncli) vault identity if you have
one. Otherwise it generates a new identity just for `cashctl`.

If you already have a Lightning wallet connection (NWC), `init` offers to
register it as your default. Run `init` again any time — it's idempotent,
and just reports where things stand.

**Note:** under `--json`, `init` always generates a fresh local identity
non-interactively and skips the wallet offer, since there's no way to paste
a connection string in that mode.

```sh
cashctl init --json
# {
#   "npub": "npub1...",
#   "identity_source": "cashctl-local",
#   "default_wallet": ""
# }
```

## `cashctl join`

Join a circle to get a personal wallet.

```sh
cashctl join --hub <circlehub1... or NWC URI> --max-amount 100000
```

The self-service entry point into a circle. Give it a Circle Hub's
connection — the `circlehub1...` string its operator gives out, or a raw NWC
URI if the Hub hasn't adopted the bech32 form yet.

`join` calls `create_circle_wallet` on your behalf and saves the resulting
wallet. If it's your first wallet, it also becomes your default.

```sh
cashctl join --hub circlehub1... --max-amount 100000 --budget-renewal monthly
```

| Flag | Meaning |
|---|---|
| `--hub` (required) | the Circle Hub connection |
| `--max-amount` | requested spend cap, in mloki |
| `--expiry` | requested expiry duration (default: the Hub's own) |
| `--budget-renewal` | `daily`\|`weekly`\|`monthly`\|`yearly`\|`never` (default: the Hub's own) |
| `--as` | override credential (defaults to your local identity) |

`join` is a top-level shortcut for `cashctl circle create`.

## `cashctl wallet show`

Your identity, wallets, and history.

```sh
cashctl wallet show      # identity, registered wallets, held tokens
cashctl wallet history   # local action log (receive/redeem/transfer/...)
cashctl wallet use <name>  # switch your default wallet (also: cashctl connect use)
```

## `cashctl wallet balance`

Your unified balance.

```sh
cashctl wallet balance             # one number: every wallet + every held token, summed
cashctl wallet balance --breakdown # itemized, per-wallet/per-token
cashctl wallet balance --from work # just one wallet or held token
```

An expired wallet can't be queried live. `balance` falls back to the
last-known figure from your most recent successful check, marked `stranded`
(`[expired — money-moving disabled]` in text mode). That way an expired
wallet's money is never silently invisible.

## `cashctl wallet <op>`

Ordinary NWC wallet operations. Plain [NIP-47](https://github.com/nostr-protocol/nips/blob/master/47.md)
calls against whichever wallet is current (`-c/--connection` overrides it
for one call):

```sh
cashctl wallet get-info
cashctl wallet budget
cashctl wallet invoice 5000 --desc "coffee"
cashctl wallet pay lnbc1...
cashctl wallet list-tx
cashctl wallet sign-message "hello"
```

`invoice` and `pay` are also available as top-level shortcuts: `cashctl
invoice 5000` / `cashctl pay lnbc1...`.

## `cashctl connect add/list/use/rm`

Register a Lightning wallet you already have, over NWC — your own, or
one handed to you from another device:

```sh
cashctl connect add work nostr+walletconnect://...
cashctl connect list
cashctl connect use work
cashctl connect rm work
```

## `cashctl receive`

"Cash-in" a token.

```sh
cashctl receive lokicash1...
cashctl receive lokicash1... --verify   # cross-check against the Hub via list_recipients
cashctl receive lokicash1... --secret <bearer_secret>   # bearer-mode tokens only, see below
```

Decodes the token locally and records it in your wallet right away, marked
unverified. No network call, so it's instant.

If you paste a Circle Hub or Cash Hub connection here instead of a token,
`cashctl` gives you a specific error pointing you to the right command.

**Bearer-mode tokens are two values, not one.** The `lokicash1...` string
only decodes the token and lists its recipients — it's never enough to
redeem or transfer a bearer slice.

The actual spending credential, `bearer_secret`, is minted once and handed
out separately by the Hub operator. Pass it with `--secret` when you
receive the token. If you skip it, the token is still saved, but
`redeem`/`transfer` will ask for `--as bearer:<secret>` before acting on it.

## `cashctl redeem`

Redeem a held token into a Lightning wallet.

```sh
cashctl redeem                          # auto-picks your one held token and default wallet
cashctl redeem --token tok-a1b2 --to work
cashctl redeem --invoice lnbc1...       # bypass both — redeem into any invoice, no cashctl wallet needed
```

| Flag | Meaning |
|---|---|
| `--token` | which held token (auto-picked if you only hold one) |
| `--to` | destination wallet (default: your default wallet) |
| `--invoice` | redeem straight into this external invoice |
| `--as` | override credential — required for a connection-key-bound token |

## `cashctl transfer`

Send a held token, in full or split.

```sh
cashctl transfer --to pubkey:<hex>                      # transfer it all
cashctl transfer --to pubkey:<hex> --split 3000          # keep the rest as a new token
cashctl transfer --to bearer-target
cashctl transfer --to connection:<platform>:<external-id>:<ia-pubkey>
```

## `cashctl consolidate`

Merge several held tokens into one.

```sh
cashctl consolidate --sources tok-a1b2,tok-c3d4
cashctl consolidate --sources tok-a1b2,lokicash1...:5000:pubkey:<privkey> --to pubkey:<hex>
```

`--sources` takes bare local ledger IDs — amount and credential are already
known for each entry. Use the verbose `<token>:<amount>:<credential>` form
only for a source that isn't in your local ledger.

## `cashctl cash list-recipients`/`decode`/`verify-provenance`

Inspect a token.

```sh
cashctl cash list-recipients               # your allocation + co-recipients of a held token
cashctl cash decode lokicash1...           # local-only, no network call
cashctl cash verify-provenance lokicash1...  # check a mint-signature, locally
```

## Agent skills

For coding agents: [AGENTS.md](AGENTS.md) points to the matching skill
under [`skills/`](skills/) — one per command group. Each skill works
standalone with just the `cashctl` binary on `PATH`.

```
npx skills add ohstr/cashctl --all -y
```

`cashctl --help` prints the complete command tree.

## Configuration

State lives under `$XDG_CONFIG_HOME/cashctl` (or `~/.config/cashctl` on
Linux/macOS): `identity.json`, `connections.json`, `ledger.json`. Override
the location with `--config-dir`.

If you point `init` at an [ncli](https://github.com/ohstr/ncli) vault,
`cashctl` only reads it. Your vault stays at its own usual path, unaffected.

## Docker

```sh
docker run --rm -v ~/.config/cashctl:/root/.config/cashctl ghcr.io/ohstr/cashctl:latest wallet show
```

The `:edge` tag tracks `main`; a versioned tag tracks that release.

## Development

Build from source with [`just`](https://github.com/casey/just):

```sh
just build   # go build -o cashctl .
```

## License

[Unlicense](LICENSE) — public domain.
