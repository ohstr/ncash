//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/ohstr/nmilat/nip47"
	"github.com/ohstr/nmilat/nipcash"
	nipcashclient "github.com/ohstr/nmilat/nipcash/client"
	relayclient "github.com/ohstr/nmilat/relay/client"
)

const ephemeralFixtureNamePrefix = "ncash integration"

// setUpCashHub provisions a throwaway, self-funded cash_hub (with
// make_invoice/pay_invoice/get_balance so it can also act as a redeem
// destination in these tests), registers cleanup, and returns the admin
// API's own response (its PairingUri dials as either a nipcash/client.Client
// for the NIP-CASH-specific calls, or a plain relayclient.NWCClient for
// ordinary NIP-47 calls like make_invoice).
func setUpCashHub(t *testing.T, admin *adminClient) adminCreateAppResponse {
	t.Helper()
	resp, err := admin.createApp(adminCreateAppRequest{
		Name: ephemeralFixtureNamePrefix + " cash_hub",
		Kind: "cash_hub",
		// Scope string literals mirror lokihub's own constants/constants.go
		// (cash_hub, pay_invoice, make_invoice, get_balance) — this suite
		// stays a black-box HTTP/NWC client, so it names them directly
		// rather than importing lokihub's internal constants package.
		Scopes:                []string{"cash_hub", "pay_invoice", "make_invoice", "get_balance"},
		CashPerWalletMaxMloki: 10_000_000,
		CashMaxExpSecs:        3600,
	})
	if err != nil {
		t.Fatalf("create ephemeral cash_hub: %v", err)
	}
	// Registered before the reclaim sweep below so it runs second
	// (t.Cleanup is LIFO): every cash_wallet child this hub ever mints
	// must be reclaimed before the hub itself can be deleted.
	t.Cleanup(func() {
		if err := admin.deleteApp(resp.ID); err != nil {
			t.Logf("cleanup: delete ephemeral cash_hub app_id=%d: %v", resp.ID, err)
		}
	})
	t.Cleanup(func() {
		claims, err := admin.listCashWalletClaims(resp.ID)
		if err != nil {
			t.Logf("cleanup: list cash wallet children for ephemeral hub app_id=%d: %v", resp.ID, err)
			return
		}
		seen := map[uint]bool{}
		for _, claim := range claims {
			if seen[claim.WalletAppID] {
				continue
			}
			seen[claim.WalletAppID] = true
			if err := admin.deleteCashWallet(resp.ID, claim.WalletAppID); err != nil {
				t.Logf("cleanup: delete ephemeral cash wallet child app_id=%d: %v", claim.WalletAppID, err)
			}
		}
	})
	if err := admin.transfer(resp.ID, 2000); err != nil {
		t.Fatalf("fund ephemeral cash_hub: %v", err)
	}
	return resp
}

