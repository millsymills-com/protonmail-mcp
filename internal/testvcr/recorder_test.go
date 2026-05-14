package testvcr_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
)

func TestModeDefaultsToReplay(t *testing.T) {
	t.Setenv("VCR_MODE", "")
	if got := testvcr.Mode(); got != testvcr.ModeReplay {
		t.Fatalf("default mode = %v, want replay", got)
	}
}

func TestModeRecord(t *testing.T) {
	t.Setenv("VCR_MODE", "record")
	if got := testvcr.Mode(); got != testvcr.ModeRecord {
		t.Fatalf("mode = %v, want record", got)
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
