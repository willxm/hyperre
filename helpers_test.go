package hyperre

var defaultConfig = newConfig(nil)

// extractLiterals with default options.
func extractLiterals(pattern string) []string { return defaultConfig.extractLiterals(pattern) }

// filterIdx returns the indexes of the candidates for text.
func filterIdx[T any](m *Matcher[T], text string) []int {
	var out []int
	m.forEach(text, func(i int) bool {
		out = append(out, i)
		return true
	})
	return out
}

// matchIdx returns the indexes of the matching items.
func matchIdx[T any](m *Matcher[T], text string) []int {
	var out []int
	for _, h := range m.FindAllIndex(text) {
		out = append(out, h.Index)
	}
	return out
}
