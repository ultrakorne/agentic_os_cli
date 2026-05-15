package main

import (
	"strings"
	"testing"
)

func TestSanitize(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"plain", "hello", "hello"},
		{"spaces become underscores", "a b c", "a_b_c"},
		{"newlines become underscores via space", "line1\nline2", "line1_line2"},
		{"mixed whitespace", "a b\nc", "a_b_c"},
		{"exactly 60 chars kept whole", strings.Repeat("x", 60), strings.Repeat("x", 60)},
		{"over 60 chars truncated", strings.Repeat("x", 75), strings.Repeat("x", 60)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitize(tc.in)
			if got != tc.want {
				t.Errorf("sanitize(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
