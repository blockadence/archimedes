package status

import "testing"

func TestParsePRList(t *testing.T) {
	tests := []struct {
		name string
		json string
		want PR
	}{
		{"empty array", `[]`, noPR},
		{"one pr", `[{"number":42,"state":"OPEN"}]`, PR{Number: "42", State: "OPEN"}},
		{"malformed json", `not json`, noPR},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePRList([]byte(tt.json))
			if got != tt.want {
				t.Errorf("parsePRList(%q) = %#v, want %#v", tt.json, got, tt.want)
			}
		})
	}
}
