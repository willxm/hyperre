// Package hyperre matches a text against many Go regular expressions
// quickly.
//
// Instead of running every regular expression on every text, it extracts from
// each pattern literal strings that must appear in any text the pattern can
// match, indexes them all in one Aho-Corasick automaton, scans the text once,
// and only runs the patterns whose literals were found. Patterns without a
// usable literal (such as `a.*b` or `\d{11}`) always run.
//
// Results are exactly those of Go's regexp package.
//
// Plain patterns:
//
//	m, err := hyperre.New([]string{`敏感.{0,3}词`, `(?i)buy\s+cheap`})
//	m.Match(text)   // true if any pattern matches
//	m.FindAll(text) // the patterns that match
//
// Your own rule type, with any custom fields:
//
//	type Rule struct{ ID int; Expr string; Level int }
//	m, err := hyperre.New(rules, hyperre.WithPattern(func(r Rule) string { return r.Expr }))
//	for _, r := range m.FindAll(text) {
//		fmt.Println(r.ID, r.Level)
//	}
//
// A Matcher is immutable and safe for concurrent use. To change the rules,
// build a new Matcher and swap it in (for example with atomic.Pointer).
package hyperre

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"sync"
)

// Matcher matches texts against a set of items, each carrying a regular
// expression.
type Matcher[T any] struct {
	items    []T
	res      []*regexp.Regexp // nil for skipped invalid patterns
	fallback []uint32         // ascending indexes of patterns that always run
	ac       *automaton
	pool     sync.Pool // *scratch
}

// Hit is an item that matched, with the positions of its matches.
type Hit[T any] struct {
	// Item is the matching item, as passed to New.
	Item T
	// Index is the item's index in the slice passed to New.
	Index int
	// Loc holds the byte ranges of all non-overlapping matches, as returned
	// by regexp.FindAllStringIndex: text[Loc[k][0]:Loc[k][1]].
	Loc [][]int
}

// New builds a Matcher over items. Items are plain pattern strings, or any
// type together with WithPattern. Patterns use Go regular expression syntax
// (RE2, see regexp/syntax). New returns an error if a pattern does not
// compile, unless WithOnInvalid is used.
func New[T any](items []T, opts ...Option) (*Matcher[T], error) {
	c := newConfig(opts)
	patternOf, err := patternFunc[T](c)
	if err != nil {
		return nil, err
	}

	// Initialise the fold table now so that the first match does not pay
	// for it.
	foldTableOnce.Do(initFoldTable)

	m := &Matcher[T]{
		items: append([]T(nil), items...),
		res:   make([]*regexp.Regexp, len(items)),
	}
	b := newBuilder()
	indexed := 0
	for i := range m.items {
		p := patternOf(m.items[i])
		if c.ignoreCase {
			p = "(?i)" + p
		}
		re, err := regexp.Compile(p)
		if err != nil {
			if c.onInvalid == nil {
				return nil, fmt.Errorf("hyperre: item %d: %w", i, err)
			}
			c.onInvalid(i, p, err)
			continue // never a candidate
		}
		m.res[i] = re

		lits := c.extractLiterals(p)
		if len(lits) == 0 || uint64(i) >= math.MaxUint32 {
			m.fallback = append(m.fallback, uint32(i))
			continue
		}
		for _, l := range lits {
			b.add(l, uint32(i))
		}
		indexed++
	}
	if indexed > 0 {
		if m.ac = b.build(); m.ac == nil {
			// The automaton could not be represented; run everything.
			// Slower, never wrong.
			m.fallback = m.fallback[:0]
			for i, re := range m.res {
				if re != nil {
					m.fallback = append(m.fallback, uint32(i))
				}
			}
		}
	}
	n := len(items)
	m.pool.New = func() interface{} { return newScratch(n) }
	return m, nil
}

// MustNew is like New but panics on error.
func MustNew[T any](items []T, opts ...Option) *Matcher[T] {
	m, err := New(items, opts...)
	if err != nil {
		panic(err)
	}
	return m
}

