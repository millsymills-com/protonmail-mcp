package testvcr

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// nowFunc is a package-level indirection so tests can freeze time.
var nowFunc = time.Now

// Finding describes one match of a forbidden pattern inside a cassette file.
type Finding struct {
	Path string
	Line int
	Rule string
	Hit  string
}

type lintRule struct {
	name string
	re   *regexp.Regexp
}

// Token rules use `(?i)` because Proton's API returns the same field in
// multiple casings — e.g. /auth/v4/refresh emits both "UID" and "Uid" in the
// same response. A case-sensitive rule misses the lowercase variant; the
// scrubber's case-fold lookup catches it, and this lint must mirror that.
var lintRules = []lintRule{
	{"bearer-token", regexp.MustCompile(`Bearer [A-Za-z0-9._\-]{20,}`)},
	{"access-token-raw", regexp.MustCompile(`(?i)"AccessToken":\s*"[^R][^"]+"`)},
	{"refresh-token-raw", regexp.MustCompile(`(?i)"RefreshToken":\s*"[^R][^"]+"`)},
	{"uid-raw", regexp.MustCompile(`(?i)"UID":\s*"[^R][^"]+"`)},
	{"key-salt-raw", regexp.MustCompile(`(?i)"KeySalt":\s*"[^R][^"]+"`)},
	{"srp-session-raw", regexp.MustCompile(`(?i)"SrpSession":\s*"[^R][^"]+"`)},
	{"server-proof-raw", regexp.MustCompile(`(?i)"ServerProof":\s*"[^R][^"]+"`)},
	{"client-proof-raw", regexp.MustCompile(`(?i)"ClientProof":\s*"[^R][^"]+"`)},
	{"client-ephemeral-raw", regexp.MustCompile(`(?i)"ClientEphemeral":\s*"[^R][^"]+"`)},
	{"two-factor-code-raw", regexp.MustCompile(`(?i)"TwoFactorCode":\s*"[^R][^"]+"`)},
	{"recovery-secret-raw", regexp.MustCompile(`(?i)"RecoverySecret":\s*"[^R][^"]+"`)},
	{"fingerprint-raw", regexp.MustCompile(`(?i)"Fingerprint":\s*"[^R][^"]+"`)},
	// Flag any Proton-issued PGP armor — PRIVATE KEY BLOCK, MESSAGE, or
	// SIGNATURE — whose header line carries "Version: ProtonMail". The
	// recorder swaps real keys for an in-repo gopenpgp fixture (see
	// internal/testvcr/fixtures.go) whose armor reads
	// "Comment: https://gopenpgp.org\nVersion: GopenPGP …", so this rule
	// distinguishes fixture from real-leak. RecoverySecretSignature is the
	// reason MESSAGE/SIGNATURE are in scope — the public-key+signature pair
	// reveals the signing key fingerprint even when the secret is redacted.
	// The escape sequence `\\n` is the literal characters backslash-n that
	// appear in the YAML body, not a real newline.
	{"pgp-proton", regexp.MustCompile(`BEGIN PGP (?:PRIVATE KEY BLOCK|MESSAGE|SIGNATURE)-----\\nVersion: ProtonMail`)},
	{"proton-email", regexp.MustCompile(`@protonmail\.|@proton\.me|@pm\.me`)},
}

const staleThreshold = 90 * 24 * time.Hour

// Scan walks root directories and returns findings for cassette lines matching
// forbidden patterns, plus staleness and version-drift findings from sidecars.
func Scan(roots ...string) []Finding {
	var out []Finding
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			ext := filepath.Ext(path)
			if ext != ".yaml" && ext != ".yml" {
				return nil
			}
			// Sidecar files are scanned via scanMeta; skip them here.
			if strings.HasSuffix(path, ".meta.yaml") {
				out = append(out, scanMeta(path)...)
				return nil
			}
			f, err := os.Open(path)
			if err != nil {
				out = append(out, Finding{Path: path, Rule: "read-error", Hit: err.Error()})
				return nil
			}
			defer func() { _ = f.Close() }()
			s := bufio.NewScanner(f)
			s.Buffer(make([]byte, 1<<16), 1<<22)
			line := 0
			for s.Scan() {
				line++
				txt := s.Text()
				for _, rule := range lintRules {
					if m := rule.re.FindString(txt); m != "" {
						if isScrubPlaceholderHit(m) {
							continue
						}
						if rule.name == "pgp-proton" && isClearsignedModulus(txt) {
							// The SRP login fixture (see
							// cmd/record-cassettes/scenarios/srp_fixture.go)
							// ships Proton's globally-published signed
							// modulus, which embeds a SIGNATURE block carrying
							// "Version: ProtonMail". That's the wrapper for a
							// clearsigned modulus, not a leaked private key.
							continue
						}
						out = append(out, Finding{Path: path, Line: line, Rule: rule.name, Hit: m})
					}
				}
			}
			return nil
		})
	}
	return out
}

