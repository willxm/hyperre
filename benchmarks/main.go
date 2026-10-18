// Command benchmarks compares hyperre with Go's standard regexp package on
// rule sets of increasing size.
//
//	go run ./benchmarks                      # 100, 1k, 10k, 100k rules
//	go run ./benchmarks -sizes 1000,10000 -budget 5s
//
// For every size it generates rule-like patterns (Chinese and English words
// with gaps, case-insensitive alternations, character classes, and ~5% rules
// without any literal), a corpus of texts of which ~30% contain a hit, and
// measures, single-threaded:
//
//   - build:   hyperre.New vs compiling every pattern with regexp.Compile
//   - FindAll: all matching rules — hyperre vs looping over every regexp
//   - Match:   any rule matches? — hyperre vs looping until the first hit
//     vs one combined regexp (p1)|(p2)|... (the usual stdlib trick)
//
// Results of hyperre are checked against the standard library on every text.
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/willxm/hyperre"
)

var (
	sizesFlag  = flag.String("sizes", "100,1000,10000,100000", "comma-separated rule counts")
	textsFlag  = flag.Int("texts", 500, "number of texts in the corpus")
	budgetFlag = flag.Duration("budget", 3*time.Second, "time budget per measurement")
	minRuns    = flag.Int("min", 20, "minimum texts per measurement")
	combined   = flag.Bool("combined", true, "also measure one combined stdlib regexp")
	verifyN    = flag.Int("verify", 100, "number of texts checked against the standard library")
	fallback   = flag.Int("fallback", 5, "percentage of rules without any literal (always executed)")
)

func main() {
	flag.Parse()
	runtime.GOMAXPROCS(1)
	fmt.Printf("# hyperre vs regexp — %s/%s, %s, %d CPU, GOMAXPROCS=1\n\n",
		runtime.GOOS, runtime.GOARCH, runtime.Version(), runtime.NumCPU())
	fmt.Printf("corpus: %d texts, ~300 bytes each, ~30%% contain a hit; %d%% of rules without a literal; budget %v per cell\n\n", *textsFlag, *fallback, *budgetFlag)

	var rows []row
	for _, s := range strings.Split(*sizesFlag, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		r := runSize(n)
		rows = append(rows, r)
		printRow(r)
	}
	printTables(rows)
}

type row struct {
	n                    int
	candidates           float64 // avg rules executed by hyperre per text
	hits                 float64 // avg matching rules per text
	buildStd, buildHyp   time.Duration
	memStd, memHyp       uint64
	findStd, findHyp     time.Duration
	matchStd, matchHyp   time.Duration
	buildComb, matchComb time.Duration
	combErr              string
}

func runSize(n int) row {
	rng := rand.New(rand.NewSource(int64(n)))
	patterns, samples := genRules(rng, n)
	texts := genTexts(rng, samples, *textsFlag)
	r := row{n: n}

	// --- build
	runtime.GC()
	before := heap()
	t0 := time.Now()
	res := make([]*regexp.Regexp, n)
	for i, p := range patterns {
		res[i] = regexp.MustCompile(p)
	}
	r.buildStd = time.Since(t0)
	r.memStd = heap() - before

	runtime.GC()
	before = heap()
	t0 = time.Now()
	m := hyperre.MustNew(patterns)
	r.buildHyp = time.Since(t0)
	r.memHyp = heap() - before

	// --- correctness + candidate stats
	var cands, hits, checked int
	for k, t := range texts {
		cands += len(m.Filter(t))
		if k >= *verifyN {
			continue
		}
		checked++
		got := m.FindAll(t)
		var want []string
		for i, re := range res {
			if re.MatchString(t) {
				want = append(want, patterns[i])
			}
		}
		if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
			fmt.Fprintf(os.Stderr, "MISMATCH n=%d text=%q\n got %q\nwant %q\n", n, t, got, want)
			os.Exit(1)
		}
		hits += len(want)
	}
	r.candidates = float64(cands) / float64(len(texts))
	r.hits = float64(hits) / float64(checked)

	// --- FindAll: all matching rules
	r.findStd = measure(texts, func(t string) int {
		c := 0
		for _, re := range res {
			if re.MatchString(t) {
				c++
			}
		}
		return c
	})
	r.findHyp = measure(texts, func(t string) int { return len(m.FindAll(t)) })

	// --- Match: any rule
	r.matchStd = measure(texts, func(t string) int {
		for _, re := range res {
			if re.MatchString(t) {
				return 1
			}
		}
		return 0
	})
	r.matchHyp = measure(texts, func(t string) int {
		if m.Match(t) {
			return 1
		}
		return 0
	})

	if *combined {
		t0 = time.Now()
		comb, err := regexp.Compile("(?:" + strings.Join(patterns, ")|(?:") + ")")
		r.buildComb = time.Since(t0)
		if err != nil {
			r.combErr = err.Error()
		} else {
			r.matchComb = measure(texts, func(t string) int {
				if comb.MatchString(t) {
					return 1
				}
				return 0
			})
		}
	}
	runtime.KeepAlive(res)
	runtime.KeepAlive(m)
	return r
}

