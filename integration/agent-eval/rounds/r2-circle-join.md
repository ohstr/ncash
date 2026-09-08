# Round R2 — Joining a circle

`ncash` should already be installed. Use `ncash --help` / `ncash <cmd>
--help`, and re-fetch
`https://raw.githubusercontent.com/ohstr/ncash/main/AGENTS.md` if needed.

Joining a circle needs the Hub operator to authorize your specific
identity first — so this round has two parts, in order:

1. Make sure you have an identity: `ncash init --json`. Whatever npub it
   reports (or, if it says you're already configured, the npub from
   `ncash wallet show --json`), write **just that npub string, nothing
   else** to `/report/r2-npub.txt`.
2. Now wait for `/fixtures/r2-hub.txt` to appear — it won't exist yet.
   Poll for it (e.g. `sleep 3` in a loop, checking with `[ -f ... ]`) for
   up to a few minutes. Once it exists, it contains a Circle Hub
   connection string that has just been authorized for the npub you wrote
   in step 1.
3. Join it: `ncash join --hub "$(cat /fixtures/r2-hub.txt)" --max-amount
   100000 --yes`.
4. Confirm the resulting wallet is live: `ncash wallet get-info` and
   `ncash wallet show`.

Write your self-report to `/report/r2-circle-join.self-report.json`.
Include how long you had to wait in step 2, and the exact `join` output.
