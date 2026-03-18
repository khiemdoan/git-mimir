package store

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"

	"github.com/thuongh2/git-mimir/internal/graph"
)

// ImpactRow is one row from an impact query.
type ImpactRow struct {
	graph.Node
	Depth      int
	Confidence float64
}

// SearchResult is a node returned by hybrid search with a relevance score.
type SearchResult struct {
	graph.Node
	Score float64
}

// BM25Result holds a UID and its BM25 relevance score.
type BM25Result struct {
	UID   string
	Score float64
}

// VectorResult holds a UID and its vector distance.
type VectorResult struct {
	UID      string
	Distance float64
}

// QuerySymbol finds all nodes with the given name.
func (s *Store) QuerySymbol(name string) ([]graph.Node, error) {
	var nodes []graph.Node
	err := s.Read(func(db *sql.DB) error {
		rows, err := db.Query(`
			SELECT uid, name, kind, file_path, start_line, end_line, exported, package_path, COALESCE(cluster_id,'')
			FROM nodes WHERE name = ?`, name)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var n graph.Node
			var exp int
			if err := rows.Scan(&n.UID, &n.Name, &n.Kind, &n.FilePath,
				&n.StartLine, &n.EndLine, &exp, &n.PackagePath, &n.ClusterID); err != nil {
				return err
			}
			n.Exported = exp == 1
			nodes = append(nodes, n)
		}
		return rows.Err()
	})
	return nodes, err
}

// QueryByFile returns all nodes in a file.
func (s *Store) QueryByFile(filePath string) ([]graph.Node, error) {
	var nodes []graph.Node
	err := s.Read(func(db *sql.DB) error {
		rows, err := db.Query(`
			SELECT uid, name, kind, file_path, start_line, end_line, exported, package_path, COALESCE(cluster_id,'')
			FROM nodes WHERE file_path = ?`, filePath)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var n graph.Node
			var exp int
			if err := rows.Scan(&n.UID, &n.Name, &n.Kind, &n.FilePath,
				&n.StartLine, &n.EndLine, &exp, &n.PackagePath, &n.ClusterID); err != nil {
				return err
			}
			n.Exported = exp == 1
			nodes = append(nodes, n)
		}
		return rows.Err()
	})
	return nodes, err
}

// QueryNodeByUID fetches a single node by UID.
func (s *Store) QueryNodeByUID(uid string) (*graph.Node, error) {
	var n graph.Node
	err := s.Read(func(db *sql.DB) error {
		var exp int
		return db.QueryRow(`
			SELECT uid, name, kind, file_path, start_line, end_line, exported, package_path, COALESCE(cluster_id,'')
			FROM nodes WHERE uid = ?`, uid).
			Scan(&n.UID, &n.Name, &n.Kind, &n.FilePath,
				&n.StartLine, &n.EndLine, &exp, &n.PackagePath, &n.ClusterID)
	})
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &n, nil
}

// QueryEdgesFrom returns all edges originating from a node UID.
func (s *Store) QueryEdgesFrom(uid string) ([]graph.Edge, error) {
	return s.queryEdges("from_uid", uid)
}

// QueryEdgesTo returns all edges pointing to a node UID.
func (s *Store) QueryEdgesTo(uid string) ([]graph.Edge, error) {
	return s.queryEdges("to_uid", uid)
}

func (s *Store) queryEdges(col, uid string) ([]graph.Edge, error) {
	var edges []graph.Edge
	err := s.Read(func(db *sql.DB) error {
		rows, err := db.Query(fmt.Sprintf(
			`SELECT from_uid, to_uid, type, confidence FROM edges WHERE %s = ?`, col), uid)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e graph.Edge
			if err := rows.Scan(&e.FromUID, &e.ToUID, &e.Type, &e.Confidence); err != nil {
				return err
			}
			edges = append(edges, e)
		}
		return rows.Err()
	})
	return edges, err
}

// QueryImpact traverses the call graph recursively via a CTE.
// direction: "downstream" (callers of target) or "upstream" (callees of target)
func (s *Store) QueryImpact(uid, direction string, minConf float64, maxDepth int) ([]ImpactRow, error) {
	fromCol, toCol := "from_uid", "to_uid"
	if direction == "upstream" {
		fromCol, toCol = "to_uid", "from_uid"
	}

	q := fmt.Sprintf(`
		WITH RECURSIVE impact(uid, depth, path_confidence) AS (
			SELECT %s, 1, confidence
			FROM edges
			WHERE %s = ? AND type IN ('CALLS','IMPORTS') AND confidence >= ?
			UNION
			SELECT e.%s, i.depth + 1, e.confidence * i.path_confidence
			FROM edges e
			JOIN impact i ON e.%s = i.uid
			WHERE i.depth < ? AND e.confidence >= ?
			  AND e.type IN ('CALLS','IMPORTS')
		)
		SELECT DISTINCT n.uid, n.name, n.kind, n.file_path,
		                n.start_line, n.end_line, n.exported, COALESCE(n.package_path,''), COALESCE(n.cluster_id,''),
		                i.depth, i.path_confidence
		FROM impact i
		JOIN nodes n ON n.uid = i.uid
		ORDER BY i.depth, i.path_confidence DESC`,
		toCol, fromCol, toCol, fromCol)

	var rows []ImpactRow
	err := s.Read(func(db *sql.DB) error {
		r, err := db.Query(q, uid, minConf, maxDepth, minConf)
		if err != nil {
			return err
		}
		defer r.Close()
		for r.Next() {
			var row ImpactRow
			var exp int
			if err := r.Scan(&row.UID, &row.Name, &row.Kind, &row.FilePath,
				&row.StartLine, &row.EndLine, &exp, &row.PackagePath, &row.ClusterID,
				&row.Depth, &row.Confidence); err != nil {
				return err
			}
			row.Exported = exp == 1
			rows = append(rows, row)
		}
		return r.Err()
	})
	return rows, err
}

