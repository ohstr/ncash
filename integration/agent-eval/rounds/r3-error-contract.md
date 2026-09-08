# Round R3 — Error contract

`ncash` should already be installed. Fetch
`https://raw.githubusercontent.com/ohstr/ncash/main/AGENTS.md` and read
its "Output conventions" section carefully before starting — it documents
exactly what every command's failure should look like.

This round needs no network access and no fixtures. For **each** of the
commands below, run it exactly as given (all under `--json`), and record
its exit code and its exact stderr JSON:

1. `ncash bogus-command --json` — a command that doesn't exist
2. `ncash wallet show --bogus-flag --json` — a flag that doesn't exist
3. `ncash join --json` — a required flag (`--hub`) missing
4. `ncash transfer --json` — a required flag (`--to`) missing
5. `ncash cash decode not-a-valid-token --json` — a value that fails to parse
6. `ncash redeem --json` — a legitimate "nothing to act on" case (assuming
   you hold no cash tokens in this fresh environment)
7. `ncash wallet balance --from does-not-exist --json` — a named
   thing that doesn't exist

For each one, check: does the exit code match AGENTS.md's table for the
`code` field in the JSON output? Is the JSON shape exactly `{"error",
"code", "retryable", "input"?}` (no extra top-level keys, nothing on
stdout)? Does `retryable` match the table? Flag any mismatch as an issue
in your self-report — don't just record the raw output uncritically.

Write your self-report to `/report/r3-error-contract.self-report.json`,
with one entry in `steps` per command above (put the exit code in
`exit_code` and the raw stderr JSON in `result`), and list every contract
mismatch you found in `issues` (empty array if none).
