package testvcr_test

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
)

func TestModeFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		vcrMode string
		want    testvcr.RecorderMode
	}{
		{"unset-defaults-to-replay", "", testvcr.ModeReplay},
		{"record-mode", "record", testvcr.ModeRecord},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("VCR_MODE", tc.vcrMode)
			if got := testvcr.Mode(); got != tc.want {
				t.Fatalf("Mode() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCassettePathResolvesUnderCallerTestdata(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VCR_TESTDATA_OVERRIDE", dir)
	t.Setenv("VCR_MODE", "replay")
	yaml := "version: 2\ninteractions: []\n"
	if err := os.WriteFile(filepath.Join(dir, "smoke.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := testvcr.New(t, "smoke")
	if rt == nil {
		t.Fatal("expected non-nil transport")
	}
}

func TestNewSkipsWhenCassetteMissing(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VCR_TESTDATA_OVERRIDE", dir)
	t.Setenv("VCR_MODE", "replay")
	t.Setenv("CI_REQUIRE_CASSETTES", "")

	var ran bool
	var skipped bool
	t.Run("missing", func(sub *testing.T) {
		defer func() { skipped = sub.Skipped() }()
		_ = testvcr.New(sub, "does_not_exist")
		ran = true
	})
	if ran {
		t.Fatal("expected New to skip before returning")
	}
	if !skipped {
		t.Fatal("expected subtest to be marked skipped")
	}
}

// TestNewFatalsWhenCassetteMissingAndRequired pins the end-to-end behaviour of
// testvcr.New: with CI_REQUIRE_CASSETTES=1 and no cassette on disk, the call
// must terminate the test with a fatal error rather than skipping. Because
// t.Fatalf inside a subtest still marks the parent test as failed, we verify
// the fatal path by re-executing the test binary as a child process and
// asserting on its non-zero exit + stderr message.
func TestNewFatalsWhenCassetteMissingAndRequired(t *testing.T) {
	if dir := os.Getenv("TESTVCR_FATAL_DIR"); dir != "" {
		t.Setenv("VCR_TESTDATA_OVERRIDE", dir)
		t.Setenv("VCR_MODE", "replay")
		t.Setenv("CI_REQUIRE_CASSETTES", "1")
		_ = testvcr.New(t, "does_not_exist")
		return
	}
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestNewFatalsWhenCassetteMissingAndRequired$", "-test.v")
	cmd.Env = append(os.Environ(), "TESTVCR_FATAL_DIR="+dir)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("child process expected to fail, got success. Output:\n%s", out)
	}
	if !strings.Contains(string(out), "testvcr: cassette not recorded") {
		t.Fatalf("expected fatal message in output, got:\n%s", out)
	}
}

// syntheticTransport returns a fixed response without performing any network
// I/O. It is used to verify that NewAtPath's WithRealTransport option installs
// the supplied transport as the recorder's real upstream — so any response it
// synthesises lands in the cassette via the normal capture path.
type syntheticTransport struct {
	status int
	body   string
	calls  int
}

func (s *syntheticTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.calls++
	return &http.Response{
		StatusCode: s.status,
		Status:     http.StatusText(s.status),
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Body:       io.NopCloser(bytes.NewBufferString(s.body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Request:    req,
	}, nil
}

// TestNewAtPathWithRealTransportRecordsSyntheticResponse pins the fix for #87:
// a custom realTransport that synthesises responses (without performing real
// network I/O) must still produce cassette interactions in record mode. The
// pre-fix injector chain wrapped the recorder externally, so synthetic
// responses short-circuited before the recorder could observe them.
func TestNewAtPathWithRealTransportRecordsSyntheticResponse(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("VCR_MODE", "record")
	t.Setenv("CI", "")
	t.Setenv("GITHUB_ACTIONS", "")
	t.Setenv("BUILDKITE", "")
	t.Setenv("CIRCLECI", "")

	syn := &syntheticTransport{
		status: http.StatusUnprocessableEntity,
		body:   `{"Code":9001,"Error":"Human verification required"}`,
	}
	cassettePath := filepath.Join(dir, "synthetic")

	rt, stop, err := testvcr.NewAtPath(
		cassettePath,
		testvcr.ModeRecord,
		testvcr.WithRealTransport(syn),
	)
	if err != nil {
		t.Fatalf("NewAtPath: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, "https://example.test/core/v4/users", http.NoBody)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	if err = stop(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if syn.calls != 1 {
		t.Fatalf("synthetic transport calls = %d, want 1", syn.calls)
	}

	yaml, err := os.ReadFile(cassettePath + ".yaml")
	if err != nil {
		t.Fatalf("read cassette: %v", err)
	}
	got := string(yaml)
	if !strings.Contains(got, "/core/v4/users") {
		t.Errorf("cassette missing recorded URL\n%s", got)
	}
	if !strings.Contains(got, "Human verification required") {
		t.Errorf("cassette missing synthetic body\n%s", got)
	}
}

// TestCassettePathSkipsTestvcrAndTestharnessFrames exercises the stack-walking
// resolver without VCR_TESTDATA_OVERRIDE. The caller of testvcr.New is this
// _test.go file in internal/testvcr/, which stays eligible despite being in
// the testvcr package because the resolver only skips non-test sources.
func TestCassettePathSkipsTestvcrAndTestharnessFrames(t *testing.T) {
	t.Setenv("VCR_MODE", "replay")
	dir := filepath.Join("testdata", "cassettes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "walk_smoke.yaml")
	yaml := "version: 2\ninteractions: []\n"
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	rt := testvcr.New(t, "walk_smoke")
	if rt == nil {
		t.Fatal("expected non-nil transport")
	}
}
