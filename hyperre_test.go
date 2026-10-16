package hyperre

import (
	"reflect"
	"regexp"
	"testing"
)

func ints(xs ...int) []int { return xs }

func eq(a, b []int) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

func TestIndexedPattern(t *testing.T) {
	m := MustNew([]string{"abc.*def"})
	if got := filterIdx(m, "xxabczz"); !eq(got, ints(0)) {
		t.Fatalf("Filter hit: %v", got)
	}
	if got := filterIdx(m, "no-hit-text"); !eq(got, nil) {
		t.Fatalf("Filter miss: %v", got)
	}
	if got := matchIdx(m, "xxabczz"); !eq(got, nil) {
		t.Fatalf("Match without def: %v", got)
	}
	if got := matchIdx(m, "xxabczzdef"); !eq(got, ints(0)) {
		t.Fatalf("Match: %v", got)
	}
}

func TestFallbackPattern(t *testing.T) {
	m := MustNew([]string{"a.*b"})
	for _, text := range []string{"completely-unrelated", "a---b", ""} {
		if got := filterIdx(m, text); !eq(got, ints(0)) {
			t.Fatalf("%q: %v", text, got)
		}
	}
	if !m.Match("a---b") || m.Match("b---a") {
		t.Fatal("MatchString")
	}
}

func TestSingleCJKLiteral(t *testing.T) {
	m := MustNew([]string{"敏.*词"})
	if got := filterIdx(m, "这里有敏感内容"); !eq(got, ints(0)) {
		t.Fatalf("hit: %v", got)
	}
	if got := filterIdx(m, "completely-unrelated"); !eq(got, nil) {
		t.Fatalf("miss: %v", got)
	}
}

func TestAlternationSharedLiteral(t *testing.T) {
	m := MustNew([]string{"下雨.然后|下雨.{0,3}下雨"})
	if got := matchIdx(m, "下雨？又下雨了？"); !eq(got, ints(0)) {
		t.Fatalf("got %v", got)
	}
}

func TestOrder(t *testing.T) {
	m := MustNew([]string{
		"a.*b",   // 0 fallback
		"xyz",    // 1 not hit
		"abc",    // 2 hit
		"[ab]+",  // 3 fallback
		"ab",     // 4 hit
		"bc|zzz", // 5 hit
	})
	if got := filterIdx(m, "xxabcxx"); !eq(got, ints(0, 2, 3, 4, 5)) {
		t.Fatalf("Filter: %v", got)
	}
	if got := matchIdx(m, "xxabcxx"); !eq(got, ints(0, 2, 3, 4, 5)) {
		t.Fatalf("Match: %v", got)
	}
}

func TestOverlappingLiterals(t *testing.T) {
	m := MustNew([]string{"he", "she", "his", "hers", "ushers"})
	cases := map[string][]int{
		"ushers": {0, 1, 3, 4},
		"his":    {2},
		"ahishe": {0, 1, 2},
		"sh":     nil,
		"HeRs":   {0, 3}, // folded candidates; patterns are case-sensitive
	}
	for text, want := range cases {
		if got := filterIdx(m, text); !eq(got, want) {
			t.Errorf("%q: got %v want %v", text, got, want)
		}
	}
	if got := matchIdx(m, "HeRs"); !eq(got, nil) {
		t.Errorf("case-sensitive patterns must not match: %v", got)
	}
}

func TestFindAllIndex(t *testing.T) {
	m := MustNew([]string{"ab+", "zzz", `\d+`})
	text := "ab abbb 12 ab"
	want := []Hit[string]{
		{Item: "ab+", Index: 0, Loc: [][]int{{0, 2}, {3, 7}, {11, 13}}},
		{Item: `\d+`, Index: 2, Loc: [][]int{{8, 10}}},
	}
	if got := m.FindAllIndex(text); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	if got := m.FindAll(text); !reflect.DeepEqual(got, []string{"ab+", `\d+`}) {
		t.Fatalf("FindAll: %v", got)
	}
	if got, ok := m.Find(text); !ok || got != "ab+" {
		t.Fatalf("Find: %v %v", got, ok)
	}
	// ab+ and \d+ have no literal of 2+ characters and are always candidates
	if got := m.Filter("zzz"); !reflect.DeepEqual(got, []string{"ab+", "zzz", `\d+`}) {
		t.Fatalf("Filter: %v", got)
	}
}