var sink int

// measure runs fn over the corpus (cyclically) for at least the budget and
// at least *minRuns texts, and returns the average time per text.
func measure(texts []string, fn func(string) int) time.Duration {
	start := time.Now()
	runs := 0
	for {
		sink += fn(texts[runs%len(texts)])
		runs++
		if runs >= *minRuns && time.Since(start) >= *budgetFlag {
			break
		}
		if runs >= len(texts)*50 && runs >= *minRuns {
			break
		}
	}
	return time.Since(start) / time.Duration(runs)
}

func heap() uint64 {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.HeapAlloc
}

// ---------------------------------------------------------------- data

var cjk = []rune("的一是在不了有和人这中大为上个国我以要他时来用们生到作地于出就分对成会可主发年动同工也能下过子说产种面而方后多定行学法所民得经十三之进着等部度家电力里如水化高自二理起小物现实加量都两体制机当使点从业本去把性好应开它合还因由其些然前外天政四日那社义事平形相全表间样与关各重新线内数正心反你明看原又么利比或但质气第向道命此变条只没结解问意建月公无系军很情者最立代想已通并提直题党程展五果料象员革位入常文总次品式活设及管特件长求老头基资边流路级少图山统接知较将组见计别她手角期根论运农指几九区强放决西被干做必战先回则任取据处理")

var english = strings.Fields("the and of to in is you that it he was for on are as with his they at be this have from or one had by word but not what all were we when your can said there use an each which she do how their if will up other about out many then them these so some her would make like him into time has look two more write go see number no way could people my than first water been call who oil its now find long down day did get come made may part")

func randWord(rng *rand.Rand) string {
	if rng.Intn(2) == 0 {
		n := 2 + rng.Intn(3)
		var sb strings.Builder
		for i := 0; i < n; i++ {
			sb.WriteRune(cjk[rng.Intn(len(cjk))])
		}
		return sb.String()
	}
	b := make([]byte, 5+rng.Intn(4))
	for i := range b {
		b[i] = byte('a' + rng.Intn(26))
	}
	return string(b)
}

// genRules returns n patterns and, for each, a string it matches.
func genRules(rng *rand.Rand, n int) (patterns, samples []string) {
	patterns = make([]string, n)
	samples = make([]string, n)
	for i := range patterns {
		w1, w2, w3 := randWord(rng), randWord(rng), randWord(rng)
		switch r := rng.Intn(100); {
		case r < *fallback: // no literal: always executed
			d := 6 + rng.Intn(6)
			patterns[i] = fmt.Sprintf(`\d{%d}[a-z]+`, d)
			samples[i] = strings.Repeat("7", d) + "abc"
		case r < *fallback+(100-*fallback)*35/95:
			patterns[i] = w1 + `.{0,5}` + w2
			samples[i] = w1 + "xx" + w2
		case r < *fallback+(100-*fallback)*65/95:
			patterns[i] = `(?i)(` + w1 + `|` + w2 + `)\s*` + w3
			samples[i] = strings.ToUpper(w2) + " " + w3
		default:
			patterns[i] = w1 + `[\p{Han}a-z]{0,3}` + w2
			samples[i] = w1 + "ab" + w2
		}
	}
	return
}