// dialCash dials hubPairingURI as a NIP-CASH client (mint_cash, cash_redeem,
// list_recipients, ...).
func dialCash(t *testing.T, ctx context.Context, hubPairingURI string) *nipcashclient.Client {
	t.Helper()
	c, err := nipcashclient.Connect(ctx, hubPairingURI)
	if err != nil {
		t.Fatalf("dial cash hub (nipcash): %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

// dialNWC dials hubPairingURI as a plain NIP-47 client (make_invoice,
// get_balance, ...) — the same connection, a different call surface.
func dialNWC(t *testing.T, ctx context.Context, hubPairingURI string) *relayclient.NWCClient {
	t.Helper()
	pairing, err := nip47.ParsePairingURI(hubPairingURI)
	if err != nil {
		t.Fatalf("parse hub pairing URI: %v", err)
	}
	c, err := relayclient.NewNWCClient(ctx, pairing, nip47.EncryptionNIP44V2)
	if err != nil {
		t.Fatalf("dial cash hub (NWC): %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

// TestCashLifecycle_MintReceiveRedeem mints a real cash token addressed to
// ncash's freshly generated local identity via a live cash_hub, receives it
// through the compiled ncash binary, and redeems it into an invoice from
// the same hub — proving the full mint -> receive -> redeem round trip
// against a real, running lokihub instance end to end.
func TestCashLifecycle_MintReceiveRedeem(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Skipf("skipping: could not load integration config (%v) — see integration/README.md", err)
	}
	admin, ok := newAdminClient(cfg)
	if !ok {
		t.Skip("skipping: admin_api not configured — see integration/README.md")
	}

	f := newFixture(t)
	initResp := f.mustJSON("wallet", "init")
	npub, _ := initResp["npub"].(string)
	if npub == "" {
		t.Fatalf("wallet init: no npub in response: %v", initResp)
	}
	pubHex, err := npubToHex(npub)
	if err != nil {
		t.Fatalf("decode local identity npub: %v", err)
	}

	hub := setUpCashHub(t, admin)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cashClient := dialCash(t, ctx, hub.PairingUri)

	const amountMillis = uint64(500_000) // 500 mloki
	mintResult, err := cashClient.MintCash(ctx, nipcash.MintCashParams{
		Recipients: []nipcash.Allocation{nipcash.Send(nipcash.Pubkey(pubHex), amountMillis)},
	})
	if err != nil {
		t.Fatalf("mint_cash: %v", err)
	}
	if mintResult.CashToken == "" {
		t.Fatalf("mint_cash: empty cash token in result: %+v", mintResult)
	}

	receiveResp := f.mustJSON("receive", mintResult.CashToken, "--verify")
	entry, _ := receiveResp["entry"].(map[string]any)
	if entry == nil {
		t.Fatalf("receive: no entry in response: %v", receiveResp)
	}
	if got, _ := entry["amount_millis"].(float64); uint64(got) != amountMillis {
		t.Errorf("receive --verify: amount_millis = %v, want %d", entry["amount_millis"], amountMillis)
	}
	if verified, _ := entry["verified"].(bool); !verified {
		t.Errorf("receive --verify: entry not marked verified: %v", entry)
	}

	nwcClient := dialNWC(t, ctx, hub.PairingUri)
	invoiceTx, err := nwcClient.MakeInvoice(ctx, nip47.MakeInvoiceParams{Amount: int64(amountMillis), Description: "ncash integration redeem"})
	if err != nil {
		t.Fatalf("make_invoice: %v", err)
	}

	redeemResp := f.mustJSON("redeem", "--invoice", invoiceTx.Invoice, "--yes")
	if preimage, _ := redeemResp["preimage"].(string); preimage == "" {
		t.Errorf("redeem: empty preimage in response (invoice may not have been paid): %v", redeemResp)
	}
}

// TestCashInspect_DecodeAndListRecipients mints a bearer cash token, then
// exercises ncash's local-only decode and network-backed list-recipients
// commands against it — the two read-only "inspect a token" entry points
// (ncash-plan.md's Cash command tree) that TestCashLifecycle_* doesn't
// otherwise cover.
func TestCashInspect_DecodeAndListRecipients(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Skipf("skipping: could not load integration config (%v) — see integration/README.md", err)
	}
	admin, ok := newAdminClient(cfg)
	if !ok {
		t.Skip("skipping: admin_api not configured — see integration/README.md")
	}

	f := newFixture(t)
	f.mustJSON("wallet", "init")

	hub := setUpCashHub(t, admin)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cashClient := dialCash(t, ctx, hub.PairingUri)

	const amountMillis = uint64(250_000)
	mintBearer := func() (token, secret string) {
		t.Helper()
		result, err := cashClient.MintCash(ctx, nipcash.MintCashParams{
			Recipients: []nipcash.Allocation{nipcash.Send(nipcash.Anyone(), amountMillis)},
		})
		if err != nil {
			t.Fatalf("mint_cash (bearer): %v", err)
		}
		if len(result.Recipients) != 1 || result.Recipients[0].BearerSecret == "" {
			t.Fatalf("mint_cash (bearer): expected exactly one recipient with a bearer_secret: %+v", result.Recipients)
		}
		return result.CashToken, result.Recipients[0].BearerSecret
	}

	// Two separate bearer mints: a token can only ever be received once per
	// ledger (ledger.ErrAlreadyHeld), so the no-secret negative-path check
	// below needs its own token, distinct from the one actually redeemed.
	noSecretToken, _ := mintBearer()
	token, bearerSecret := mintBearer()

	decodeResp := f.mustJSON("cash", "decode", token)
	if wp, _ := decodeResp["wallet_pubkey"].(string); wp == "" {
		t.Errorf("cash decode: empty wallet_pubkey: %v", decodeResp)
	}

	entryIDFor := func(tok string) string {
		t.Helper()
		showResp := f.mustJSON("wallet", "show")
		held, _ := showResp["held_tokens"].([]any)
		for _, h := range held {
			entry, _ := h.(map[string]any)
			if entry["token"] == tok {
				id, _ := entry["id"].(string)
				return id
			}
		}
		t.Fatalf("wallet show: couldn't find an entry for token %q: %v", tok, held)
		return ""
	}

	// The token's own connection secret is NEVER a valid bearer credential
	// (NIP-CASH.md's Redemption Metadata section) — receiving without
	// --secret must still succeed (best-effort, same as any other mode),
	// but redeeming it afterward must fail asking for one explicitly. It's
	// still held afterward (the redeem attempt was rejected, not
	// consumed), so every later --token reference in this test must stay
	// explicit — two tokens are held from here on, never just one.
	if res := f.run("receive", noSecretToken); res.ExitCode != 0 {
		t.Fatalf("receive (no --secret): exit %d\nstderr: %s", res.ExitCode, res.Stderr)
	}
	noSecretID := entryIDFor(noSecretToken)
	noSecretRedeem := f.run("redeem", "--token", noSecretID, "--yes")
	if noSecretRedeem.ExitCode != 2 {
		t.Fatalf("redeem with no captured bearer secret: exit = %d, want 2 (usage)\nstderr: %s", noSecretRedeem.ExitCode, noSecretRedeem.Stderr)
	}
	if !jsonErrorContains(t, noSecretRedeem.Stderr, "usage", "--as bearer:") {
		t.Errorf("unexpected error body: %s", noSecretRedeem.Stderr)
	}

	// Now the real flow: receive WITH --secret, mirroring how a Hub
	// operator actually hands out a bearer note (token + bearer_secret,
	// conveyed together, out of band — see lokihub's own
	// ConnectAppCard/RevealConnectionDialog for the reference UX).
	if res := f.run("receive", token, "--secret", bearerSecret); res.ExitCode != 0 {
		t.Fatalf("receive --secret: exit %d\nstderr: %s", res.ExitCode, res.Stderr)
	}
	tokenID := entryIDFor(token)

	listResp := f.mustJSON("cash", "list-recipients", "--token", tokenID)
	recipients, _ := listResp["recipients"].([]any)
	if len(recipients) == 0 {
		t.Fatalf("list-recipients: no recipients returned: %v", listResp)
	}

	// Redeeming now needs no --as override: --secret at receive time
	// already captured the real spending credential.
	hubClient := dialNWC(t, ctx, hub.PairingUri)
	invoiceTx, err := hubClient.MakeInvoice(ctx, nip47.MakeInvoiceParams{Amount: int64(amountMillis)})
	if err != nil {
		t.Fatalf("make_invoice: %v", err)
	}
	redeemResp := f.mustJSON("redeem", "--token", tokenID, "--invoice", invoiceTx.Invoice, "--yes")
	if preimage, _ := redeemResp["preimage"].(string); preimage == "" {
		t.Errorf("redeem (bearer mode): empty preimage: %v", redeemResp)
	}
}

// TestRedeem_NoWalletConfigured mints a real, live-reachable token (so the
// redeem attempt gets past resolveHeldToken/resolveCredential/the source
// dial/resolveAmount and actually reaches resolveDestWallet) and confirms
// that with no wallet ever registered in this fixture, redeem fails with
// the documented noWalletConfiguredMsg — not a fake token with an
// unreachable relay, which would misleadingly fail earlier on the source
// dial instead and never prove this specific path at all.
func TestRedeem_NoWalletConfigured(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Skipf("skipping: could not load integration config (%v) — see integration/README.md", err)
	}
	admin, ok := newAdminClient(cfg)
	if !ok {
		t.Skip("skipping: admin_api not configured — see integration/README.md")
	}

	f := newFixture(t)
	initResp := f.mustJSON("wallet", "init")
	npub, _ := initResp["npub"].(string)
	myPubHex, err := npubToHex(npub)
	if err != nil {
		t.Fatalf("decode local identity npub: %v", err)
	}

	// pubkey mode, deliberately: this test is isolating resolveDestWallet's
	// own not_found path specifically, which sits behind resolveCredential
	// in runCashRedeem — a bearer-mode token without --secret would trip
	// resolveCredential's own (correct, and separately tested by
	// TestCashInspect_DecodeAndListRecipients) usage error first, never
	// reaching the code path this test exists to check.
	token := mintPubkeyToken(t, admin, myPubHex, 50_000)
	if res := f.run("receive", token, "--verify"); res.ExitCode != 0 {
		t.Fatalf("receive: exit %d\nstderr: %s", res.ExitCode, res.Stderr)
	}

	res := f.run("redeem")
	if res.ExitCode != 4 {
		t.Fatalf("redeem with no wallet configured: exit = %d, want 4 (not_found)\nstderr: %s", res.ExitCode, res.Stderr)
	}
	if !jsonErrorContains(t, res.Stderr, "not_found", "no wallet configured yet") {
		t.Errorf("unexpected error body: %s", res.Stderr)
	}
}

// TestRedeem_InvoiceFlagBypassesNoWalletCheck confirms --invoice skips
// resolveDestWallet entirely: against the same no-wallet-registered
// fixture as above, passing a syntactically-invalid invoice should fail
// on the Hub's own decline of that invoice, never on "no wallet
// configured" — proving the bypass actually happens, not just that some
// other error occurred first.
func TestRedeem_InvoiceFlagBypassesNoWalletCheck(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Skipf("skipping: could not load integration config (%v) — see integration/README.md", err)
	}
	admin, ok := newAdminClient(cfg)
	if !ok {
		t.Skip("skipping: admin_api not configured — see integration/README.md")
	}

	f := newFixture(t)
	initResp := f.mustJSON("wallet", "init")
	npub, _ := initResp["npub"].(string)
	myPubHex, err := npubToHex(npub)
	if err != nil {
		t.Fatalf("decode local identity npub: %v", err)
	}

	// pubkey mode, deliberately — see TestRedeem_NoWalletConfigured's own
	// comment: a bearer token without --secret would trip
	// resolveCredential's usage error before ever reaching the
	// resolveDestWallet bypass this test exists to check.
	token := mintPubkeyToken(t, admin, myPubHex, 50_000)
	if res := f.run("receive", token, "--verify"); res.ExitCode != 0 {
		t.Fatalf("receive: exit %d\nstderr: %s", res.ExitCode, res.Stderr)
	}

	res := f.run("redeem", "--invoice", "lnbc1notarealinvoice", "--yes")
	if res.ExitCode == 4 {
		t.Fatalf("redeem --invoice: got not_found — the no-wallet-configured check was NOT bypassed\nstderr: %s", res.Stderr)
	}
}

// mintPubkeyToken mints a pubkey-mode cash token addressed to pubkeyHex,
// via a fresh ephemeral cash_hub, and returns the token string.
func mintPubkeyToken(t *testing.T, admin *adminClient, pubkeyHex string, amountMillis uint64) string {
	t.Helper()
	hub := setUpCashHub(t, admin)
	return mintPubkeyTokenFromHub(t, hub, pubkeyHex, amountMillis)
}

// mintPubkeyTokenFromHub mints from an already-provisioned hub — needed
// whenever a test requires multiple tokens to be children of the SAME Cash
// Hub (e.g. cash_consolidate, which rejects sources from different hubs
// per NIP-CASH §Consolidating Tokens step 2).
func mintPubkeyTokenFromHub(t *testing.T, hub adminCreateAppResponse, pubkeyHex string, amountMillis uint64) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cashClient := dialCash(t, ctx, hub.PairingUri)
	result, err := cashClient.MintCash(ctx, nipcash.MintCashParams{
		Recipients: []nipcash.Allocation{nipcash.Send(nipcash.Pubkey(pubkeyHex), amountMillis)},
	})
	if err != nil {
		t.Fatalf("mint_cash: %v", err)
	}
	return result.CashToken
}

// TestCashTransfer_Full receives a real pubkey-mode token, transfers it in
// full to a second (uncontrolled) identity, and independently verifies the
// transfer landed server-side by dialing the resulting new token directly
// and calling list_recipients on it — not by trusting ncash transfer's own
// reported success.
func TestCashTransfer_Full(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Skipf("skipping: could not load integration config (%v) — see integration/README.md", err)
	}
	admin, ok := newAdminClient(cfg)
	if !ok {
		t.Skip("skipping: admin_api not configured — see integration/README.md")
	}

	f := newFixture(t)
	initResp := f.mustJSON("wallet", "init")
	npub, _ := initResp["npub"].(string)
	myPubHex, err := npubToHex(npub)
	if err != nil {
		t.Fatalf("decode local identity npub: %v", err)
	}

	const amountMillis = uint64(400_000)
	token := mintPubkeyToken(t, admin, myPubHex, amountMillis)
	if res := f.run("receive", token, "--verify"); res.ExitCode != 0 {
		t.Fatalf("receive --verify: exit %d\nstderr: %s", res.ExitCode, res.Stderr)
	}

	targetHex := fakeHex32(t)
	transferResp := f.mustJSON("transfer", "--to", "pubkey:"+targetHex, "--yes")
	if transferResp["remaining_amount_millis"] != nil {
		t.Errorf("transfer (full, no --split): expected no remaining_amount_millis, got %v", transferResp["remaining_amount_millis"])
	}

	// A full transfer to a pubkey/connection-key target is an IN-PLACE
	// identity reassignment (NIP-CASH §Transferring and Splitting a Slice:
	// "pubkey or connection_key -> reassigned in place: same wallet, same
	// connection") — no new wallet is minted, so new_wallet_token/
	// new_wallet_pubkey are correctly empty here (only a bearer target, or
	// a multi-recipient-history wallet, spins off a new one). Verify via
	// the ORIGINAL token's own connection instead.
	if nwt, _ := transferResp["new_wallet_token"].(string); nwt != "" {
		t.Errorf("transfer (full, pubkey target): expected an in-place reassignment (empty new_wallet_token), got %q", nwt)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	verifyClient, err := nipcashclient.Connect(ctx, token)
	if err != nil {
		t.Fatalf("dial the original token's connection: %v", err)
	}
	defer verifyClient.Close()
	recipients, err := verifyClient.ListRecipients(ctx)
	if err != nil {
		t.Fatalf("list_recipients on the original (now-reassigned) connection: %v", err)
	}
	found := false
	for _, r := range recipients.Recipients {
		if r.IdentityType == "pubkey" && r.IdentityValue == targetHex {
			found = true
			if r.AmountMillis != amountMillis {
				t.Errorf("transferred amount = %d, want %d", r.AmountMillis, amountMillis)
			}
		}
	}
	if !found {
		t.Errorf("original connection's recipients don't reflect the reassignment to %s: %+v", targetHex, recipients.Recipients)
	}
}

// TestCashTransfer_Split transfers part of a held token and confirms the
// remainder was saved back into the local wallet with the correct amount —
// checked via `wallet show`, independent of the transfer command's own
// reported success.
func TestCashTransfer_Split(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Skipf("skipping: could not load integration config (%v) — see integration/README.md", err)
	}
	admin, ok := newAdminClient(cfg)
	if !ok {
		t.Skip("skipping: admin_api not configured — see integration/README.md")
	}

	f := newFixture(t)
	initResp := f.mustJSON("wallet", "init")
	npub, _ := initResp["npub"].(string)
	myPubHex, err := npubToHex(npub)
	if err != nil {
		t.Fatalf("decode local identity npub: %v", err)
	}

	const totalMillis = uint64(500_000)
	const splitMillis = uint64(200_000)
	const remainderMillis = totalMillis - splitMillis

	token := mintPubkeyToken(t, admin, myPubHex, totalMillis)
	if res := f.run("receive", token, "--verify"); res.ExitCode != 0 {
		t.Fatalf("receive --verify: exit %d\nstderr: %s", res.ExitCode, res.Stderr)
	}

	targetHex := fakeHex32(t)
	transferResp := f.mustJSON("transfer", "--to", "pubkey:"+targetHex, "--split", "200000", "--yes")
	remaining, _ := transferResp["remaining_amount_millis"].(float64)
	if uint64(remaining) != remainderMillis {
		t.Errorf("remaining_amount_millis = %v, want %d", transferResp["remaining_amount_millis"], remainderMillis)
	}

	showResp := f.mustJSON("wallet", "show")
	held, _ := showResp["held_tokens"].([]any)
	foundRemainder := false
	for _, h := range held {
		entry, _ := h.(map[string]any)
		if amt, _ := entry["amount_millis"].(float64); uint64(amt) == remainderMillis {
			foundRemainder = true
		}
	}
	if !foundRemainder {
		t.Errorf("wallet show: no held token with the expected remainder amount %d: %v", remainderMillis, held)
	}
}

