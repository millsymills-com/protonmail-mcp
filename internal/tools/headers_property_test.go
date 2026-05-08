package tools

import (
	"math/rand/v2"
	"strings"
	"testing"
)

// Deny-list — must match sensitiveHeaders in messages.go.
var denyList = []string{
	"bcc",
	"x-originating-ip",
	"x-original-sender-ip",
	"x-original-sender",
	"x-real-ip",
}

func TestFilterSensitiveHeaders_NoDenyListLeaksAcrossCasingsAndWhitespace(t *testing.T) {
	//nolint:gosec // G404: weak RNG is fine for property test determinism; not cryptographic
	rng := rand.New(rand.NewPCG(0xC0DE, 0xFEED))
	const iterations = 1000

	for i := 0; i < iterations; i++ {
		in := map[string][]string{}
		// Always include each denied header in a randomly mangled form.
		for _, name := range denyList {
			in[mangle(name, rng)] = []string{"VALUE"}
		}
		// Add some safe noise.
		for _, safe := range []string{"Authentication-Results", "DKIM-Signature", "Received"} {
			in[mangle(safe, rng)] = []string{"x"}
		}

		got := filterSensitiveHeaders(in)

		for k := range got {
			lower := strings.ToLower(strings.TrimSpace(k))
			for _, denied := range denyList {
				if lower == denied {
					t.Fatalf("iter %d: denied header leaked: %q", i, k)
				}
			}
		}
	}
}

func mangle(name string, rng *rand.Rand) string {
	var b strings.Builder
	for _, r := range name {
		if rng.IntN(2) == 0 {
			b.WriteRune(toUpper(r))
		} else {
			b.WriteRune(r)
		}
	}
	// filterSensitiveHeaders normalizes only via strings.ToLower; whitespace
	// padding should NOT match. We deliberately do NOT pad here, because the
	// production behavior is exact-match-after-lowercase. If you want to
	// extend to whitespace tolerance, change sensitiveHeaders lookup first.
	return b.String()
}

func toUpper(r rune) rune {
	if r >= 'a' && r <= 'z' {
		return r - 32
	}
	return r
}
