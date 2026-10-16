# hyperre

[![Go Reference](https://pkg.go.dev/badge/github.com/willxm/hyperre.svg)](https://pkg.go.dev/github.com/willxm/hyperre)
[![CI](https://github.com/willxm/hyperre/actions/workflows/ci.yml/badge.svg)](https://github.com/willxm/hyperre/actions/workflows/ci.yml)

English | [简体中文](README_zh.md)

**High-performance matching against massive sets of Go regular expressions.**
Match a text against thousands of regexps without running thousands of regexps.

`hyperre` extracts from each pattern the literal strings that *must* appear in any
matching text, indexes them in one Aho-Corasick automaton, scans the text once, and only
runs the patterns whose literals were found. Results are identical to Go's `regexp` —
typically **10–15× faster** for large rule sets.

Pure Go, standard library only, Go 1.18+. The name is a nod to Intel's
[Hyperscan](https://github.com/intel/hyperscan), which solves the same problem in C.

```sh
go get github.com/willxm/hyperre
```

## Usage

### Patterns

```go
m, err := hyperre.New([]string{
    `(?i)buy\s+cheap\s+\w+`,
    `敏感.{0,3}词`,
    `1[3-9]\d{9}`,
})

m.Match(text)   // bool: does any pattern match?
m.FindAll(text) // []string: the patterns that match
```

### Your own rules

Pass your rule type directly and say where its pattern is. Matches come back as your
rules, with all their fields:

```go
type Rule struct {
    ID     int
    Expr   string
    Action string
}

m, err := hyperre.New(rules, hyperre.WithPattern(func(r Rule) string { return r.Expr }))

for _, r := range m.FindAll(text) { // []Rule
    fmt.Println(r.ID, r.Action)
}
```

### Methods

| | |
|---|---|
| `Match(text) bool` | does any item match (stops at the first hit) |
| `Find(text) (T, bool)` | the first item that matches |
| `FindAll(text) []T` | all items that match, in order |
| `FindAllIndex(text) []Hit[T]` | same, plus each item's index and match positions (`Loc`) |
| `Filter(text) []T` | prefilter only: items that *may* match, without running their regexps |
| `Len() int` | number of items |

### Options

All optional:

| Option | Default | |
|---|---|---|
| `WithPattern(func(T) string)` | — | where the pattern of an item is (not needed for `[]string`) |
| `WithIgnoreCase()` | off | make every pattern case-insensitive |
| `WithOnInvalid(func(i int, pattern string, err error))` | off | skip patterns that do not compile instead of returning an error |
| `WithMinLiteralLen(n)` | 2 | minimum literal length used for indexing |
| `WithCJKSingleChar(bool)` | true | accept one CJK character as a literal (`敏.*词` → `敏`) |
| `WithMaxLiterals(n)` | 1024 | max literals per pattern; larger classes/alternations always run |

The last three only tune the index (speed); they never change results.

### Concurrency and updates

A `Matcher` is immutable and safe for concurrent use. To change the rules, build a new
one and swap it in:

```go
var current atomic.Pointer[hyperre.Matcher[Rule]]

current.Store(hyperre.MustNew(rules, opt)) // on (re)load
current.Load().FindAll(text)                 // on each request
```

## Guarantee

> If a pattern matches a text with Go's `regexp`, the Matcher reports it, and `Filter`
> includes it.

The prefilter only skips patterns that provably cannot match. Patterns without a usable
literal (`a.*b`, `\d{11}`, `[a-z]+`) always run. Verified by unit tests, a randomized
comparison with `regexp` on hundreds of thousands of (pattern, text) pairs under every
option, and a fuzz test.

## How it works

Each pattern is parsed with `regexp/syntax` (the same flags `regexp.Compile` uses) and
walked recursively. Each node yields a set of literals such that *every* string it
matches contains at least one of them:

| Node | Example | Literals | Why it is safe |
|---|---|---|---|
| Literal | `abc` | `{abc}` | part of every match |
| Char class | `[中美]` | `{中, 美}` | every match is one of the members |
| Concat | `foo.*bar` | best of `{foo}`, `{bar}` | every part occurs; pick the most selective |
| Alternate | `abc\|def` | `{abc, def}` | only if *every* branch has literals |
| Capture / `+` / `{n,}` n≥1 | `(abc)+` | sub-expression's | occurs at least once |
| `?` `*` `{0,n}` anchors `.` … | `a?` | none | nothing is required |

**Case folding.** Literals and text are normalised to the canonical `unicode.SimpleFold`
key — the same normalisation `regexp/syntax` applies to `(?i)`. A plain `ToLower` would
silently miss matches such as `(?i)ss` vs `ſſ`, `(?i)σα` vs `ςα` or `(?i)вв` vs `ᲀᲀ`.

**Automaton.** The Aho-Corasick automaton is flattened into three pointer-free arrays,
compact and cheap for the garbage collector.

## Benchmarks

Synthetic rule-like patterns (Chinese/English words, `.{0,5}`, `(?i)` alternations, char
classes; ~5% without literals), 300-byte text, 2-core Intel Xeon @ 2.8 GHz, Go 1.24:

| patterns | naive loop | `FindAll` | speed-up | patterns run |
|---:|---:|---:|---:|---:|
| 100 | 448 µs | 44 µs | 10× | 6 |
| 1,000 | 4.5 ms | 0.33 ms | 14× | 50 |
| 10,000 | 52.7 ms | 4.1 ms | 13× | 504 |

`New` takes about 20 ms per 1,000 patterns (mostly `regexp.Compile`).

```sh
go test -bench . -run '^$'
```

## Limitations

- Go RE2 semantics: no lookarounds or backreferences.
- Patterns without a literal run on every text. Giving them a distinctive literal (e.g.
  `订单\d{11}` instead of `\d{11}`) is the most effective optimisation.

## License

MIT
