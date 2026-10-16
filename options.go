package hyperre

// Option customises New. All options are optional.
type Option func(*config)

type config struct {
	pattern     any // func(T) string, set by WithPattern
	ignoreCase  bool
	onInvalid   func(index int, pattern string, err error)
	minLiteral  int
	cjkSingle   bool
	maxLiterals int
}

func newConfig(opts []Option) *config {
	c := &config{
		minLiteral:  2,
		cjkSingle:   true,
		maxLiterals: 1024,
	}
	for _, o := range opts {
		if o != nil {
			o(c)
		}
	}
	if c.minLiteral < 1 {
		c.minLiteral = 1
	}
	if c.maxLiterals < 1 {
		c.maxLiterals = 1
	}
	return c
}

// WithPattern tells New how to get the regular expression of an item, when
// the items are not plain strings:
//
//	type Rule struct{ ID int; Expr string; Level int }
//	m, err := hyperre.New(rules, hyperre.WithPattern(func(r Rule) string { return r.Expr }))
//
// The function's parameter type must be the item type passed to New.
func WithPattern[T any](fn func(item T) string) Option {
	return func(c *config) { c.pattern = fn }
}

// WithIgnoreCase makes every pattern case-insensitive, as if it started
// with (?i).
func WithIgnoreCase() Option {
	return func(c *config) { c.ignoreCase = true }
}

// WithOnInvalid makes New skip patterns that do not compile instead of
// failing; fn is called for each of them. Skipped items never match.
func WithOnInvalid(fn func(index int, pattern string, err error)) Option {
	return func(c *config) { c.onInvalid = fn }
}

// WithMinLiteralLen sets the minimum length, in characters, of a literal
// used to index a pattern (default 2). Patterns with only shorter literals,
// such as `a.*b`, are not indexed and run on every text. Raising it makes the
// index more selective but leaves more patterns unindexed.
func WithMinLiteralLen(n int) Option {
	return func(c *config) { c.minLiteral = n }
}

// WithCJKSingleChar sets whether a single CJK (Chinese, Japanese, Korean)
// character is accepted as a literal even when shorter than the minimum
// literal length (default true): one CJK character is selective enough on
// its own, so `敏.*词` can be indexed by "敏".
func WithCJKSingleChar(enabled bool) Option {
	return func(c *config) { c.cjkSingle = enabled }
}

// WithMaxLiterals caps how many literals a single pattern may add to the
// index (default 1024). Patterns that would need more, such as large
// character classes or alternations, run on every text instead.
func WithMaxLiterals(n int) Option {
	return func(c *config) { c.maxLiterals = n }
}
