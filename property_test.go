package hyperre

import (
	"math/rand"
	"regexp"
	"strings"
	"testing"
)

// The alphabet deliberately mixes ASCII, CJK and runes whose SimpleFold
// orbits are not covered by unicode.ToLower.
var propAlphabet = []rune("abcABsSſkKKσς Σ中国敏é1-")

func randLiteral(rng *rand.Rand, max int) string {
	n := 1 + rng.Intn(max)
	var sb strings.Builder
	for i := 0; i < n; i++ {
		r := propAlphabet[rng.Intn(len(propAlphabet))]
		sb.WriteString(regexp.QuoteMeta(string(r)))
	}
	return sb.String()
}

func randRegexp(rng *rand.Rand, depth int) string {
	if depth <= 0 {
		return randLiteral(rng, 3)
	}
	var s string
	switch rng.Intn(10) {
	case 0, 1, 2:
		s = randLiteral(rng, 4)
	case 3:
		s = randRegexp(rng, depth-1) + randRegexp(rng, depth-1)
	case 4:
		s = "(?:" + randRegexp(rng, depth-1) + "|" + randRegexp(rng, depth-1) + ")"
	case 5:
		classes := []string{"[ab]", "[中国]", "[sS]", "[σΣ]", "[^a]", `\d`, ".", `\w`, "[a-c]"}
		s = classes[rng.Intn(len(classes))]
	case 6:
		s = "(" + randRegexp(rng, depth-1) + ")"
	case 7:
		anchors := []string{"^", "$", `\b`, `\B`}
		s = anchors[rng.Intn(len(anchors))] + randRegexp(rng, depth-1)
	default:
		s = randRegexp(rng, depth-1)
	}
	quants := []string{"", "", "", "?", "*", "+", "{0,2}", "{1,3}", "{2}"}
	q := quants[rng.Intn(len(quants))]
	if q != "" {
		s = "(?:" + s + ")" + q
	}
	return s
}

func randText(rng *rand.Rand, literals []string) string {
	var sb strings.Builder
	n := rng.Intn(16)
	for i := 0; i < n; i++ {
		if len(literals) > 0 && rng.Intn(4) == 0 {
			// splice in a piece of some pattern, possibly re-cased
			l := literals[rng.Intn(len(literals))]
			if rng.Intn(2) == 0 {
				l = strings.ToLower(l)
			}
			sb.WriteString(l)
			continue
		}
		sb.WriteRune(propAlphabet[rng.Intn(len(propAlphabet))])
	}
	return sb.String()
}

func foldString(s string) string {
	return foldRunes([]rune(s))
}

// TestPropertyNoFalseNegatives checks the core guarantee on random patterns:
// whenever Go's regexp matches, the pattern is a candidate. It also checks
// the automaton exactly: an indexed pattern is a candidate iff one of its
// literals occurs in the folded text.
func TestPropertyNoFalseNegatives(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	rounds := 300
	if testing.Short() {
		rounds = 30
	}
	optionSets := [][]Option{
		nil,
		{WithMinLiteralLen(1)},
		{WithMinLiteralLen(3)},
		{WithCJKSingleChar(false)},
		{WithMaxLiterals(2)},
	}
	for round := 0; round < rounds; round++ {
		opts := optionSets[round%len(optionSets)]
		cfg := newConfig(opts)
		var patterns []string
		var res []*regexp.Regexp
		var unquoted []string
		for len(patterns) < 40 {
			p := randRegexp(rng, 3)
			if rng.Intn(3) == 0 {
				p = "(?i)" + p
			}
			re, err := regexp.Compile(p)
			if err != nil {
				continue
			}
			patterns = append(patterns, p)
			res = append(res, re)
			unquoted = append(unquoted, extractLiterals(p)...)
		}
		set := MustNew(patterns, opts...)
		for k := 0; k < 50; k++ {
			text := randText(rng, unquoted)
			cands := map[int]bool{}
			for _, i := range filterIdx(set, text) {
				cands[i] = true
			}
			matched := map[int]bool{}
			for _, i := range matchIdx(set, text) {
				matched[i] = true
			}
			folded := foldString(text)
			for i, re := range res {
				want := re.MatchString(text)
				if want && !cands[i] {
					t.Fatalf("false negative: pattern %q matches %q but is not a candidate (literals %q)",
						patterns[i], text, cfg.extractLiterals(patterns[i]))
				}
				if want != matched[i] {
					t.Fatalf("Match differs from regexp for %q on %q", patterns[i], text)
				}
				lits := cfg.extractLiterals(patterns[i])
				if len(lits) == 0 {
					if !cands[i] {
						t.Fatalf("fallback pattern %q missing for %q", patterns[i], text)
					}
					continue
				}
				present := false
				for _, l := range lits {
					if strings.Contains(folded, l) {
						present = true
						break
					}
				}
				if present != cands[i] {
					t.Fatalf("automaton mismatch for %q on %q: candidate=%v, literal present=%v (literals %q)",
						patterns[i], text, cands[i], present, lits)
				}
			}
		}
	}
}
