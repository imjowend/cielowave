package tidal

import (
	"strings"
	"testing"
)

// RFC 7636 Appendix B test vector.
const (
	knownVerifier  = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	knownChallenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
)

func TestGenerateCodeVerifier_Format(t *testing.T) {
	v, err := generateCodeVerifier()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(v) == 0 {
		t.Fatal("verifier is empty")
	}
	// base64url has no padding or '+' or '/'
	if strings.ContainsAny(v, "+/=") {
		t.Fatalf("verifier contains non-base64url chars: %q", v)
	}
}

func TestComputeCodeChallenge_KnownVector(t *testing.T) {
	got := computeCodeChallenge(knownVerifier)
	if got != knownChallenge {
		t.Fatalf("expected %q, got %q", knownChallenge, got)
	}
}

func TestGenerateState_Format(t *testing.T) {
	s, err := generateState()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s) == 0 {
		t.Fatal("state is empty")
	}
	if strings.ContainsAny(s, "+/=") {
		t.Fatalf("state contains non-base64url chars: %q", s)
	}
}