// TestCashConsolidate merges two held tokens (minted to the same local
// identity) into one, and independently verifies the merged total via
// list_recipients on the new token.
func TestCashConsolidate(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Skipf("skipping: could not load integration config (%v) — see integration/README.md", err)
	}
	admin, ok := newAdminClient(cfg)
	if !ok {
		t.Skip("skipping: admin_api not configured — see integration/README.md")
	}

	f := newFixture(t)
	initResp := f.mustJSON("wallet", "init")
	npub, _ := initResp["npub"].(string)
	myPubHex, err := npubToHex(npub)
	if err != nil {
		t.Fatalf("decode local identity npub: %v", err)
	}

	const amount1 = uint64(150_000)
	const amount2 = uint64(250_000)
	// Both sources MUST be children of the same Cash Hub (NIP-CASH
	// §Consolidating Tokens step 2 rejects otherwise) — mint both from one
	// shared hub, not mintPubkeyToken's own one-hub-per-call default.
	hub := setUpCashHub(t, admin)
	token1 := mintPubkeyTokenFromHub(t, hub, myPubHex, amount1)
	token2 := mintPubkeyTokenFromHub(t, hub, myPubHex, amount2)

	receive1 := f.mustJSON("receive", token1, "--verify")
	receive2 := f.mustJSON("receive", token2, "--verify")
	entry1, _ := receive1["entry"].(map[string]any)
	entry2, _ := receive2["entry"].(map[string]any)
	id1, _ := entry1["id"].(string)
	id2, _ := entry2["id"].(string)
	if id1 == "" || id2 == "" {
		t.Fatalf("receive: missing entry id(s): %v / %v", receive1, receive2)
	}

	consolidateResp := f.mustJSON("consolidate", "--sources", id1+","+id2, "--yes")
	newEntry, _ := consolidateResp["new_entry"].(map[string]any)
	newToken, _ := newEntry["token"].(string)
	if newToken == "" {
		t.Fatalf("consolidate: no new_entry.token in response: %v", consolidateResp)
	}
	if amt, _ := newEntry["amount_millis"].(float64); uint64(amt) != amount1+amount2 {
		t.Errorf("consolidate: new_entry.amount_millis = %v, want %d", newEntry["amount_millis"], amount1+amount2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	verifyClient, err := nipcashclient.Connect(ctx, newToken)
	if err != nil {
		t.Fatalf("dial the consolidated token: %v", err)
	}
	defer verifyClient.Close()
	recipients, err := verifyClient.ListRecipients(ctx)
	if err != nil {
		t.Fatalf("list_recipients on consolidated token: %v", err)
	}
	var total uint64
	for _, r := range recipients.Recipients {
		total += r.AmountMillis
	}
	if total != amount1+amount2 {
		t.Errorf("consolidated token's total = %d, want %d", total, amount1+amount2)
	}
}
