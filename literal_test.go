package hyperre

import (
	"reflect"
	"sort"
	"testing"
)

func sorted(xs []string) []string {
	out := append([]string(nil), xs...)
	sort.Strings(out)
	return out
}

func TestExtractLiterals(t *testing.T) {
	tests := []struct {
		pattern string
		want    []string // sorted; nil means fallback
	}{
		// literals are SimpleFold keys (ASCII letters upper case)
		{`abc`, []string{"ABC"}},
		{`(?i)abc`, []string{"ABC"}},
		{`ABC`, []string{"ABC"}},
		{`abc.*def`, []string{"ABC"}},
		{`ab.*cdef`, []string{"CDEF"}},
		{`(abc|defg)`, []string{"ABC", "DEFG"}},
		{`(?:abc|abd)xyz`, []string{"XYZ"}},
		{`(abc)+`, []string{"ABC"}},
		{`(abc){2,5}`, []string{"ABC"}},
		{`^\bhello\b$`, []string{"HELLO"}},
		{`敏.*词`, []string{"敏"}},
		{`[中美]国`, []string{"国"}},
		{`[中美]`, []string{"中", "美"}},
		{`下雨.然后|下雨.{0,3}下雨`, []string{"下雨"}},
		{`foo|bar|baz`, []string{"BA", "FOO"}}, // factored to foo|ba[rz]
		{`x?abc`, []string{"ABC"}},
		{`é`, nil}, // single non-CJK rune
		{`éa`, []string{"ÉA"}},
		{`é.*ab`, []string{"AB"}}, // skips the unusable "é" instead of giving up
		{`a.*b`, nil},
		{`a`, nil},
		{`.*`, nil},
		{`(abc)?`, nil},
		{`(abc)*`, nil},
		{`(abc){0,3}`, nil},
		{`abc|.*`, nil},
		{`abc|d`, nil},
		{`[a-z]+`, nil},
		{`\d{6}`, nil},
		{``, nil},
		{`[^a]bc`, []string{"BC"}},
		{`(`, nil}, // parse error
	}
	for _, tt := range tests {
		got := extractLiterals(tt.pattern)
		if got != nil {
			got = sorted(got)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("extractLiterals(%q) = %q, want %q", tt.pattern, got, tt.want)
		}
	}
}

func TestFoldRune(t *testing.T) {
	groups := [][]rune{
		{'A', 'a'},
		{'K', 'k', 'K'},
		{'S', 's', 'ſ'},
		{'Σ', 'σ', 'ς'},
		{'µ', 'Μ', 'μ'},
		{'Β', 'β', 'ϐ'},
		{'В', 'в', 'ᲀ'},
		{'Ṡ', 'ṡ', 'ẛ'},
		{0x10400, 0x10428}, // Deseret, outside the BMP table
	}
	for _, g := range groups {
		k := foldRune(g[0])
		for _, r := range g[1:] {
			if foldRune(r) != k {
				t.Errorf("foldRune(%U)=%U, foldRune(%U)=%U; want equal", g[0], k, r, foldRune(r))
			}
		}
	}
	for _, r := range []rune{'中', '1', '-', 0x10FFFF} {
		if foldRune(r) != r {
			t.Errorf("foldRune(%U) = %U, want unchanged", r, foldRune(r))
		}
	}
	for r := rune(0); r < foldTableSize; r++ {
		if foldRune(r) != foldRuneSlow(r) {
			t.Fatalf("table mismatch at %U", r)
		}
	}
}