// isClearsignedModulus reports whether the line contains a PGP clearsigned
// message envelope. The Modulus value in /auth/v4/info responses is a
// clearsigned block whose SIGNATURE section is innocuous (it's the public
// proof Proton publishes for every account); pgp-proton would otherwise
// flag the embedded "Version: ProtonMail" header.
func isClearsignedModulus(line string) bool {
	return strings.Contains(line, "BEGIN PGP SIGNED MESSAGE") &&
		strings.Contains(line, `"Modulus"`)
}

// isScrubPlaceholderHit recognises a lint hit that matched a value already
// scrubbed by [bodyScrubber]. The scrubber emits either a literal
// "REDACTED_*" placeholder or, for keys whose consumer code base64-decodes
// the value before use, the base64 of that placeholder (which begins with
// "UkVEQUNURUQ" — base64("REDACTED")). The raw-value lint regexes use a
// "[^R]" lead so they already skip the literal form; this catches the
// base64 form too.
func isScrubPlaceholderHit(hit string) bool {
	// The matched string includes the JSON key prefix, e.g.
	// `"ServerProof":"UkVEQUNURUQ..."`. Strip everything up to the quoted
	// value to test the placeholder content alone.
	q := strings.IndexByte(hit, ':')
	if q < 0 {
		return false
	}
	rest := strings.TrimSpace(hit[q+1:])
	rest = strings.Trim(rest, `"`)
	// base64.StdEncoding.EncodeToString([]byte("REDACTED_")) = "UkVEQUNURURf";
	// all base64-encoded scrub placeholders begin with that prefix.
	return strings.HasPrefix(rest, "REDACTED_") || strings.HasPrefix(rest, "UkVEQUNURURf")
}

// scanMeta parses a .meta.yaml sidecar and returns staleness/version-drift findings.
func scanMeta(path string) []Finding {
	data, err := os.ReadFile(path)
	if err != nil {
		return []Finding{{Path: path, Rule: "read-error", Hit: err.Error()}}
	}
	var out []Finding
	recordedAt, apiVer := parseMeta(string(data))
	if !recordedAt.IsZero() && nowFunc().Sub(recordedAt) > staleThreshold {
		out = append(out, Finding{
			Path: path,
			Rule: "stale-cassette",
			Hit:  recordedAt.Format(time.RFC3339) + " > 90d old",
		})
	}
	if apiVer != "" {
		current := goProtonAPIVersion()
		if current != "unknown" && apiVer != current {
			out = append(out, Finding{
				Path: path,
				Rule: "version-drift",
				Hit:  apiVer + " vs " + current,
			})
		}
	}
	return out
}

// parseMeta extracts recorded_at and go_proton_api_version from raw YAML text.
func parseMeta(data string) (time.Time, string) {
	var recordedAt time.Time
	var apiVer string
	for _, line := range strings.Split(data, "\n") {
		if after, ok := strings.CutPrefix(line, "recorded_at:"); ok {
			t, err := time.Parse(time.RFC3339, strings.TrimSpace(after))
			if err == nil {
				recordedAt = t
			}
		}
		if after, ok := strings.CutPrefix(line, "go_proton_api_version:"); ok {
			apiVer = strings.TrimSpace(after)
		}
	}
	return recordedAt, apiVer
}

// GoProtonAPIVersion reads the go-proton-api version from go.mod.
// Exported so tests can embed the current version into fixture metadata.
func GoProtonAPIVersion() string {
	return goProtonAPIVersion()
}

// goProtonAPIVersion reads the go-proton-api version from the nearest go.mod,
// walking upward from the current working directory.
func goProtonAPIVersion() string {
	dir, err := os.Getwd()
	if err != nil {
		return "unknown"
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				if strings.Contains(line, "github.com/ProtonMail/go-proton-api") {
					parts := strings.Fields(line)
					if len(parts) >= 2 {
						return parts[len(parts)-1]
					}
				}
			}
			return "unknown"
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "unknown"
}