func patternFunc[T any](c *config) (func(T) string, error) {
	if c.pattern != nil {
		fn, ok := c.pattern.(func(T) string)
		if !ok {
			var zero T
			return nil, fmt.Errorf("hyperre: WithPattern function takes %T, but items are %T", c.pattern, zero)
		}
		return fn, nil
	}
	if fn, ok := any(func(s string) string { return s }).(func(T) string); ok {
		return fn, nil
	}
	var zero T
	return nil, fmt.Errorf("hyperre: items of type %T need WithPattern", zero)
}

// Len returns the number of items.
func (m *Matcher[T]) Len() int { return len(m.items) }

// Match reports whether any item matches text. It stops at the first match.
func (m *Matcher[T]) Match(text string) bool {
	_, ok := m.Find(text)
	return ok
}

// Find returns the first item (in order) that matches text.
func (m *Matcher[T]) Find(text string) (T, bool) {
	var (
		item  T
		found bool
	)
	m.forEach(text, func(i int) bool {
		if m.res[i].MatchString(text) {
			item, found = m.items[i], true
			return false
		}
		return true
	})
	return item, found
}

// FindAll returns, in order, the items that match text.
func (m *Matcher[T]) FindAll(text string) []T {
	var out []T
	m.forEach(text, func(i int) bool {
		if m.res[i].MatchString(text) {
			out = append(out, m.items[i])
		}
		return true
	})
	return out
}

// FindAllIndex returns, in order, the items that match text, with their
// index and the positions of their matches.
func (m *Matcher[T]) FindAllIndex(text string) []Hit[T] {
	var out []Hit[T]
	m.forEach(text, func(i int) bool {
		// MatchString is much cheaper than FindAllStringIndex, and most
		// candidates do not match.
		if m.res[i].MatchString(text) {
			out = append(out, Hit[T]{Item: m.items[i], Index: i, Loc: m.res[i].FindAllStringIndex(text, -1)})
		}
		return true
	})
	return out
}

// Filter returns, in order, the items that may match text, without running
// their regular expressions: the prefilter step on its own. Every item that
// matches text is included; some returned items may not match.
func (m *Matcher[T]) Filter(text string) []T {
	var out []T
	m.forEach(text, func(i int) bool {
		out = append(out, m.items[i])
		return true
	})
	return out
}

// forEach calls fn with the index of every candidate for text, in ascending
// order, until fn returns false.
func (m *Matcher[T]) forEach(text string, fn func(i int) bool) {
	if m.ac == nil {
		for _, i := range m.fallback {
			if !fn(int(i)) {
				return
			}
		}
		return
	}

	sc := m.pool.Get().(*scratch)
	defer func() {
		sc.reset()
		m.pool.Put(sc)
	}()
	m.ac.search(text, sc)
	ids := sc.ids
	sortUint32s(ids)

	// Merge the two ascending, disjoint lists.
	fb := m.fallback
	i, j := 0, 0
	for i < len(ids) || j < len(fb) {
		var next uint32
		if j >= len(fb) || (i < len(ids) && ids[i] < fb[j]) {
			next = ids[i]
			i++
		} else {
			next = fb[j]
			j++
		}
		if !fn(int(next)) {
			return
		}
	}
}

type uint32s []uint32

func (x uint32s) Len() int           { return len(x) }
func (x uint32s) Less(i, j int) bool { return x[i] < x[j] }
func (x uint32s) Swap(i, j int)      { x[i], x[j] = x[j], x[i] }

func sortUint32s(x []uint32) {
	if len(x) <= 12 {
		for i := 1; i < len(x); i++ {
			for j := i; j > 0 && x[j] < x[j-1]; j-- {
				x[j], x[j-1] = x[j-1], x[j]
			}
		}
		return
	}
	sort.Sort(uint32s(x))
}