// genTexts builds ~300-byte texts mixing Chinese characters and English
// words; ~30% of them embed the sample of a random rule.
func genTexts(rng *rand.Rand, samples []string, count int) []string {
	texts := make([]string, count)
	for i := range texts {
		var sb strings.Builder
		for sb.Len() < 300 {
			if rng.Intn(3) == 0 {
				sb.WriteString(" " + english[rng.Intn(len(english))] + " ")
			} else {
				sb.WriteRune(cjk[rng.Intn(len(cjk))])
			}
		}
		t := sb.String()
		if rng.Intn(10) < 3 {
			cut := strings.LastIndex(t[:len(t)/2], " ")
			if cut < 0 {
				cut = 0
			}
			t = t[:cut] + " " + samples[rng.Intn(len(samples))] + " " + t[cut:]
		}
		texts[i] = t
	}
	return texts
}

// ---------------------------------------------------------------- output

func printRow(r row) {
	fmt.Printf("n=%-6d build std=%v hyp=%v | FindAll std=%v hyp=%v (%.1fx) | Match std=%v hyp=%v (%.1fx) comb=%v %s | cand=%.1f hits=%.2f | mem std=%s hyp=%s\n",
		r.n, rd(r.buildStd), rd(r.buildHyp), rd(r.findStd), rd(r.findHyp), ratio(r.findStd, r.findHyp),
		rd(r.matchStd), rd(r.matchHyp), ratio(r.matchStd, r.matchHyp), rd(r.matchComb), r.combErr,
		r.candidates, r.hits, mb(r.memStd), mb(r.memHyp))
}

func printTables(rows []row) {
	fmt.Println("\n## FindAll — all matching rules, per text")
	fmt.Println("\n| rules | regexp loop | hyperre | speed-up | rules run by hyperre | QPS regexp | QPS hyperre |")
	fmt.Println("|---:|---:|---:|---:|---:|---:|---:|")
	for _, r := range rows {
		fmt.Printf("| %s | %s | %s | **%.0f×** | %.0f (%.2f%%) | %s | %s |\n", count(r.n), rd(r.findStd), rd(r.findHyp),
			ratio(r.findStd, r.findHyp), r.candidates, 100*r.candidates/float64(r.n), qps(r.findStd), qps(r.findHyp))
	}
	fmt.Println("\n## Match — does any rule match, per text")
	fmt.Println("\n| rules | regexp loop | combined regexp | hyperre | speed-up vs loop | vs combined |")
	fmt.Println("|---:|---:|---:|---:|---:|---:|")
	for _, r := range rows {
		comb, vs := rd(r.matchComb), fmt.Sprintf("%.0f×", ratio(r.matchComb, r.matchHyp))
		if r.combErr != "" {
			comb, vs = "compile error", "—"
		}
		fmt.Printf("| %s | %s | %s | %s | **%.0f×** | %s |\n", count(r.n), rd(r.matchStd), comb, rd(r.matchHyp),
			ratio(r.matchStd, r.matchHyp), vs)
	}
	fmt.Println("\n## Build and memory")
	fmt.Println("\n| rules | regexp.Compile all | hyperre.New | combined regexp compile | heap regexp | heap hyperre |")
	fmt.Println("|---:|---:|---:|---:|---:|---:|")
	for _, r := range rows {
		comb := rd(r.buildComb)
		if !*combined {
			comb = "—"
		}
		fmt.Printf("| %s | %s | %s | %s | %s | %s |\n", count(r.n), rd(r.buildStd), rd(r.buildHyp), comb, mb(r.memStd), mb(r.memHyp))
	}
}

func ratio(a, b time.Duration) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b)
}

func rd(d time.Duration) string {
	switch {
	case d == 0:
		return "—"
	case d < time.Microsecond:
		return fmt.Sprintf("%d ns", d.Nanoseconds())
	case d < time.Millisecond:
		return fmt.Sprintf("%.1f µs", float64(d)/1e3)
	case d < time.Second:
		return fmt.Sprintf("%.2f ms", float64(d)/1e6)
	default:
		return fmt.Sprintf("%.2f s", d.Seconds())
	}
}

func qps(d time.Duration) string {
	q := float64(time.Second) / float64(d)
	switch {
	case q >= 1e6:
		return fmt.Sprintf("%.1fM", q/1e6)
	case q >= 1e3:
		return fmt.Sprintf("%.1fk", q/1e3)
	default:
		return fmt.Sprintf("%.0f", q)
	}
}

func mb(b uint64) string { return fmt.Sprintf("%.1f MB", float64(b)/(1<<20)) }

func count(n int) string {
	switch {
	case n >= 1000 && n%1000 == 0:
		return fmt.Sprintf("%dk", n/1000)
	default:
		return strconv.Itoa(n)
	}
}
