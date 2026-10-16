package hyperre

import (
	"regexp/syntax"
	"unicode"
	"unicode/utf8"
)

// extractLiterals returns case-folded literals such that any text the
// pattern matches contains (after case folding) at least one of them. It
// returns nil when no such set is known; the pattern is then a fallback that
// always runs.
func (c *config) extractLiterals(pattern string) []string {
	// syntax.Perl is exactly what regexp.Compile uses, so the AST we inspect
	// is the AST Go's regexp engine executes.
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil
	}
	return c.required(re.Simplify())
}

// required returns a set of folded literals such that every string matched by
// r contains at least one of them, or nil if no such (usable) set is known.
// Every literal returned already satisfies the length rules.
//
// Safety argument per op:
//   - Literal: the literal itself is part of every match.
//   - CharClass: every match is one of the class members.
//   - Concat: every match contains a match of every sub-expression, so any
//     single sub-expression's set is valid; we pick the most selective one.
//   - Alternate: every match is a match of some branch, so the union of all
//     branch sets is valid, but only if every branch has a set.
//   - Capture / Plus / Repeat{min>=1}: every match contains a match of the
//     sub-expression.
//   - Quest / Star / Repeat{min=0} and everything else (anchors, any-char,
//     empty-match, ...): nothing is required, return nil.
func (c *config) required(r *syntax.Regexp) []string {
	switch r.Op {
	case syntax.OpLiteral:
		if len(r.Rune) == 0 {
			return nil
		}
		s := foldRunes(r.Rune)
		if !c.validLiteral(s) {
			return nil
		}
		return []string{s}

	case syntax.OpCharClass:
		return c.charClass(r.Rune)

	case syntax.OpConcat:
		var best []string
		bestMin := -1
		for _, sub := range r.Sub {
			group := c.required(sub)
			if len(group) == 0 {
				continue
			}
			m := minByteLen(group)
			if m > bestMin || (m == bestMin && len(group) < len(best)) {
				best, bestMin = group, m
			}
		}
		return best

	case syntax.OpAlternate:
		var merged []string
		seen := make(map[string]struct{})
		for _, sub := range r.Sub {
			group := c.required(sub)
			if len(group) == 0 {
				return nil
			}
			for _, s := range group {
				if _, ok := seen[s]; !ok {
					seen[s] = struct{}{}
					merged = append(merged, s)
				}
			}
			if len(merged) > c.maxLiterals {
				return nil
			}
		}
		return merged

	case syntax.OpCapture, syntax.OpPlus:
		return c.required(r.Sub[0])

	case syntax.OpRepeat:
		// Simplify normally rewrites repeats, but handle them defensively.
		if r.Min >= 1 {
			return c.required(r.Sub[0])
		}
	}
	return nil
}

// charClass returns the class members as single-rune literals, provided
// every member is selective on its own.
func (c *config) charClass(ranges []rune) []string {
	if len(ranges) < 2 || (c.minLiteral > 1 && !c.cjkSingle) {
		return nil
	}
	total := 0
	for i := 0; i+1 < len(ranges); i += 2 {
		total += int(ranges[i+1]) - int(ranges[i]) + 1
		if total > c.maxLiterals {
			return nil
		}
	}
	out := make([]string, 0, total)
	seen := make(map[rune]struct{}, total)
	for i := 0; i+1 < len(ranges); i += 2 {
		for ch := ranges[i]; ch <= ranges[i+1]; ch++ {
			k := foldRune(ch)
			if _, ok := seen[k]; ok {
				continue
			}
			if !c.validSingle(k) {
				return nil
			}
			seen[k] = struct{}{}
			out = append(out, string(k))
		}
	}
	return out
}

// validLiteral accepts literals of at least minLiteral runes, and (by
// default) single CJK characters, which carry far more information than one
// ASCII letter.
func (c *config) validLiteral(s string) bool {
	n := utf8.RuneCountInString(s)
	if n >= c.minLiteral {
		return true
	}
	if n != 1 {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s)
	return c.validSingle(r)
}

func (c *config) validSingle(r rune) bool {
	return c.minLiteral <= 1 || (c.cjkSingle && isCJK(r))
}

func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

func minByteLen(group []string) int {
	m := len(group[0])
	for _, s := range group[1:] {
		if len(s) < m {
			m = len(s)
		}
	}
	return m
}
