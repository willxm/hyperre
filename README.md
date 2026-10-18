# hyperre

[![Go Reference](https://pkg.go.dev/badge/github.com/willxm/hyperre.svg)](https://pkg.go.dev/github.com/willxm/hyperre)
[![CI](https://github.com/willxm/hyperre/actions/workflows/ci.yml/badge.svg)](https://github.com/willxm/hyperre/actions/workflows/ci.yml)

English | [简体中文](README_zh.md)

**High-performance matching against massive sets of Go regular expressions.**
Match a text against thousands of regexps without running thousands of regexps.

`hyperre` extracts from each pattern the literal strings that *must* appear in any
matching text, indexes them in one Aho-Corasick automaton, scans the text once, and only
runs the patterns whose literals were found. Results are identical to Go's `regexp` —
**13–15× faster** on a typical rule set, and thousands of times faster when every rule has a literal.

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

Real numbers from `go run ./benchmarks`, single-threaded (GOMAXPROCS=1), on an Apple Silicon
Mac (Linux arm64 VM, 4 vCPU, Go 1.24.7). Rules are generated rule-like patterns —
Chinese/English words with gaps (`w1.{0,5}w2`), case-insensitive alternations
(`(?i)(w1|w2)\s*w3`), character classes (`w1[\p{Han}a-z]{0,3}w2`). Each text is ~300
bytes of mixed Chinese/English and 30% of them contain a hit. Every run checks hyperre's
results against `regexp`.

"regexp loop" runs every compiled `regexp.Regexp` on the text; "combined regexp" is
the usual one-big-`(p1)|(p2)|…` trick.

### Typical rule set: 5% of the rules have no literal

Rules like `\d{11}` have nothing to index and run on every text, so they set the floor.

**FindAll** — all matching rules, per text:

| rules | regexp loop | hyperre | speed-up | rules hyperre ran |
|---:|---:|---:|---:|---:|
| 100 | 128.2 µs | 9.0 µs | **14×** | 3 (3.3%) |
| 1k | 1.27 ms | 87.6 µs | **15×** | 45 (4.5%) |
| 10k | 12.62 ms | 982 µs | **13×** | 525 (5.2%) |
| 100k | 128.87 ms | 9.78 ms | **13×** | 4,957 (5.0%) |

**Match** — does any rule match, per text:

| rules | regexp loop | combined regexp | hyperre | vs loop | vs combined |
|---:|---:|---:|---:|---:|---:|
| 100 | 109.6 µs | 226.7 µs | 8.3 µs | **13×** | 27× |
| 1k | 1.05 ms | 3.48 ms | 72.2 µs | **15×** | 48× |
| 10k | 10.91 ms | 52.66 ms | 839.8 µs | **13×** | 63× |
| 100k | 107.66 ms | 2.24 s | 7.67 ms | **14×** | 292× |

### Every rule has a literal

Same generator with `-fallback 0` (e.g. a sensitive-word list). The scan cost stays
almost flat while the regexp loop grows linearly:

| rules | regexp loop (FindAll) | hyperre (FindAll) | speed-up | combined regexp (Match) | hyperre (Match) |
|---:|---:|---:|---:|---:|---:|
| 100 | 158.1 µs | 3.5 µs | **45×** | 256.2 µs | 3.5 µs |
| 1k | 1.28 ms | 4.7 µs | **272×** | 3.69 ms | 4.8 µs |
| 10k | 12.91 ms | 6.0 µs | **2,149×** | 57.92 ms | 5.8 µs |
| 100k | 131.44 ms | 18.2 µs | **7,218×** | 2.43 s | 15.9 µs |

### Build and memory (5% set)

| rules | `regexp.Compile` all | `hyperre.New` | heap, regexps | heap, hyperre |
|---:|---:|---:|---:|---:|
| 100 | 0.31 ms | 1.28 ms | 0.2 MB | 0.3 MB |
| 1k | 3.38 ms | 5.43 ms | 1.6 MB | 1.7 MB |
| 10k | 128 ms | 137 ms | 15.7 MB | 17.0 MB |
| 100k | 657 ms | 740 ms | 157.7 MB | 167.8 MB |

`hyperre.New` includes compiling every regexp; the index adds about 6–10% memory.

**Takeaways**

- hyperre's cost is one scan of the text (a few µs, nearly independent of the rule
  count) plus the rules it cannot index. With 5% such rules it is a steady 13–15× faster
  at every size; with none, the gap grows with the rule set, to over 7,000× at 100k rules.
- The combined-regexp trick is slower than the plain loop and degrades badly
  (2.2 s per text at 100k rules).
- The single most effective optimisation is to give rules without a literal a
  distinctive one (`订单\d{11}` instead of `\d{11}`).

Reproduce on your machine:

```sh
go run ./benchmarks                       # 100, 1k, 10k, 100k rules
go run ./benchmarks -fallback 0           # every rule has a literal
go run ./benchmarks -sizes 1000 -budget 5s
```

## Limitations

- Go RE2 semantics: no lookarounds or backreferences.
- Patterns without a literal run on every text. Giving them a distinctive literal (e.g.
  `订单\d{11}` instead of `\d{11}`) is the most effective optimisation.

## License

MIT
