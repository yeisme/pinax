package mlptest

import "testing"

func TestValidPathHashMatchesBackendHashShape(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "sha256 value", value: "sha256:path-a", want: true},
		{name: "path prefix", value: "path_abc123", want: true},
		{name: "hex64", value: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", want: true},
		{name: "empty", value: "", want: false},
		{name: "sha256 empty", value: "sha256:", want: false},
		{name: "slash", value: "sha256:path/private", want: false},
		{name: "backslash", value: "path_private\\note", want: false},
		{name: "plaintext path", value: "path/private", want: false},
		{name: "arbitrary plaintext", value: "private", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validPathHash(tt.value); got != tt.want {
				t.Fatalf("validPathHash(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}