func TestMatchStopsEarly(t *testing.T) {
	m := MustNew([]string{"abc", "bcd", "a.*b"})
	if !m.Match("abcd") {
		t.Fatal("should match")
	}
	// the pooled scratch must have been reset after the early stop
	if got := filterIdx(m, "zzz"); !eq(got, ints(2)) {
		t.Fatalf("after early stop: %v", got)
	}
}

func TestEmpty(t *testing.T) {
	m := MustNew([]string(nil))
	if m.Len() != 0 || m.Match("abc") || m.FindAll("abc") != nil || m.Filter("abc") != nil {
		t.Fatal("empty matcher must never match")
	}
}

func TestCompileError(t *testing.T) {
	if _, err := New([]string{"ok", "("}); err == nil {
		t.Fatal("expected error")
	}
}

// Regression tests for (?i) + Unicode SimpleFold. A plain lower-case
// normalisation misses all of these.
func TestUnicodeFoldNoFalseNegative(t *testing.T) {
	cases := []struct{ pattern, text string }{
		{`(?i)SS`, "ſſ"},
		{`(?i)ss`, "ſſ"},
		{`(?i)ΣΑ`, "ςα"},
		{`(?i)σα`, "ςα"},
		{`(?i)μμ`, "µµ"},
		{`(?i)ΒΒ`, "ϐϐ"},
		{`(?i)ВВ`, "ᲀᲀ"},
		{`(?i)Ṡx`, "ẛx"},
		{`(?i)kelvin`, "Kelvin"},
		{`(?i)θεος`, "ϑϵος"},
		{`(?i)[sk]x`, "ſx"},
		{`(?i)[σ][σ]`, "ςς"},
	}
	for _, tc := range cases {
		if !regexp.MustCompile(tc.pattern).MatchString(tc.text) {
			t.Fatalf("test bug: %q should match %q", tc.pattern, tc.text)
		}
		m := MustNew([]string{tc.pattern})
		if got := matchIdx(m, tc.text); !eq(got, ints(0)) {
			t.Errorf("%q vs %q: got %v (literals %q)", tc.pattern, tc.text, got, extractLiterals(tc.pattern))
		}
	}
}

func TestEquivalentToNaive(t *testing.T) {
	patterns := []string{
		`foo.*bar`, `(?i)hello|world`, `敏感.{0,3}词`, `a.*b`, `\bcat\b`, `(?i)ΣΑ`,
		`[中美]国`, `x{2,}y`, `^start`, `end$`, `(ab|cd)+ef`, `\d{3}-\d{4}`,
	}
	m := MustNew(patterns)
	texts := []string{
		"foo and bar", "HELLO there", "敏感的词", "a then b", "a cat sat", "ςα",
		"美国", "xxy", "start here", "the end", "ababef", "555-1234", "", "nothing",
	}
	for _, text := range texts {
		var want []int
		for i, p := range patterns {
			if regexp.MustCompile(p).MatchString(text) {
				want = append(want, i)
			}
		}
		if got := matchIdx(m, text); !eq(got, want) {
			t.Errorf("%q: got %v want %v", text, got, want)
		}
		if m.Match(text) != (len(want) > 0) {
			t.Errorf("%q: MatchString", text)
		}
	}
}

func TestOverflowBuilder(t *testing.T) {
	b := newBuilder()
	b.overflow = true
	if b.build() != nil {
		t.Fatal("overflowed builder must not build")
	}
}

type testRule struct {
	ID    int
	Expr  string
	Level int
}

