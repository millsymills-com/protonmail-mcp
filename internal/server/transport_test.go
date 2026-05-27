package server

import "testing"

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestTransportConfigFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    transportConfig
		wantErr string
	}{
		{name: "default stdio", env: nil, want: transportConfig{kind: "stdio"}},
		{
			name: "sse with host and port",
			env:  map[string]string{"PROTONMAIL_MCP_TRANSPORT": "sse", "PROTONMAIL_MCP_PORT": "8770"},
			want: transportConfig{kind: "sse", host: "127.0.0.1", port: 8770},
		},
		{
			name: "sse custom host",
			env:  map[string]string{"PROTONMAIL_MCP_TRANSPORT": "sse", "PROTONMAIL_MCP_HOST": "0.0.0.0", "PROTONMAIL_MCP_PORT": "9000"},
			want: transportConfig{kind: "sse", host: "0.0.0.0", port: 9000},
		},
		{name: "sse missing port", env: map[string]string{"PROTONMAIL_MCP_TRANSPORT": "sse"}, wantErr: "PROTONMAIL_MCP_PORT is required"},
		{name: "invalid transport", env: map[string]string{"PROTONMAIL_MCP_TRANSPORT": "grpc"}, wantErr: `invalid PROTONMAIL_MCP_TRANSPORT "grpc"`},
		{name: "invalid port", env: map[string]string{"PROTONMAIL_MCP_TRANSPORT": "sse", "PROTONMAIL_MCP_PORT": "nope"}, wantErr: "PROTONMAIL_MCP_PORT"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := transportConfigFromEnv(env(tc.env))
			if tc.wantErr != "" {
				if err == nil || !tcContains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %+v want %+v", got, tc.want)
			}
		})
	}
}

func tcContains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && tcStringIndex(s, sub) >= 0)
}

func tcStringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
