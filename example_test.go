package hyperre_test

import (
	"fmt"

	"github.com/willxm/hyperre"
)

func Example() {
	m := hyperre.MustNew([]string{
		`(?i)buy\s+cheap\s+\w+`,
		`敏感.{0,3}词`,
		`free.{0,10}money`,
		`1[3-9]\d{9}`,
	})

	text := "BUY cheap watches, call 13800138000. 这是敏感的词"
	fmt.Println(m.Match(text))
	fmt.Println(m.FindAll(text))
	// Output:
	// true
	// [(?i)buy\s+cheap\s+\w+ 敏感.{0,3}词 1[3-9]\d{9}]
}

// Rules can be any type: the matching rules come back with all their fields.
func Example_rules() {
	type Rule struct {
		ID     int
		Expr   string
		Action string
	}
	rules := []Rule{
		{1, `free.{0,10}money`, "block"},
		{2, `敏感.{0,3}词`, "review"},
		{3, `1[3-9]\d{9}`, "mask"},
	}
	m := hyperre.MustNew(rules, hyperre.WithPattern(func(r Rule) string { return r.Expr }))

	text := "free money! 这是敏感的词"
	for _, r := range m.FindAll(text) {
		fmt.Println(r.ID, r.Action)
	}
	if r, ok := m.Find(text); ok {
		fmt.Println("first:", r.ID)
	}
	for _, h := range m.FindAllIndex(text) {
		fmt.Println(h.Item.ID, text[h.Loc[0][0]:h.Loc[0][1]])
	}
	// Output:
	// 1 block
	// 2 review
	// first: 1
	// 1 free money
	// 2 敏感的词
}

func ExampleWithOnInvalid() {
	m := hyperre.MustNew([]string{`ok`, `(broken`},
		hyperre.WithIgnoreCase(),
		hyperre.WithOnInvalid(func(i int, pattern string, err error) {
			fmt.Println("skipped", i)
		}),
	)
	fmt.Println(m.Match("OK"), m.Len())
	// Output:
	// skipped 1
	// true 2
}

func ExampleMatcher_Filter() {
	m := hyperre.MustNew([]string{`abc.*def`, `hello`, `\d{11}`})
	// Only the patterns that may match; \d{11} has no literal and is
	// always a candidate.
	fmt.Println(m.Filter("say hello"))
	// Output:
	// [hello \d{11}]
}
