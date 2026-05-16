package testvcr

import "testing"

func TestRequireCassettesPresent(t *testing.T) {
	tests := []struct {
		name string
		val  string
		want bool
	}{
		{"unset", "", false},
		{"explicit zero", "0", false},
		{"explicit false", "false", false},
		{"truthy 1", "1", true},
		{"truthy true", "true", true},
		{"any non-falsy value", "yes", true},
		{"case-sensitive False is truthy", "False", true},
		{"case-sensitive FALSE is truthy", "FALSE", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CI_REQUIRE_CASSETTES", tt.val)
			if got := requireCassettesPresent(); got != tt.want {
				t.Errorf("requireCassettesPresent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequireCassettesPresentIgnoresCIEnv(t *testing.T) {
	t.Setenv("CI_REQUIRE_CASSETTES", "")
	for _, k := range ciEnvKeys {
		t.Run(k, func(t *testing.T) {
			t.Setenv(k, "true")
			if requireCassettesPresent() {
				t.Fatalf("requireCassettesPresent() = true with only %s set; want explicit opt-in", k)
			}
		})
	}
}
