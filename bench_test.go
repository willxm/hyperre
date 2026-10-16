package hyperre

import (
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"testing"
)

var benchCJK = []rune("的一是在不了有和人这中大为上个国我以要他时来用们生到作地于出就分对成会可主发年动同工也能下过子说产种面而方后多定行学法所民得经十三之进着等部度家电力里如水化高自二理起小物现实加量都两体制机当使点从业本去把性好应开它合还因由其些然前外天政四日那社义事平形相全表间样与关各重新线内数正心反你明看原又么利比或但质气第向道命此变条只没结解问意建月公无系军很情者最立代想已通并提直题党程展五果料象员革位入常文总次品式活设及管特件长求老头基资边流路级少图山统接知较将组见计别她手角期根论运农指几九区强放决西被干做必战先回则任取据处理")

func randWord(rng *rand.Rand) string {
	if rng.Intn(2) == 0 {
		n := 2 + rng.Intn(3)
		var sb strings.Builder
		for i := 0; i < n; i++ {
			sb.WriteRune(benchCJK[rng.Intn(len(benchCJK))])
		}
		return sb.String()
	}
	b := make([]byte, 4+rng.Intn(5))
	for i := range b {
		b[i] = byte('a' + rng.Intn(26))
	}
	return string(b)
}

// benchPatterns generates rule-like patterns; about 5% cannot be prefiltered.
func benchPatterns(n int) []string {
	rng := rand.New(rand.NewSource(42))
	out := make([]string, n)
	for i := range out {
		var p string
		switch r := rng.Intn(100); {
		case r < 5:
			p = fmt.Sprintf(`\d{%d}[a-z]+`, 6+rng.Intn(6))
		case r < 40:
			p = randWord(rng) + `.{0,5}` + randWord(rng)
		case r < 70:
			p = `(?i)(` + randWord(rng) + `|` + randWord(rng) + `)\s*` + randWord(rng)
		default:
			p = randWord(rng) + `[\p{Han}a-z]{0,3}` + randWord(rng)
		}
		out[i] = p
	}
	return out
}

func benchText(n int) string {
	rng := rand.New(rand.NewSource(7))
	var sb strings.Builder
	for sb.Len() < n {
		if rng.Intn(3) == 0 {
			sb.WriteString(" hello world ")
		} else {
			sb.WriteRune(benchCJK[rng.Intn(len(benchCJK))])
		}
	}
	return sb.String()
}

var benchSink int

func BenchmarkMatch(b *testing.B) {
	text := benchText(300)
	for _, n := range []int{100, 1000, 10000} {
		patterns := benchPatterns(n)
		res := make([]*regexp.Regexp, n)
		for i, p := range patterns {
			res[i] = regexp.MustCompile(p)
		}
		m := MustNew(patterns)

		b.Run(fmt.Sprintf("naive/n=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				c := 0
				for _, re := range res {
					if re.MatchString(text) {
						c++
					}
				}
				benchSink = c
			}
		})
		b.Run(fmt.Sprintf("FindAll/n=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				benchSink = len(m.FindAll(text))
			}
			b.ReportMetric(float64(len(m.Filter(text))), "candidates")
		})
		b.Run(fmt.Sprintf("Match/n=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				if m.Match(text) {
					benchSink++
				}
			}
		})
		b.Run(fmt.Sprintf("Filter/n=%d", n), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				benchSink = len(m.Filter(text))
			}
		})
	}
}

func BenchmarkNew(b *testing.B) {
	for _, n := range []int{1000, 10000, 100000} {
		patterns := benchPatterns(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				MustNew(patterns)
			}
		})
	}
}
