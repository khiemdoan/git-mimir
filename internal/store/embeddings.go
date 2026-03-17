package store

import (
	"database/sql"
	"encoding/binary"
	"math"
	"sort"
)

// Float32ToBytes encodes a float32 slice as little-endian bytes (sqlite-vec format).
func Float32ToBytes(f []float32) []byte {
	b := make([]byte, len(f)*4)
	for i, v := range f {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(v))
	}
	return b
}

// BytesToFloat32 decodes little-endian bytes to a float32 slice.
func BytesToFloat32(b []byte) []float32 {
	f := make([]float32, len(b)/4)
	for i := range f {
		f[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return f
}

// UpsertEmbedding stores an embedding for a node UID.
func (s *Store) UpsertEmbedding(uid string, emb []float32) error {
	return s.Write(func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO node_embeddings(uid, embedding) VALUES(?,?)
			ON CONFLICT(uid) DO UPDATE SET embedding=excluded.embedding`,
			uid, Float32ToBytes(emb))
		return err
	})
}

// BatchUpdateEmbeddings stores multiple embeddings in one transaction.
func (s *Store) BatchUpdateEmbeddings(embeddings map[string][]float32) error {
	if len(embeddings) == 0 {
		return nil
	}
	return s.Write(func(tx *sql.Tx) error {
		stmt, err := tx.Prepare(`
			INSERT INTO node_embeddings(uid, embedding) VALUES(?,?)
			ON CONFLICT(uid) DO UPDATE SET embedding=excluded.embedding`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for uid, emb := range embeddings {
			if _, err := stmt.Exec(uid, Float32ToBytes(emb)); err != nil {
				return err
			}
		}
		return nil
	})
}

// VectorSearch computes cosine similarity in Go over stored embeddings.
// Returns the top-k nearest neighbors by cosine similarity.
func (s *Store) VectorSearch(query []float32, k int) ([]VectorResult, error) {
	type row struct {
		uid string
		emb []float32
	}

	var candidates []row
	err := s.Read(func(db *sql.DB) error {
		rows, err := db.Query(`SELECT uid, embedding FROM node_embeddings LIMIT 10000`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var uid string
			var blob []byte
			if err := rows.Scan(&uid, &blob); err != nil {
				return err
			}
			if len(blob) == 0 {
				continue
			}
			candidates = append(candidates, row{uid: uid, emb: BytesToFloat32(blob)})
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	qNorm := norm(query)
	if qNorm == 0 {
		return nil, nil
	}

	type scored struct {
		uid  string
		dist float64
	}
	results := make([]scored, 0, len(candidates))
	for _, c := range candidates {
		sim := cosineSim(query, qNorm, c.emb)
		// Convert similarity to distance (1 - sim) so lower = closer
		results = append(results, scored{uid: c.uid, dist: 1 - sim})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].dist < results[j].dist })
	if len(results) > k {
		results = results[:k]
	}

	out := make([]VectorResult, len(results))
	for i, r := range results {
		out[i] = VectorResult{UID: r.uid, Distance: r.dist}
	}
	return out, nil
}

// UpsertEmbedCache stores a text hash → embedding in embed_cache.
func (s *Store) UpsertEmbedCache(textHash string, emb []float32) error {
	return s.Write(func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO embed_cache(text_hash, embedding) VALUES(?,?)
			ON CONFLICT(text_hash) DO NOTHING`,
			textHash, Float32ToBytes(emb))
		return err
	})
}

// GetEmbedCache retrieves a cached embedding by text hash.
func (s *Store) GetEmbedCache(textHash string) ([]float32, error) {
	var blob []byte
	err := s.Read(func(db *sql.DB) error {
		return db.QueryRow(`SELECT embedding FROM embed_cache WHERE text_hash=?`, textHash).Scan(&blob)
	})
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return BytesToFloat32(blob), nil
}

// EmbeddingCount returns how many nodes have embeddings.
func (s *Store) EmbeddingCount() (int, error) {
	var count int
	err := s.Read(func(db *sql.DB) error {
		return db.QueryRow("SELECT COUNT(*) FROM node_embeddings").Scan(&count)
	})
	return count, err
}

func norm(v []float32) float64 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	return math.Sqrt(sum)
}

func cosineSim(a []float32, aNorm float64, b []float32) float64 {
	if len(a) != len(b) || aNorm == 0 {
		return 0
	}
	var dot float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
	}
	bNorm := norm(b)
	if bNorm == 0 {
		return 0
	}
	return dot / (aNorm * bNorm)
}
