package output

import (
	"errors"
	"testing"

	relayclient "github.com/ohstr/nmilat/relay/client"
	"github.com/spf13/cobra"
)

func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool("json", false, "")
	return cmd
}

func TestExitCode_EachClassifierMapsToItsOwnCode(t *testing.T) {
	cmd := newTestCmd()
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"usage", UsageError(cmd, errors.New("bad")), 2},
		{"invalid_input", InvalidInputError(cmd, "x", errors.New("bad")), 3},
		{"not_found", NotFoundError(cmd, "x", errors.New("bad")), 4},
		{"conflict", ConflictError(cmd, "x", errors.New("bad")), 5},
		{"network", NetworkError(cmd, errors.New("bad")), 6},
		{"auth", AuthError(cmd, errors.New("bad")), 7},
		{"internal/runtime", RuntimeError(cmd, errors.New("bad")), 1},
		{"unclassified plain error", errors.New("bad"), 1},
		{"nil", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitCode(tt.err); got != tt.want {
				t.Errorf("ExitCode() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWrapCLIError_PreservesExistingClassification(t *testing.T) {
	cmd := newTestCmd()
	original := NotFoundError(cmd, "abc", errors.New("no such wallet"))

	// Re-wrapping an already-classified error (e.g. a lower-level helper
	// that already called NotFoundError, whose result bubbles up through a
	// caller that would otherwise wrap it as RuntimeError) must not
	// silently downgrade its classification.
	rewrapped := RuntimeError(cmd, original)

	if ExitCode(rewrapped) != ExitCode(original) {
		t.Errorf("ExitCode(rewrapped) = %d, want unchanged %d", ExitCode(rewrapped), ExitCode(original))
	}
	ce := AsCLIError(rewrapped)
	if ce.Code != CodeNotFound {
		t.Errorf("Code = %q, want %q (original classification preserved)", ce.Code, CodeNotFound)
	}
}

func TestAsCLIError_UnclassifiedFallsBackToInternal(t *testing.T) {
	ce := AsCLIError(errors.New("boom"))
	if ce.Code != CodeInternal {
		t.Errorf("Code = %q, want %q", ce.Code, CodeInternal)
	}
	if ce.Error() != "boom" {
		t.Errorf("Error() = %q, want %q", ce.Error(), "boom")
	}
}

func TestRetryableCodes(t *testing.T) {
	tests := []struct {
		code ErrorCode
		want bool
	}{
		{CodeConflict, true},
		{CodeNetwork, true},
		{CodeUsage, false},
		{CodeInvalidInput, false},
		{CodeNotFound, false},
		{CodeAuth, false},
		{CodeInternal, false},
	}
	for _, tt := range tests {
		if got := retryableCodes[tt.code]; got != tt.want {
			t.Errorf("retryableCodes[%q] = %v, want %v", tt.code, got, tt.want)
		}
	}
}

func TestRedactSecretInput(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "bare 64-hex secret",
			input: "bb7a99ee8fc7ac5529e0747fc12f438f8e3c7765cd0a887e33d16dee8c0ba7a6",
			want:  "",
		},
		{
			name:  "nsec",
			input: "nsec1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq",
			want:  "",
		},
		{
			name:  "pubkey credential redacts the privkey",
			input: "pubkey:bb7a99ee8fc7ac5529e0747fc12f438f8e3c7765cd0a887e33d16dee8c0ba7a6",
			want:  "pubkey:<redacted>",
		},
		{
			name:  "bearer credential redacts the secret",
			input: "bearer:deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
			want:  "bearer:<redacted>",
		},
		{
			name:  "connection-key credential redacts only the leading privkey",
			input: "connection-key:bb7a99ee8fc7ac5529e0747fc12f438f8e3c7765cd0a887e33d16dee8c0ba7a6,discord,482910,./attestation.json",
			want:  "connection-key:<redacted>,discord,482910,./attestation.json",
		},
		{
			name:  "an ordinary non-secret value passes through unchanged",
			input: "circle:family",
			want:  "circle:family",
		},
		{
			name:  "a cash token passes through unchanged (not secret-shaped)",
			input: "lokicash1qypqxpq9qcrsszg2pvxq6rs0zqg3zyg3zygs9qypqxpq",
			want:  "lokicash1qypqxpq9qcrsszg2pvxq6rs0zqg3zyg3zygs9qypqxpq",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RedactSecretInput(tt.input); got != tt.want {
				t.Errorf("RedactSecretInput(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestArgsValidators(t *testing.T) {
	t.Run("ExactArgs", func(t *testing.T) {
		v := ExactArgs(1)
		cmd := newTestCmd()
		if err := v(cmd, []string{"a"}); err != nil {
			t.Errorf("unexpected error for correct arg count: %v", err)
		}
		cmd2 := newTestCmd()
		err := v(cmd2, []string{})
		if err == nil {
			t.Fatal("expected error for wrong arg count")
		}
		if ExitCode(err) != 2 {
			t.Errorf("ExitCode = %d, want 2 (usage)", ExitCode(err))
		}
	})
	t.Run("MaximumNArgs", func(t *testing.T) {
		v := MaximumNArgs(1)
		if err := v(newTestCmd(), []string{"a"}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if err := v(newTestCmd(), []string{"a", "b"}); err == nil {
			t.Error("expected error for too many args")
		}
	})
	t.Run("MinimumNArgs", func(t *testing.T) {
		v := MinimumNArgs(1)
		if err := v(newTestCmd(), []string{"a"}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if err := v(newTestCmd(), []string{}); err == nil {
			t.Error("expected error for too few args")
		}
	})
	t.Run("NoArgs", func(t *testing.T) {
		if err := NoArgs(newTestCmd(), []string{}); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if err := NoArgs(newTestCmd(), []string{"a"}); err == nil {
			t.Error("expected error for unexpected arg")
		}
	})
}

func TestNWCError_KnownCodeMapsAndTranslates(t *testing.T) {
	cmd := newTestCmd()
	err := NWCError(cmd, &relayclient.WalletError{Method: "pay_invoice", Code: "QUOTA_EXCEEDED", Message: "raw wallet message"})
	ce := AsCLIError(err)
	if ce.Code != CodeInternal {
		t.Errorf("Code = %q, want %q (QUOTA_EXCEEDED has no more specific bucket)", ce.Code, CodeInternal)
	}
	if ce.NWCCode != "QUOTA_EXCEEDED" {
		t.Errorf("NWCCode = %q, want %q", ce.NWCCode, "QUOTA_EXCEEDED")
	}
	if ce.Error() == "raw wallet message" {
		t.Error("expected the translated human message, not the raw wallet message, in human-mode Error()")
	}
}

func TestNWCError_ExpiredMapsToAuth(t *testing.T) {
	err := NWCError(newTestCmd(), &relayclient.WalletError{Code: "EXPIRED", Message: "..."})
	if AsCLIError(err).Code != CodeAuth {
		t.Errorf("Code = %q, want %q", AsCLIError(err).Code, CodeAuth)
	}
	if ExitCode(err) != 7 {
		t.Errorf("ExitCode = %d, want 7", ExitCode(err))
	}
}

func TestNWCError_UnknownCodeFallsBackToWalletMessage(t *testing.T) {
	err := NWCError(newTestCmd(), &relayclient.WalletError{Code: "SOME_FUTURE_CODE", Message: "a message cashctl doesn't know how to translate"})
	ce := AsCLIError(err)
	if ce.Code != CodeInternal {
		t.Errorf("Code = %q, want %q", ce.Code, CodeInternal)
	}
	if ce.Error() != "a message cashctl doesn't know how to translate" {
		t.Errorf("Error() = %q, want the wallet's own message verbatim", ce.Error())
	}
}
