package hyperre

import "math"

// The automaton is built in two phases:
//
//  1. builder: a trie stored as a slice of nodes whose children are kept as
//     sorted edge slices. Literals are inserted, then failure links are
//     computed breadth-first.
//  2. automaton: the builder is flattened into three contiguous arrays
//     (nodes, edges, ids). This is the only structure kept after building:
//     it has no pointers, so it is cheap for the GC to scan, and lookups are
//     cache friendly.
//
// Output ids are not merged along failure links (that would multiply memory
// for large rule sets). Instead each node records whether some node on its
// failure chain has outputs, and the search only walks the chain when it
// does.

// smallFanout is the edge count up to which a linear scan is used instead of
// a binary search.
const smallFanout = 8

type buildEdge struct {
	r    rune
	next uint32
}

type buildNode struct {
	children      []buildEdge // sorted by r
	fail          uint32
	ids           []uint32
	hasFailOutput bool
}

type builder struct {
	nodes    []buildNode
	overflow bool
}

func newBuilder() *builder {
	return &builder{nodes: make([]buildNode, 1, 64)}
}

// add inserts an already case-folded literal that maps to item id.
func (b *builder) add(literal string, id uint32) {
	if b.overflow || literal == "" {
		return
	}
	n := uint32(0)
	for _, r := range literal {
		var ok bool
		if n, ok = b.addChild(n, r); !ok {
			b.overflow = true
			return
		}
	}
	ids := b.nodes[n].ids
	if len(ids) > 0 && ids[len(ids)-1] == id {
		return // same item, same literal
	}
	b.nodes[n].ids = append(ids, id)
}

func (b *builder) addChild(n uint32, r rune) (uint32, bool) {
	children := b.nodes[n].children
	idx, found := findBuildEdge(children, r)
	if found {
		return children[idx].next, true
	}
	if uint64(len(b.nodes)) >= math.MaxUint32 {
		return 0, false
	}
	next := uint32(len(b.nodes))
	b.nodes = append(b.nodes, buildNode{})
	children = append(children, buildEdge{})
	copy(children[idx+1:], children[idx:])
	children[idx] = buildEdge{r: r, next: next}
	b.nodes[n].children = children
	return next, true
}

func (b *builder) child(n uint32, r rune) (uint32, bool) {
	children := b.nodes[n].children
	idx, found := findBuildEdge(children, r)
	if !found {
		return 0, false
	}
	return children[idx].next, true
}

