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

var lintRules = []lintRule{
	{"bearer-token", regexp.MustCompile(`Bearer [A-Za-z0-9._\-]{20,}`)},
	{"access-token-raw", regexp.MustCompile(`"AccessToken":\s*"[^R][^"]+"`)},
	{"refresh-token-raw", regexp.MustCompile(`"RefreshToken":\s*"[^R][^"]+"`)},
	{"uid-raw", regexp.MustCompile(`"UID":\s*"[^R][^"]+"`)},
	{"key-salt-raw", regexp.MustCompile(`"KeySalt":\s*"[^R][^"]+"`)},
	{"srp-session-raw", regexp.MustCompile(`"SrpSession":\s*"[^R][^"]+"`)},
	{"server-proof-raw", regexp.MustCompile(`"ServerProof":\s*"[^R][^"]+"`)},
	{"client-proof-raw", regexp.MustCompile(`"ClientProof":\s*"[^R][^"]+"`)},
	{"client-ephemeral-raw", regexp.MustCompile(`"ClientEphemeral":\s*"[^R][^"]+"`)},
	{"two-factor-code-raw", regexp.MustCompile(`"TwoFactorCode":\s*"[^R][^"]+"`)},
	{"pgp-private", regexp.MustCompile(`BEGIN PGP PRIVATE KEY BLOCK`)},
	{"pgp-message", regexp.MustCompile(`BEGIN PGP MESSAGE`)},
	{"proton-email", regexp.MustCompile(`@protonmail\.|@proton\.me`)},
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
						out = append(out, Finding{Path: path, Line: line, Rule: rule.name, Hit: m})
					}
				}
			}
			return nil
		})
	}
	return out
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
