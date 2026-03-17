package store

import (
	"database/sql"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"
)

// Store manages the SQLite database with a single write connection
// (mutex-serialized) and a concurrent read pool.
type Store struct {
	write *sql.DB
	read  *sql.DB
	mu    sync.Mutex
}

// OpenStore opens (or creates) the Mimir SQLite database at path.
// Use ":memory:" for tests.
func OpenStore(path string) (*Store, error) {
	dsn := path
	if path != ":memory:" {
		dsn = path + "?_journal=WAL&_synchronous=NORMAL&_cache_size=-64000&_foreign_keys=ON&_temp_store=MEMORY"
	} else {
		dsn = "file::memory:?cache=shared&_foreign_keys=ON"
	}

	w, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open write db: %w", err)
	}
	w.SetMaxOpenConns(1)
	w.SetMaxIdleConns(1)

	// Read pool uses the same file (WAL allows concurrent reads)
	rdDSN := dsn
	if path != ":memory:" {
		rdDSN = path + "?_journal=WAL&_synchronous=NORMAL&_cache_size=-64000&_foreign_keys=ON"
	}
	r, err := sql.Open("sqlite", rdDSN)
	if err != nil {
		w.Close()
		return nil, fmt.Errorf("open read db: %w", err)
	}
	r.SetMaxOpenConns(8)
	r.SetMaxIdleConns(8)

	s := &Store{write: w, read: r}
	if err := s.migrate(); err != nil {
		s.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close closes both database connections.
func (s *Store) Close() {
	if s.write != nil {
		s.write.Close()
	}
	if s.read != nil {
		s.read.Close()
	}
}

// Write executes fn inside a mutex-protected write transaction.
func (s *Store) Write(fn func(*sql.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.write.Begin()
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Read executes fn against the concurrent read pool.
func (s *Store) Read(fn func(*sql.DB) error) error {
	return fn(s.read)
}

// SetMeta stores a key-value pair in the index_meta table.
func (s *Store) SetMeta(key, value string) error {
	return s.Write(func(tx *sql.Tx) error {
		_, err := tx.Exec(`INSERT INTO index_meta(key,value) VALUES(?,?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
		return err
	})
}

// ReadRaw executes a raw SQL query on the read pool and calls fn with the results.
func (s *Store) ReadRaw(query string, fn func(columns []string, rows [][]interface{})) error {
	return s.Read(func(db *sql.DB) error {
		rows, err := db.Query(query)
		if err != nil {
			return err
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			return err
		}

		var allRows [][]interface{}
		for rows.Next() {
			vals := make([]interface{}, len(cols))
			ptrs := make([]interface{}, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				return err
			}
			row := make([]interface{}, len(cols))
			copy(row, vals)
			allRows = append(allRows, row)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		fn(cols, allRows)
		return nil
	})
}

// GetMeta retrieves a value from index_meta. Returns ("", nil) if not found.
func (s *Store) GetMeta(key string) (string, error) {
	var value string
	err := s.Read(func(db *sql.DB) error {
		row := db.QueryRow(`SELECT value FROM index_meta WHERE key=?`, key)
		return row.Scan(&value)
	})
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}