func TestCustomItems(t *testing.T) {
	rules := []testRule{
		{1, `(?i)buy\s+cheap`, 2},
		{2, `敏感.{0,3}词`, 3},
		{3, `1[3-9]\d{9}`, 1},
	}
	m := MustNew(rules, WithPattern(func(r testRule) string { return r.Expr }))
	rules[0].ID = 99 // the matcher keeps its own copy

	text := "BUY cheap, call 13800138000, 敏感的词"
	got := m.FindAll(text)
	if len(got) != 3 || got[0].ID != 1 || got[1].Level != 3 || got[2].ID != 3 {
		t.Fatalf("FindAll: %+v", got)
	}
	if r, ok := m.Find("x 敏感词 y"); !ok || r.ID != 2 {
		t.Fatalf("Find: %+v %v", r, ok)
	}
	if _, ok := m.Find("hello"); ok {
		t.Fatal("Find miss")
	}

	ptrs := []*testRule{{ID: 7, Expr: "foo"}}
	pm := MustNew(ptrs, WithPattern(func(r *testRule) string { return r.Expr }))
	if r, ok := pm.Find("foo"); !ok || r != ptrs[0] {
		t.Fatal("pointer items should come back as the same pointer")
	}
}

func TestPatternOption(t *testing.T) {
	if _, err := New([]testRule{{1, "a", 0}}); err == nil {
		t.Fatal("non-string items without WithPattern must fail")
	}
	if _, err := New([]testRule{{1, "a", 0}}, WithPattern(func(s string) string { return s })); err == nil {
		t.Fatal("WithPattern with the wrong type must fail")
	}
	// strings may still use WithPattern, e.g. to transform them
	m := MustNew([]string{"a.b"}, WithPattern(func(s string) string { return regexp.QuoteMeta(s) }))
	if !m.Match("xa.by") || m.Match("axb") {
		t.Fatal("custom pattern func")
	}
}

func TestIgnoreCase(t *testing.T) {
	m := MustNew([]string{"hello", "ΣΑ"}, WithIgnoreCase())
	if got := matchIdx(m, "HeLLo ςα"); !eq(got, ints(0, 1)) {
		t.Fatalf("got %v", got)
	}
}

func TestOnInvalid(t *testing.T) {
	var skipped []int
	m, err := New([]string{"ok", "(", "a.*b", "["}, WithOnInvalid(func(i int, p string, err error) {
		skipped = append(skipped, i)
	}))
	if err != nil {
		t.Fatal(err)
	}
	if !eq(skipped, ints(1, 3)) || m.Len() != 4 {
		t.Fatalf("skipped %v", skipped)
	}
	if got := filterIdx(m, "nothing"); !eq(got, ints(2)) {
		t.Fatalf("invalid items must never be candidates: %v", got)
	}
	if got := matchIdx(m, "ok a-b"); !eq(got, ints(0, 2)) {
		t.Fatalf("got %v", got)
	}
}

func TestLiteralOptions(t *testing.T) {
	// min length 1: single ASCII characters become index keys
	m := MustNew([]string{"a.*b"}, WithMinLiteralLen(1))
	if m.ac == nil || len(m.fallback) != 0 {
		t.Fatal("a.*b should be indexed with min length 1")
	}
	if got := filterIdx(m, "zzz"); !eq(got, nil) {
		t.Fatalf("got %v", got)
	}
	// no CJK single characters: 敏.*词 is a fallback
	m = MustNew([]string{"敏.*词", "[中美]国"}, WithCJKSingleChar(false))
	if m.ac != nil || len(m.fallback) != 2 {
		t.Fatal("CJK single chars should be rejected")
	}
	// min length 3: "ab" is too short
	m = MustNew([]string{"ab.*cde"}, WithMinLiteralLen(3))
	if got := filterIdx(m, "ab"); !eq(got, nil) {
		t.Fatalf("got %v", got)
	}
	if got := filterIdx(m, "xcdex"); !eq(got, ints(0)) {
		t.Fatalf("got %v", got)
	}
	// max literals: alternation of 3 with cap 2 falls back
	m = MustNew([]string{"abc|def|ghi"}, WithMaxLiterals(2))
	if m.ac != nil {
		t.Fatal("should fall back")
	}
	// options never change results
	for _, opts := range [][]Option{nil, {WithMinLiteralLen(1)}, {WithMinLiteralLen(4)}, {WithCJKSingleChar(false)}, {WithMaxLiterals(1)}} {
		m := MustNew([]string{"a.*b", "敏.*词", "hello|world", "[中美]国", `\d+x`}, opts...)
		if got := matchIdx(m, "a-b 敏感词 world 美国 12x"); !eq(got, ints(0, 1, 2, 3, 4)) {
			t.Fatalf("opts %d: %v", len(opts), got)
		}
	}
}
