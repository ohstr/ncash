# ncash

`ncash` is a Go CLI wallet for [NIP-CASH](https://github.com/flokiorg/lokihub/blob/main/docs/nips/NIP-CASH.md)
cash tokens and [NIP-CW](https://github.com/flokiorg/lokihub/blob/main/docs/nips/NIP-CW.md)
circle wallets — for someone who doesn't run a Hub or node themselves.
Assume the `ncash` binary is already on `PATH`. State (identity, registered
wallets, held tokens) lives under `$XDG_CONFIG_HOME/ncash`, overridable with
`--config-dir`.

## Commands

| Command | Purpose |
|---|---|
| `ncash init` | Set up your identity (reusing an ncli vault entry if you have one) and optionally a default wallet |
| `ncash join --hub <connection>` | Join a circle via its Circle Hub connection (`circlehub1...` or a raw NWC URI), creating a personal wallet |
| `ncash circle create --hub <connection>` | Same as `join` — the canonical, fully-namespaced form |
| `ncash wallet show` | Your identity, registered wallets, and held cash tokens |
| `ncash wallet history` | Local action log (receive/redeem/transfer/consolidate) |
| `ncash wallet use <name>` / `ncash connect use <name>` | Switch your default wallet |
| `ncash wallet balance [--breakdown] [--from <name>]` | Unified balance: every wallet's live balance + every held token's value |
| `ncash wallet get-info` / `budget` / `invoice <amount>` / `pay <invoice>` / `list-tx` / `sign-message <msg>` | Ordinary NIP-47 calls against the current wallet |
| `ncash invoice <amount>` / `ncash pay <invoice>` | Top-level shortcuts for `wallet invoice`/`wallet pay` |
| `ncash connect add <name> <connection>` / `list` / `rm <name>` | Register/list/remove any other NWC connection |
| `ncash receive <token> [--verify] [--secret <bearer_secret>]` | Decode a cash token locally and add it to your wallet; `--verify` cross-checks it against the Hub; `--secret` captures a bearer-mode token's spending credential (see below) |
| `ncash redeem [--token <id>] [--to <name>] [--invoice <bolt11>] [--as <credential>]` | Redeem a held token into a wallet or a raw invoice |
| `ncash transfer --to <target> [--split <mloki>] [--as <credential>]` | Send a held token, in full or split |
| `ncash consolidate --sources <ids-or-verbose> [--to <target>]` | Merge several held tokens into one |
| `ncash cash list-recipients [--token <id>]` | Your allocation + co-recipients of a held token (network) |
| `ncash cash decode <token>` | Inspect a token locally, no network call |
| `ncash cash verify-provenance <token>` | Verify a token's mint-signature, locally |
| `ncash version` | Print the ncash version |

Credential/target flag syntax (`--as`, `--to`): `pubkey:<hex-or-privkey>`,
`bearer:<secret>` / `bearer-target`, `connection-key:<privkey>,<platform>,
<external-id>,<attestation-file>` (redeem/transfer only), `connection:
<platform>:<external-id>:<ia-pubkey>` (transfer `--to` only).

## Output conventions

Every command's result goes to **stdout only**, always as a single JSON
document under `--json`; progress narration and errors go to **stderr**
always, never stdout — a script parsing stdout never has to distinguish a
success shape from a failure shape on the same stream. `--json`, `-c/
--connection`, `--yes`, and `--config-dir` are global flags declared once
on the root command. `--yes` (or `--json`, which implies it) skips
confirmation prompts. Every command is JSON-only-on-request (human text by
default, `--json` for the machine shape) — there is no command that is
JSON-only always.

**Failures**: exactly one top-level error report, always on stderr — a
plain `Error: ...` line by default, or `{"error", "code", "retryable",
"input"?, "nwc_code"?}` with `--json`:

| `code` | exit | retryable | meaning |
|---|---|---|---|
| `usage` | 2 | no | bad/missing/conflicting flags or args, or a group command invoked without a subcommand |
| `invalid_input` | 3 | no | a supplied value failed validation/parsing (bad token, connection string, credential syntax, amount, ...) |
| `not_found` | 4 | no | the referenced thing doesn't exist (held token, registered wallet, vault entry, ...) — includes "no wallet configured yet" |
| `conflict` | 5 | yes | collides with existing state (a token already held, a wallet's rate limit) |
| `network` | 6 | yes | couldn't reach a relay/wallet |
| `auth` | 7 | no | not authorized, or no longer (a wallet declined as restricted/unauthorized/expired) |
| `internal` | 1 | no | anything else — a wallet-side decline that isn't one of the above, or an ncash-side failure |

`input`, when present, is the single specific value that caused the
failure — **never** raw secret material: an `nsec1...`-shaped or bare
64-hex-char value is redacted to `""`, and a `pubkey:<privkey>`/`bearer:
<secret>`/`connection-key:<privkey>,...` credential string has just its
secret component blanked (`pubkey:<redacted>`, etc.), keeping the rest of
the string legible in the error. `retryable` lets an agent decide whether
to back off and retry (`conflict`/`network`) or fix the input and try
again (everything else) without string-matching the message. `nwc_code`,
when present, is the raw NIP-47 error code (`RESTRICTED`, `EXPIRED`,
`INSUFFICIENT_BALANCE`, ...) a wallet returned — ncash's own 7-code table
is deliberately coarse, so this is there for an agent that needs
finer-grained branching. A usage mistake in `--json` mode skips the
human-readable help dump (which would otherwise land on stdout) in favor
of the structured error alone.

## Before attempting a task, read the matching skill

This repo ships example-driven guidance in `skills/`, one file per area:

- Setting up an identity, managing wallets, or making ordinary Lightning
  calls (`init`, `wallet ...`, `connect ...`) → `skills/ncash-wallet/SKILL.md`
- Receiving, redeeming, transferring, or consolidating NIP-CASH tokens
  (`receive`, `redeem`, `transfer`, `consolidate`, `cash ...`) →
  `skills/ncash-cash/SKILL.md`
- Joining a circle for a personal wallet (`join`, `circle create`) →
  `skills/ncash-circle/SKILL.md`
