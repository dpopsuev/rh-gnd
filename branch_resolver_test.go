package dsr

import "testing"

func TestResolveBranch(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		attrs   map[string]string
		want    string
	}{
		{"empty pattern", "", nil, ""},
		{"no placeholders", "main", nil, "main"},
		{"single placeholder", "release-{ocp_version}", map[string]string{"ocp_version": "4.21"}, "release-4.21"},
		{"multiple placeholders", "{prefix}-{version}", map[string]string{"prefix": "release", "version": "4.21"}, "release-4.21"},
		{"missing key left as-is", "release-{ocp_version}", map[string]string{}, "release-{ocp_version}"},
		{"extra attrs ignored", "main", map[string]string{"ocp_version": "4.21"}, "main"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveBranch(tt.pattern, tt.attrs)
			if got != tt.want {
				t.Errorf("ResolveBranch(%q, %v) = %q, want %q", tt.pattern, tt.attrs, got, tt.want)
			}
		})
	}
}
