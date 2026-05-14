package pack

import (
	"slices"
	"testing"
)

func TestParseCsv(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", []string{""}},
		{"single", "abc", []string{"abc"}},
		{"two", "a,b", []string{"a", "b"}},
		{"three", "a,b,c", []string{"a", "b", "c"}},
		{"trailing-comma", "a,", []string{"a", ""}},
		{"leading-comma", ",a", []string{"", "a"}},
		{"quoted-comma", `"a,b",c`, []string{"a,b", "c"}},
		{"only-quotes", `"abc"`, []string{"abc"}},
		{"empty-quoted", `""`, []string{""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseCsv(tc.in)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("parseCsv(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
