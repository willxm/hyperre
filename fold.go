package hyperre

import (
	"sync"
	"unicode"
	"unicode/utf8"
)

// foldTableSize covers the whole Basic Multilingual Plane. Every rune in the
// BMP folds to a key that is also in the BMP (the key is the minimum of the
// orbit, so it is never larger than the rune itself).
const foldTableSize = 0x10000

var (
	foldTableOnce sync.Once
	foldTable     []uint16
)

func initFoldTable() {
	t := make([]uint16, foldTableSize)
	for r := rune(0); r < foldTableSize; r++ {
		t[r] = uint16(foldRuneSlow(r))
	}
	foldTable = t
}

// foldRuneSlow returns the smallest rune in r's unicode.SimpleFold orbit.
func foldRuneSlow(r rune) rune {
	m := r
	for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
		if f < m {
			m = f
		}
	}
	return m
}

// foldRune returns the canonical case-folding key of r: the smallest rune of
// its unicode.SimpleFold orbit. Two runes that Go's regexp considers equal
// under (?i) always map to the same key.
//
// For ASCII letters the key is the upper-case letter.
func foldRune(r rune) rune {
	if r < utf8.RuneSelf {
		if 'a' <= r && r <= 'z' {
			return r - ('a' - 'A')
		}
		return r
	}
	if r < foldTableSize {
		foldTableOnce.Do(initFoldTable)
		return rune(foldTable[r])
	}
	return foldRuneSlow(r)
}

// foldRunes returns the folded form of runes as a string.
func foldRunes(runes []rune) string {
	buf := make([]byte, 0, len(runes))
	for _, r := range runes {
		buf = utf8.AppendRune(buf, foldRune(r))
	}
	return string(buf)
}
