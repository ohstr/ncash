# >_ ncash

[![Release](https://img.shields.io/github/v/release/ohstr/ncash)](https://github.com/ohstr/ncash/releases/latest)
[![CI](https://github.com/ohstr/ncash/actions/workflows/ci.yml/badge.svg)](https://github.com/ohstr/ncash/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/ohstr/ncash.svg)](https://pkg.go.dev/github.com/ohstr/ncash)
[![License: Unlicense](https://img.shields.io/badge/license-Unlicense-blue.svg)](LICENSE)

**A wallet CLI for [cash](https://github.com/flokiorg/lokihub/blob/main/docs/nips/NIP-CASH.md)
tokens on energy-backed coins with Lightning support: receive, hold, spend,
and consolidate them.**

**Join a [circle](https://github.com/flokiorg/lokihub/blob/main/docs/nips/NIP-CW.md)
to get a personal Lightning wallet.**

## Features

- [`ncash init`](#ncash-init) — Set up your identity
- [`ncash join`](#ncash-join) — Join a circle to get a personal Lightning wallet
- [`ncash wallet show/history/use`](#ncash-wallet-show) — Your identity, registered wallets, and local action history
- [`ncash wallet balance`](#ncash-wallet-balance) — Every wallet's live balance plus every held cash token's value, summed into one figure
- [`ncash wallet <op>`](#ncash-wallet-op) — get-info, budget, invoice, pay, list-tx, sign-message: ordinary NWC operations against whichever wallet is current
- [`ncash connect add/list/use/rm`](#ncash-connect-addlistuserm) — Register an NWC connection: a plain Lightning wallet you already have
- [`ncash receive`](#ncash-receive) — "Cash-in" a token: decode it and add it to your wallet
- [`ncash redeem`](#ncash-redeem) — Redeem a held token into a Lightning wallet
- [`ncash transfer`](#ncash-transfer) — Send a held token to someone else, in full or split
- [`ncash consolidate`](#ncash-consolidate) — Merge several held tokens into one
- [`ncash cash list-recipients/decode/verify-provenance`](#ncash-cash-list-recipientsdecodeverify-provenance) — Inspect a token, locally or against its Hub

`ncash` mints nothing itself — minting is the Hub operator's own tooling.

## Installation

**AI agents:**

```
Fetch https://ohstr.github.io/ncash/PROMPT.md
```

See [Agent skills](#agent-skills) below for what that does.

**macOS / Linux:**

```sh
curl -fsSL https://ohstr.github.io/ncash/install.sh | sh
```

Detects your OS and CPU (amd64/arm64) and installs to `/usr/local/bin`.
Falls back to `~/.local/bin` if that's not writable.

**Homebrew** (macOS/Linux):

```sh
brew install ohstr/tap/ncash
```

**Windows (PowerShell):**

```powershell
irm https://ohstr.github.io/ncash/install.ps1 | iex
```

**go install**:

```sh
go install github.com/ohstr/ncash@latest
```

**Docker** — no toolchain required:

```sh
# :latest tracks the newest release, :edge tracks main — see Docker section below.
docker run --rm ghcr.io/ohstr/ncash:latest --help
```

**From source** — see [Development](#development).

## `ncash init`

Set up your identity and first wallet.

```sh
ncash init
```

Reuses your [ncli](https://github.com/ohstr/ncli) vault identity if you have
one. Otherwise it generates a new identity just for `ncash`.

If you already have a Lightning wallet connection (NWC), `init` offers to
register it as your default. Run `init` again any time — it's idempotent,
and just reports where things stand.

**Note:** under `--json`, `init` always generates a fresh local identity
non-interactively and skips the wallet offer, since there's no way to paste
a connection string in that mode.

```sh
ncash init --json
# {
#   "npub": "npub1...",
#   "identity_source": "ncash-local",
#   "default_wallet": ""
# }
```

## `ncash join`

Join a circle to get a personal wallet.

```sh
ncash join --hub <circlehub1... or NWC URI> --max-amount 100000
```

The self-service entry point into a circle. Give it a Circle Hub's
connection — the `circlehub1...` string its operator gives out, or a raw NWC
URI if the Hub hasn't adopted the bech32 form yet.

`join` calls `create_circle_wallet` on your behalf and saves the resulting
wallet. If it's your first wallet, it also becomes your default.

```sh
ncash join --hub circlehub1... --max-amount 100000 --budget-renewal monthly
```

| Flag | Meaning |
|---|---|
| `--hub` (required) | the Circle Hub connection |
| `--max-amount` | requested spend cap, in mloki |
| `--expiry` | requested expiry duration (default: the Hub's own) |
| `--budget-renewal` | `daily`\|`weekly`\|`monthly`\|`yearly`\|`never` (default: the Hub's own) |
| `--as` | override credential (defaults to your local identity) |

`join` is a top-level shortcut for `ncash circle create`.

## `ncash wallet show`

Your identity, wallets, and history.

```sh
ncash wallet show      # identity, registered wallets, held tokens
ncash wallet history   # local action log (receive/redeem/transfer/...)
ncash wallet use <name>  # switch your default wallet (also: ncash connect use)
```

## `ncash wallet balance`

Your unified balance.

```sh
ncash wallet balance             # one number: every wallet + every held token, summed
ncash wallet balance --breakdown # itemized, per-wallet/per-token
ncash wallet balance --from work # just one wallet or held token
```

An expired wallet can't be queried live. `balance` falls back to the
last-known figure from your most recent successful check, marked `stranded`
(`[expired — money-moving disabled]` in text mode). That way an expired
wallet's money is never silently invisible.

## `ncash wallet <op>`

Ordinary NWC wallet operations. Plain [NIP-47](https://github.com/nostr-protocol/nips/blob/master/47.md)
calls against whichever wallet is current (`-c/--connection` overrides it
for one call):

```sh
ncash wallet get-info
ncash wallet budget
ncash wallet invoice 5000 --desc "coffee"
ncash wallet pay lnbc1...
ncash wallet list-tx
ncash wallet sign-message "hello"
```

`invoice` and `pay` are also available as top-level shortcuts: `ncash
invoice 5000` / `ncash pay lnbc1...`.

## `ncash connect add/list/use/rm`

Register a Lightning wallet you already have, over NWC — your own, or
one handed to you from another device:

```sh
ncash connect add work nostr+walletconnect://...
ncash connect list
ncash connect use work
ncash connect rm work
```

## `ncash receive`

"Cash-in" a token.

```sh
ncash receive lokicash1...
ncash receive lokicash1... --verify   # cross-check against the Hub via list_recipients
ncash receive lokicash1... --secret <bearer_secret>   # bearer-mode tokens only, see below
```

Decodes the token locally and records it in your wallet right away, marked
unverified. No network call, so it's instant.

If you paste a Circle Hub or Cash Hub connection here instead of a token,
`ncash` gives you a specific error pointing you to the right command.

**Bearer-mode tokens are two values, not one.** The `lokicash1...` string
only decodes the token and lists its recipients — it's never enough to
redeem or transfer a bearer slice.

The actual spending credential, `bearer_secret`, is minted once and handed
out separately by the Hub operator. Pass it with `--secret` when you
receive the token. If you skip it, the token is still saved, but
`redeem`/`transfer` will ask for `--as bearer:<secret>` before acting on it.

## `ncash redeem`

Redeem a held token into a Lightning wallet.

```sh
ncash redeem                          # auto-picks your one held token and default wallet
ncash redeem --token tok-a1b2 --to work
ncash redeem --invoice lnbc1...       # bypass both — redeem into any invoice, no ncash wallet needed
```

| Flag | Meaning |
|---|---|
| `--token` | which held token (auto-picked if you only hold one) |
| `--to` | destination wallet (default: your default wallet) |
| `--invoice` | redeem straight into this external invoice |
| `--as` | override credential — required for a connection-key-bound token |

## `ncash transfer`

Send a held token, in full or split.

```sh
ncash transfer --to pubkey:<hex>                      # transfer it all
ncash transfer --to pubkey:<hex> --split 3000          # keep the rest as a new token
ncash transfer --to bearer-target
ncash transfer --to connection:<platform>:<external-id>:<ia-pubkey>
```

## `ncash consolidate`

Merge several held tokens into one.

```sh
ncash consolidate --sources tok-a1b2,tok-c3d4
ncash consolidate --sources tok-a1b2,lokicash1...:5000:pubkey:<privkey> --to pubkey:<hex>
```

`--sources` takes bare local ledger IDs — amount and credential are already
known for each entry. Use the verbose `<token>:<amount>:<credential>` form
only for a source that isn't in your local ledger.

## `ncash cash list-recipients`/`decode`/`verify-provenance`

Inspect a token.

```sh
ncash cash list-recipients               # your allocation + co-recipients of a held token
ncash cash decode lokicash1...           # local-only, no network call
ncash cash verify-provenance lokicash1...  # check a mint-signature, locally
```

## Agent skills

For coding agents: [AGENTS.md](AGENTS.md) points to the matching skill
under [`skills/`](skills/) — one per command group. Each skill works
standalone with just the `ncash` binary on `PATH`.

```
npx skills add ohstr/ncash --all -y
```

`ncash --help` prints the complete command tree.

## Configuration

State lives under `$XDG_CONFIG_HOME/ncash` (or `~/.config/ncash` on
Linux/macOS): `identity.json`, `connections.json`, `ledger.json`. Override
the location with `--config-dir`.

If you point `init` at an [ncli](https://github.com/ohstr/ncli) vault,
`ncash` only reads it. Your vault stays at its own usual path, unaffected.

## Docker

```sh
docker run --rm -v ~/.config/ncash:/root/.config/ncash ghcr.io/ohstr/ncash:latest wallet show
```

The `:edge` tag tracks `main`; a versioned tag tracks that release.

## Development

Build from source with [`just`](https://github.com/casey/just):

```sh
just build   # go build -o ncash .
```

## License

[Unlicense](LICENSE) — public domain.