func findBuildEdge(children []buildEdge, r rune) (int, bool) {
	if len(children) <= smallFanout {
		for i, e := range children {
			if e.r >= r {
				return i, e.r == r
			}
		}
		return len(children), false
	}
	lo, hi := 0, len(children)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if children[mid].r < r {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, lo < len(children) && children[lo].r == r
}

// build computes failure links and returns the compact automaton, or nil if
// the trie cannot be represented (overflow).
func (b *builder) build() *automaton {
	if b.overflow {
		return nil
	}
	queue := make([]uint32, 0, len(b.nodes))
	for _, e := range b.nodes[0].children {
		b.nodes[e.next].fail = 0
		queue = append(queue, e.next)
	}
	for head := 0; head < len(queue); head++ {
		curr := queue[head]
		for _, e := range b.nodes[curr].children {
			fail := uint32(0)
			for f := b.nodes[curr].fail; ; f = b.nodes[f].fail {
				if next, ok := b.child(f, e.r); ok && next != e.next {
					fail = next
					break
				}
				if f == 0 {
					break
				}
			}
			b.nodes[e.next].fail = fail
			fn := &b.nodes[fail]
			b.nodes[e.next].hasFailOutput = fail != 0 && (len(fn.ids) > 0 || fn.hasFailOutput)
			queue = append(queue, e.next)
		}
	}
	return newAutomaton(b.nodes)
}

const (
	flagFailOutput = uint32(1) << 31
	idsCountMask   = flagFailOutput - 1
)

type node struct {
	first    uint32 // index of the first edge in automaton.edges
	count    uint32 // number of edges
	fail     uint32
	idsFirst uint32 // index of the first id in automaton.ids
	// idsCount: low 31 bits are the number of own ids; the high bit is set
	// when some node on the failure chain has ids.
	idsCount uint32
}

type edge struct {
	r    rune
	next uint32
}

type automaton struct {
	nodes []node
	edges []edge
	ids   []uint32
}

func newAutomaton(bn []buildNode) *automaton {
	edgeCount, idCount := uint64(0), uint64(0)
	for i := range bn {
		if uint64(len(bn[i].ids)) > uint64(idsCountMask) {
			return nil
		}
		edgeCount += uint64(len(bn[i].children))
		idCount += uint64(len(bn[i].ids))
	}
	if edgeCount > math.MaxUint32 || idCount > math.MaxUint32 {
		return nil
	}
	a := &automaton{
		nodes: make([]node, len(bn)),
		edges: make([]edge, 0, edgeCount),
		ids:   make([]uint32, 0, idCount),
	}
	for i := range bn {
		b := &bn[i]
		n := node{fail: b.fail}
		if b.hasFailOutput {
			n.idsCount |= flagFailOutput
		}
		if len(b.ids) > 0 {
			n.idsFirst = uint32(len(a.ids))
			n.idsCount |= uint32(len(b.ids))
			a.ids = append(a.ids, b.ids...)
		}
		n.first = uint32(len(a.edges))
		n.count = uint32(len(b.children))
		for _, e := range b.children {
			a.edges = append(a.edges, edge{r: e.r, next: e.next})
		}
		a.nodes[i] = n
		// Release builder memory as we go to lower the peak.
		b.children, b.ids = nil, nil
	}
	return a
}

func (a *automaton) child(n uint32, r rune) (uint32, bool) {
	nd := &a.nodes[n]
	edges := a.edges[nd.first : nd.first+nd.count]
	if len(edges) <= smallFanout {
		for _, e := range edges {
			if e.r >= r {
				return e.next, e.r == r
			}
		}
		return 0, false
	}
	lo, hi := 0, len(edges)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if edges[mid].r < r {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo < len(edges) && edges[lo].r == r {
		return edges[lo].next, true
	}
	return 0, false
}

func (a *automaton) emit(sc *scratch, n uint32) {
	nd := &a.nodes[n]
	c := nd.idsCount & idsCountMask
	if c == 0 {
		return
	}
	for _, id := range a.ids[nd.idsFirst : nd.idsFirst+c] {
		sc.add(id)
	}
}

// search scans text (folding each rune) and records every id whose literal
// occurs in it.
func (a *automaton) search(text string, sc *scratch) {
	var n uint32
	for _, r := range text {
		r = foldRune(r)
		for {
			if next, ok := a.child(n, r); ok {
				n = next
				break
			}
			if n == 0 {
				break
			}
			n = a.nodes[n].fail
		}
		if n == 0 {
			continue
		}
		a.emit(sc, n)
		for f := n; a.nodes[f].idsCount&flagFailOutput != 0; {
			f = a.nodes[f].fail
			a.emit(sc, f)
		}
	}
}

// scratch holds per-search state. It is pooled by Matcher.
type scratch struct {
	seen []uint64 // bitset over item indexes
	ids  []uint32 // ids set in seen, in discovery order
}

func newScratch(items int) *scratch {
	return &scratch{seen: make([]uint64, (items+63)/64)}
}

func (sc *scratch) add(id uint32) {
	w, bit := id>>6, uint64(1)<<(id&63)
	if sc.seen[w]&bit != 0 {
		return
	}
	sc.seen[w] |= bit
	sc.ids = append(sc.ids, id)
}

func (sc *scratch) reset() {
	for _, id := range sc.ids {
		sc.seen[id>>6] = 0
	}
	sc.ids = sc.ids[:0]
}
