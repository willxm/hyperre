package hyperre

import (
	"reflect"
	"regexp"
	"testing"
	"unicode/utf8"
)

// FuzzMatchesRegexp: Matcher must report exactly what Go's regexp reports.
// Run with: go test -fuzz=FuzzMatchesRegexp
func FuzzMatchesRegexp(f *testing.F) {
	seeds := []struct{ pattern, text string }{
		{`abc.*def`, "xxabczzdef"},
		{`(?i)SS`, "ſſ"},
		{`(?i)Σ+x`, "ςx"},
		{`(?i)kelvin`, "Kelvin"},
		{`下雨.然后|下雨.{0,3}下雨`, "下雨？又下雨了？"},
		{`[中美]国`, "美国"},
		{`(foo|bar)+baz`, "barbaz"},
		{`\bhello\b`, "say hello"},
		{`(?i)[sk]x`, "ſx"},
		{`a{2,}b`, "aaab"},
	}
	for _, s := range seeds {
		f.Add(s.pattern, s.text)
	}
	f.Fuzz(func(t *testing.T, pattern, text string) {
		if len(pattern) > 64 || !utf8.ValidString(pattern) {
			return
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return
		}
		m := MustNew([]string{"zzzunrelated", pattern})
		want := re.MatchString(text)
		got := matchIdx(m, text)
		if want != (len(got) == 1 && got[0] == 1) || len(got) > 1 {
			t.Fatalf("pattern %q text %q: regexp=%v matcher=%v (literals %q)", pattern, text, want, got, extractLiterals(pattern))
		}
		if want {
			hits := m.FindAllIndex(text)
			if len(hits) != 1 || !reflect.DeepEqual(hits[0].Loc, re.FindAllStringIndex(text, -1)) {
				t.Fatalf("FindAll differs: %v", hits)
			}
		}
	})
}
