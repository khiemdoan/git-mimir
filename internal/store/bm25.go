package store

import (
	"math"
	"strings"
	"unicode"

	"github.com/yourusername/mimir/internal/graph"
)

// IndexNodesBM25 builds TF-IDF index entries for a batch of nodes.
// The text for each node is: name + kind tokens.
func (s *Store) IndexNodesBM25(nodes []graph.Node) error {
	// Compute IDF across the batch (simplified: per-batch doc freq)
	docFreq := make(map[string]int)
	N := len(nodes)

	type docTerms struct {
		uid   string
		terms map[string]int
	}
	docs := make([]docTerms, N)

	for i, n := range nodes {
		toks := tokenize(n.Name + " " + n.Kind + " " + n.FilePath)
		tf := make(map[string]int, len(toks))
		for _, t := range toks {
			tf[t]++
		}
		docs[i] = docTerms{uid: n.UID, terms: tf}
		for t := range tf {
			docFreq[t]++
		}
	}

	for _, doc := range docs {
		scores := make(map[string]float64, len(doc.terms))
		total := 0
		for _, cnt := range doc.terms {
			total += cnt
		}
		for term, cnt := range doc.terms {
			tf := float64(cnt) / float64(total)
			idf := math.Log(float64(N+1)/float64(docFreq[term]+1)) + 1
			scores[term] = tf * idf
		}
		if err := s.UpsertBM25Index(doc.uid, scores); err != nil {
			return err
		}
	}
	return nil
}

// tokenize splits a string into lower-case tokens, splitting on camelCase and non-alpha.
func tokenize(s string) []string {
	// First split on non-alphanumeric
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})

	var tokens []string
	for _, part := range parts {
		// Split camelCase
		for _, sub := range splitCamel(part) {
			t := strings.ToLower(sub)
			if len(t) >= 2 {
				tokens = append(tokens, t)
			}
		}
	}
	return tokens
}

func splitCamel(s string) []string {
	var result []string
	start := 0
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			result = append(result, s[start:i])
			start = i
		}
	}
	result = append(result, s[start:])
	return result
}
