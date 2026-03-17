package store

import (
	"database/sql"
	"fmt"

	"github.com/yourusername/mimir/internal/graph"
)

// BatchUpsertNodes inserts or updates nodes in a single transaction.
func (s *Store) BatchUpsertNodes(nodes []graph.Node) error {
	if len(nodes) == 0 {
		return nil
	}
	return s.Write(func(tx *sql.Tx) error {
		stmt, err := tx.Prepare(`
			INSERT INTO nodes (uid, name, kind, file_path, start_line, end_line, exported, package_path)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(uid) DO UPDATE SET
				name=excluded.name, kind=excluded.kind,
				file_path=excluded.file_path, start_line=excluded.start_line,
				end_line=excluded.end_line, exported=excluded.exported,
				package_path=excluded.package_path,
				indexed_at=datetime('now')`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, n := range nodes {
			exp := 0
			if n.Exported {
				exp = 1
			}
			if _, err := stmt.Exec(
				n.UID, n.Name, n.Kind, n.FilePath,
				n.StartLine, n.EndLine, exp, n.PackagePath,
			); err != nil {
				return fmt.Errorf("upsert node %s: %w", n.UID, err)
			}
		}
		return nil
	})
}

// BatchUpsertEdges inserts or updates edges in a single transaction.
func (s *Store) BatchUpsertEdges(edges []graph.Edge) error {
	if len(edges) == 0 {
		return nil
	}
	return s.Write(func(tx *sql.Tx) error {
		stmt, err := tx.Prepare(`
			INSERT INTO edges (id, from_uid, to_uid, type, confidence)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET confidence=excluded.confidence`)
		if err != nil {
			return err
		}
		defer stmt.Close()

		for _, e := range edges {
			id := graph.EdgeID(e.FromUID, e.ToUID, e.Type)
			if _, err := stmt.Exec(id, e.FromUID, e.ToUID, e.Type, e.Confidence); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteNodesByFile removes all nodes (and cascading edges) for a file.
func (s *Store) DeleteNodesByFile(filePath string) error {
	return s.Write(func(tx *sql.Tx) error {
		// Collect UIDs first so we can clean bm25_index and embeddings (no FK CASCADE there).
		rows, err := tx.Query("SELECT uid FROM nodes WHERE file_path = ?", filePath)
		if err != nil {
			return err
		}
		var uids []string
		for rows.Next() {
			var uid string
			if err := rows.Scan(&uid); err != nil {
				rows.Close()
				return err
			}
			uids = append(uids, uid)
		}
		rows.Close()

		for _, uid := range uids {
			tx.Exec("DELETE FROM bm25_index WHERE symbol_uid = ?", uid)
			tx.Exec("DELETE FROM node_embeddings WHERE uid = ?", uid)
			tx.Exec("DELETE FROM cluster_members WHERE symbol_uid = ?", uid)
			tx.Exec("DELETE FROM edges WHERE from_uid = ? OR to_uid = ?", uid, uid)
		}

		_, err = tx.Exec("DELETE FROM nodes WHERE file_path = ?", filePath)
		return err
	})
}

// AllNodes returns every node in the store.
func (s *Store) AllNodes() ([]graph.Node, error) {
	var nodes []graph.Node
	err := s.Read(func(db *sql.DB) error {
		rows, err := db.Query(`
			SELECT uid, name, kind, file_path, start_line, end_line, exported, package_path, COALESCE(cluster_id,'')
			FROM nodes`)
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

// AllEdges returns every edge in the store.
func (s *Store) AllEdges() ([]graph.Edge, error) {
	var edges []graph.Edge
	err := s.Read(func(db *sql.DB) error {
		rows, err := db.Query(`SELECT from_uid, to_uid, type, confidence FROM edges`)
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

// NodeCount returns the total number of nodes.
func (s *Store) NodeCount() (int, error) {
	var count int
	err := s.Read(func(db *sql.DB) error {
		return db.QueryRow("SELECT COUNT(*) FROM nodes").Scan(&count)
	})
	return count, err
}

// EdgeCount returns the total number of edges.
func (s *Store) EdgeCount() (int, error) {
	var count int
	err := s.Read(func(db *sql.DB) error {
		return db.QueryRow("SELECT COUNT(*) FROM edges").Scan(&count)
	})
	return count, err
}