// HybridSearch performs BM25 + vector RRF search.
// If queryEmb is nil, falls back to BM25 only.
func (s *Store) HybridSearch(terms []string, queryEmb []float32, limit int) ([]SearchResult, error) {
	const rrfK = 60

	bm25Results, err := s.bm25Search(terms, limit*2)
	if err != nil {
		return nil, err
	}

	var vecResults []VectorResult
	if queryEmb != nil {
		vecResults, err = s.VectorSearch(queryEmb, limit*2)
		if err != nil {
			return nil, err
		}
	}

	scores := map[string]float64{}
	for rank, r := range bm25Results {
		scores[r.UID] += 1.0 / float64(rrfK+rank+1)
	}
	for rank, r := range vecResults {
		scores[r.UID] += 1.0 / float64(rrfK+rank+1)
	}

	type scored struct {
		uid   string
		score float64
	}
	var ranked []scored
	for uid, score := range scores {
		ranked = append(ranked, scored{uid, score})
	}
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	if len(ranked) > limit {
		ranked = ranked[:limit]
	}

	uids := make([]string, len(ranked))
	scoreMap := make(map[string]float64, len(ranked))
	for i, r := range ranked {
		uids[i] = r.uid
		scoreMap[r.uid] = r.score
	}
	return s.hydrateNodes(uids, scoreMap)
}

func (s *Store) bm25Search(terms []string, limit int) ([]BM25Result, error) {
	if len(terms) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(terms))
	placeholders = placeholders[:len(placeholders)-1]

	q := fmt.Sprintf(`
		SELECT symbol_uid, SUM(tfidf) AS score
		FROM bm25_index
		WHERE term IN (%s)
		GROUP BY symbol_uid
		ORDER BY score DESC
		LIMIT ?`, placeholders)

	args := make([]any, len(terms)+1)
	for i, t := range terms {
		args[i] = strings.ToLower(t)
	}
	args[len(terms)] = limit

	var results []BM25Result
	err := s.Read(func(db *sql.DB) error {
		rows, err := db.Query(q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r BM25Result
			if err := rows.Scan(&r.UID, &r.Score); err != nil {
				return err
			}
			results = append(results, r)
		}
		return rows.Err()
	})
	return results, err
}

func (s *Store) hydrateNodes(uids []string, scores map[string]float64) ([]SearchResult, error) {
	if len(uids) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(uids))
	placeholders = placeholders[:len(placeholders)-1]

	args := make([]any, len(uids))
	for i, u := range uids {
		args[i] = u
	}

	var results []SearchResult
	err := s.Read(func(db *sql.DB) error {
		rows, err := db.Query(fmt.Sprintf(`
			SELECT uid, name, kind, file_path, start_line, end_line, exported, COALESCE(package_path,''), COALESCE(cluster_id,'')
			FROM nodes WHERE uid IN (%s)`, placeholders), args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var n graph.Node
			var exp int
			if err := rows.Scan(&n.UID, &n.Name, &n.Kind, &n.FilePath,
				&n.StartLine, &n.EndLine, &exp, &n.PackagePath, &n.ClusterID); err != nil {
				return err
			}
			n.Exported = exp == 1
			results = append(results, SearchResult{Node: n, Score: scores[n.UID]})
		}
		return rows.Err()
	})
	// Re-sort by score (DB order not guaranteed with IN)
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	return results, err
}

// UpsertBM25Index inserts TF-IDF entries for a node.
func (s *Store) UpsertBM25Index(uid string, terms map[string]float64) error {
	if len(terms) == 0 {
		return nil
	}
	return s.Write(func(tx *sql.Tx) error {
		stmt, err := tx.Prepare(`INSERT INTO bm25_index(term, symbol_uid, tfidf)
			VALUES(?,?,?) ON CONFLICT(term,symbol_uid) DO UPDATE SET tfidf=excluded.tfidf`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for term, score := range terms {
			if _, err := stmt.Exec(term, uid, score); err != nil {
				return err
			}
		}
		return nil
	})
}
