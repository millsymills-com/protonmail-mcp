package tools_test

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/zalando/go-keyring"

	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
	"github.com/millsymills-com/protonmail-mcp/internal/tools"
)

// matrixPath is the checked-in coverage matrix, relative to this package.
const matrixPath = "../../docs/tool-schema-matrix.md"

// updateMatrix regenerates the golden matrix instead of asserting against it:
//
//	go test ./internal/tools -run TestSchemaMatrix -update
var updateMatrix = flag.Bool("update", false, "regenerate docs/tool-schema-matrix.md")

// TestSchemaMatrixNoDrift asserts that docs/tool-schema-matrix.md exactly
// describes the tools the server advertises. The matrix is derived from the
// live MCP tools/list output (the schema clients actually see), so any change
// to a tool name, field, type, required-set, read/write mode, or env gate
// shows up as a diff here. Regenerate with -update after an intentional change.
func TestSchemaMatrixNoDrift(t *testing.T) {
	got := renderMatrix(t)

	if *updateMatrix {
		if err := os.WriteFile(matrixPath, []byte(got), 0o644); err != nil {
			t.Fatalf("write matrix: %v", err)
		}
		t.Logf("wrote %s", matrixPath)
		return
	}

	want, err := os.ReadFile(matrixPath)
	if err != nil {
		t.Fatalf("read matrix (run `go test ./internal/tools -run TestSchemaMatrix -update`): %v", err)
	}
	if string(want) != got {
		t.Fatalf("docs/tool-schema-matrix.md is out of date.\n"+
			"Regenerate with: go test ./internal/tools -run TestSchemaMatrix -update\n\n%s",
			firstDiff(string(want), got))
	}
}

// renderMatrix derives the full matrix by booting the server under each gate
// configuration and reading the advertised schemas. Mode and gate are computed
// from the set differences, so they cannot drift from the registration code.
func renderMatrix(t *testing.T) string {
	t.Helper()

	reads := listTools(t, false, false)
	withWrites := listTools(t, true, false)
	all := listTools(t, true, true)

	names := make([]string, 0, len(all))
	for name := range all {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	b.WriteString(matrixHeader)
	fmt.Fprintf(&b, "Tools: %d total — %d read, %d write, %d dangerous.\n\n",
		len(all), len(reads), len(withWrites)-len(reads), len(all)-len(withWrites))
	b.WriteString("| Tool | Mode | Gate | Input fields | Output fields |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")

	for _, name := range names {
		mode, gate := classify(name, reads, withWrites)
		tool := all[name]
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s | %s |\n",
			name, mode, gate, schemaFields(t, tool.InputSchema), schemaFields(t, tool.OutputSchema))
	}
	return b.String()
}

const matrixHeader = "<!-- GENERATED FILE — do not edit by hand.\n" +
	"     Regenerate: go test ./internal/tools -run TestSchemaMatrix -update\n" +
	"     Source of truth: the tools registered in internal/tools (MCP tools/list). -->\n\n" +
	"# Tool ↔ schema coverage matrix\n\n" +
	"Every tool the server advertises, with its input/output schema fields and the\n" +
	"env gate that registers it. Required fields are marked `*`. `TestSchemaMatrixNoDrift`\n" +
	"asserts this file matches the live schema; it fails if a tool or field drifts.\n\n"

// classify returns the mode and gate for a tool from the set differences.
func classify(name string, reads, withWrites map[string]*mcp.Tool) (mode, gate string) {
	switch {
	case contains(reads, name):
		return "read", "—"
	case contains(withWrites, name):
		return "write", "`WRITES`"
	default:
		return "dangerous", "`WRITES`+`DANGEROUS`"
	}
}

func contains(m map[string]*mcp.Tool, name string) bool {
	_, ok := m[name]
	return ok
}

// schemaFields renders a JSON Schema object's properties as a sorted,
// comma-separated list of "name*:type" (the `*` marks required fields).
func schemaFields(t *testing.T, schema any) string {
	t.Helper()
	if schema == nil {
		return "—"
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var parsed struct {
		Properties map[string]struct {
			Type  json.RawMessage `json:"type"`
			Items struct {
				Type json.RawMessage `json:"type"`
			} `json:"items"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	if len(parsed.Properties) == 0 {
		return "—"
	}
	required := make(map[string]bool, len(parsed.Required))
	for _, r := range parsed.Required {
		required[r] = true
	}

	names := make([]string, 0, len(parsed.Properties))
	for name := range parsed.Properties {
		names = append(names, name)
	}
	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		p := parsed.Properties[name]
		typ := typeString(p.Type)
		if typ == "array" {
			typ = "array<" + typeString(p.Items.Type) + ">"
		}
		marker := ""
		if required[name] {
			marker = "*"
		}
		parts = append(parts, fmt.Sprintf("`%s%s`:%s", name, marker, typ))
	}
	return strings.Join(parts, ", ")
}

// typeString normalizes a JSON Schema "type" (string, or array of strings for
// nullable fields) to a single token.
func typeString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "any"
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return single
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err == nil {
		nonNull := many[:0]
		for _, x := range many {
			if x != "null" {
				nonNull = append(nonNull, x)
			}
		}
		if len(nonNull) > 0 {
			return strings.Join(nonNull, "|")
		}
	}
	return "any"
}

// listTools boots a server under the given gate flags and returns the
// advertised tools keyed by name. No session calls are made — listing schemas
// does not invoke handlers — so an empty mock keychain is sufficient.
func listTools(t *testing.T, writes, dangerous bool) map[string]*mcp.Tool {
	t.Helper()
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", envValue(writes))
	t.Setenv("PROTONMAIL_MCP_ENABLE_DANGEROUS", envValue(dangerous))

	keyring.MockInit()
	sess := session.New("https://mail.proton.me/api", keychain.New())

	srv := mcp.NewServer(&mcp.Implementation{Name: "protonmail-mcp", Version: "test"}, nil)
	tools.Register(srv, tools.Deps{Session: sess})

	clientT, serverT := mcp.NewInMemoryTransports()
	ctx := context.Background()
	srvSession, err := srv.Connect(ctx, serverT, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = srvSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "matrix", Version: "0.0.0"}, nil)
	csess, err := client.Connect(ctx, clientT, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = csess.Close() })

	res, err := csess.ListTools(ctx, &mcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	out := make(map[string]*mcp.Tool, len(res.Tools))
	for _, tool := range res.Tools {
		out[tool.Name] = tool
	}
	return out
}

func envValue(on bool) string {
	if on {
		return "1"
	}
	return "0"
}

// firstDiff returns a short, line-oriented description of the first mismatch
// between want and got, to make a drift failure readable.
func firstDiff(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")
	for i := 0; i < len(wantLines) && i < len(gotLines); i++ {
		if wantLines[i] != gotLines[i] {
			return fmt.Sprintf("first diff at line %d:\n  want: %s\n  got:  %s", i+1, wantLines[i], gotLines[i])
		}
	}
	if len(wantLines) != len(gotLines) {
		return fmt.Sprintf("line count differs: want %d, got %d", len(wantLines), len(gotLines))
	}
	return "files differ"
}
