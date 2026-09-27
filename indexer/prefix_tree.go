package indexer

import (
	"sort"
	"strings"
)

const entrySimilarityThreshold = 0.7

// PrefixTree supports fuzzy prefix search for autocomplete.
type PrefixTree struct {
	root *prefixNode
}

type prefixNode struct {
	children map[rune]*prefixNode
	entries  []Entry
	isEnd    bool
}

// NewPrefixTree creates an empty prefix tree.
func NewPrefixTree() *PrefixTree {
	return &PrefixTree{root: &prefixNode{children: make(map[rune]*prefixNode)}}
}

// Insert adds an entry to the tree by its searchable name.
func (t *PrefixTree) Insert(name string, entry Entry) {
	node := t.root
	lower := strings.ToLower(name)
	for _, ch := range lower {
		if node.children == nil {
			node.children = make(map[rune]*prefixNode)
		}
		if node.children[ch] == nil {
			node.children[ch] = &prefixNode{children: make(map[rune]*prefixNode)}
		}
		node = node.children[ch]
	}
	node.isEnd = true
	node.entries = append(node.entries, entry)
}

// Delete removes entries for a given file path.
func (t *PrefixTree) Delete(filePath string) {
	t.deleteFromNode(t.root, filePath)
}

func (t *PrefixTree) deleteFromNode(node *prefixNode, filePath string) {
	if node == nil {
		return
	}
	filtered := node.entries[:0]
	for _, e := range node.entries {
		if e.GetLocation().URI != fileURI(filePath) && filePathFromURI(e.GetLocation().URI) != filePath {
			filtered = append(filtered, e)
		}
	}
	node.entries = filtered
	if len(node.entries) == 0 {
		node.isEnd = false
	}
	for _, child := range node.children {
		t.deleteFromNode(child, filePath)
	}
}

func fileURI(path string) string {
	if strings.HasPrefix(path, "/") {
		return "file://" + path
	}
	return "file:///" + path
}

// Search finds entries matching prefix with fuzzy threshold.
func (t *PrefixTree) Search(prefix string) []Entry {
	if prefix == "" {
		return nil
	}
	lower := strings.ToLower(prefix)
	node := t.root
	for _, ch := range lower {
		if node.children == nil || node.children[ch] == nil {
			return t.fuzzySearch(prefix)
		}
		node = node.children[ch]
	}
	return t.collectEntries(node)
}

func (t *PrefixTree) collectEntries(node *prefixNode) []Entry {
	var results []Entry
	var walk func(*prefixNode)
	walk = func(n *prefixNode) {
		if n.isEnd {
			results = append(results, n.entries...)
		}
		for _, child := range n.children {
			walk(child)
		}
	}
	walk(node)
	return dedupeEntries(results)
}

func (t *PrefixTree) fuzzySearch(prefix string) []Entry {
	var all []Entry
	t.collectAll(t.root, &all)
	lower := strings.ToLower(prefix)
	var matched []Entry
	for _, e := range all {
		name := strings.ToLower(e.GetName())
		if strings.HasPrefix(name, lower) {
			matched = append(matched, e)
		} else if similarity(name, lower) >= entrySimilarityThreshold {
			matched = append(matched, e)
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		return strings.ToLower(matched[i].GetName()) < strings.ToLower(matched[j].GetName())
	})
	return dedupeEntries(matched)
}

func (t *PrefixTree) collectAll(node *prefixNode, out *[]Entry) {
	if node.isEnd {
		*out = append(*out, node.entries...)
	}
	for _, child := range node.children {
		t.collectAll(child, out)
	}
}

func dedupeEntries(entries []Entry) []Entry {
	seen := make(map[string]bool)
	var result []Entry
	for _, e := range entries {
		loc := e.GetLocation()
		key := loc.URI + ":" + itoa(loc.Range.Start.Line) + ":" + e.GetName()
		if !seen[key] {
			seen[key] = true
			result = append(result, e)
		}
	}
	return result
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	if neg {
		s = "-" + s
	}
	return s
}

// similarity computes a simple ratio between two strings.
func similarity(a, b string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	shorter, longer := a, b
	if len(a) > len(b) {
		shorter, longer = b, a
	}
	matches := 0
	for i := 0; i < len(shorter); i++ {
		if i < len(longer) && shorter[i] == longer[i] {
			matches++
		}
	}
	return float64(matches) / float64(len(longer))
}
