# Round R1 — Cash lifecycle

`ncash` should already be installed (an earlier round did this; if it
somehow isn't, install it the same way
`https://ohstr.github.io/ncash/PROMPT.md` describes before continuing).
Use `ncash --help` / `ncash <cmd> --help`, and re-fetch
`https://raw.githubusercontent.com/ohstr/ncash/main/AGENTS.md` if you need
the command/flag contract again.

You've been handed two fixtures, mounted read-only:
- `/fixtures/r1-cash-token.txt` — a real NIP-CASH token
- `/fixtures/r1-wallet-uri.txt` — a plain Lightning wallet connection (NWC)
  you can redeem into

1. Make sure you have an identity: `ncash init --json` (fine to re-run if
   an earlier round already did this — it should say so, not error).
2. Inspect the token **without** touching your wallet yet:
   `ncash cash decode "$(cat /fixtures/r1-cash-token.txt)"`.
3. Now actually receive it into your wallet, with verification:
   `ncash receive "$(cat /fixtures/r1-cash-token.txt)" --verify`.
4. Check its co-recipients: `ncash cash list-recipients`.
5. Register the wallet fixture as a named connection called `payout`:
   `ncash connect add payout "$(cat /fixtures/r1-wallet-uri.txt)"`.
6. Redeem the token you received in step 3 into `payout`:
   `ncash redeem --to payout --yes`.
7. Confirm it: `ncash wallet balance --breakdown` and `ncash wallet
   history` — the token should no longer show up as a held token, and the
   history should record the redeem.

Write your self-report to `/report/r1-cash-lifecycle.self-report.json`.
Include the exact `redeem` output (it should contain a Lightning payment
preimage on success) and whether `wallet balance --breakdown` matched what
you expected after redeeming.
