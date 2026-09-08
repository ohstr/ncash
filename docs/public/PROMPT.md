# ncash — agent bootstrap prompt

You've been pointed at this file (by a user, or by another agent) to
install and start using `ncash`, a wallet CLI for NIP-CASH cash tokens and
NIP-CW circle wallets, in a project that has no local copy of this repo.
Follow the steps in order.

## 1. Install

Skip this if it's already on `PATH`:

```sh
command -v ncash && ncash version
```

Otherwise, pick one for the current OS:

**macOS / Linux**

```sh
curl -fsSL https://ohstr.github.io/ncash/install.sh | sh
```

**Windows (PowerShell)**

```powershell
irm https://ohstr.github.io/ncash/install.ps1 | iex
```

**Homebrew (macOS/Linux)**

```sh
brew install ohstr/tap/ncash
```

**go install**

```sh
go install github.com/ohstr/ncash@latest
```

**Docker** (no toolchain required)

```sh
docker run --rm ghcr.io/ohstr/ncash:latest --help
```

## 2. Confirm it works

```sh
ncash version --json
ncash init --json
```

`init` generates a local identity (reusing an existing ncli vault entry
instead, if one is found) — no network call, no password needed for a
fresh local identity. Valid JSON back from both means the install is good.

## 3. Load the real reference

`ncash` joins circles for a personal Lightning wallet, and receives,
holds, spends, and consolidates NIP-CASH cash tokens — it never mints
anything itself.

Before running any real command, fetch the full command table and the
`--json`/error-code contract — it's the source of truth, don't guess at it
from this file:

```
https://raw.githubusercontent.com/ohstr/ncash/main/AGENTS.md
```

## 4. Pull in task-specific skills

For example-driven guidance beyond `--help`, install the skills instead of
re-deriving them:

```sh
npx skills add ohstr/ncash --all -y
```

AGENTS.md (step 3) points to which one to read for a given task.
