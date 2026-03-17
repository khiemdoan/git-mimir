package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// ClusterRow represents a cluster from the store.
type ClusterRow struct {
	ID            string
	Label         string
	CohesionScore float64
	Members       []string
}

// ProcessRow represents a process from the store.
type ProcessRow struct {
	ID          string
	Name        string
	EntryUID    string
	ProcessType string
	Steps       []ProcessStep
}

// ProcessStep is one step within a process trace.
type ProcessStep struct {
	Index     int
	SymbolUID string
	Depth     int
}

// UpsertCluster inserts or updates a cluster.
func (s *Store) UpsertCluster(id, label string, cohesion float64) error {
	return s.Write(func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO clusters(id, label, cohesion_score) VALUES(?,?,?)
			ON CONFLICT(id) DO UPDATE SET label=excluded.label, cohesion_score=excluded.cohesion_score`,
			id, label, cohesion)
		return err
	})
}

// UpsertClusterMembers sets the cluster membership for a set of node UIDs.
func (s *Store) UpsertClusterMembers(clusterID string, memberUIDs []string) error {
	if len(memberUIDs) == 0 {
		return nil
	}
	return s.Write(func(tx *sql.Tx) error {
		stmt, err := tx.Prepare(`
			INSERT INTO cluster_members(cluster_id, symbol_uid) VALUES(?,?)
			ON CONFLICT DO NOTHING`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, uid := range memberUIDs {
			if _, err := stmt.Exec(clusterID, uid); err != nil {
				return err
			}
		}
		return nil
	})
}

// SetClusterForNodes updates the cluster_id column on nodes.
func (s *Store) SetClusterForNodes(memberUIDs []string, clusterID string) error {
	if len(memberUIDs) == 0 {
		return nil
	}
	return s.Write(func(tx *sql.Tx) error {
		placeholders := strings.Repeat("?,", len(memberUIDs))
		placeholders = placeholders[:len(placeholders)-1]
		args := make([]any, len(memberUIDs)+1)
		args[0] = clusterID
		for i, uid := range memberUIDs {
			args[i+1] = uid
		}
		_, err := tx.Exec(fmt.Sprintf(
			`UPDATE nodes SET cluster_id=? WHERE uid IN (%s)`, placeholders), args...)
		return err
	})
}

// AllClusters returns all clusters with their member UIDs.
func (s *Store) AllClusters() ([]ClusterRow, error) {
	var clusters []ClusterRow
	err := s.Read(func(db *sql.DB) error {
		rows, err := db.Query(`SELECT id, COALESCE(label,''), COALESCE(cohesion_score,0) FROM clusters`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c ClusterRow
			if err := rows.Scan(&c.ID, &c.Label, &c.CohesionScore); err != nil {
				return err
			}
			clusters = append(clusters, c)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	// Load members
	for i, c := range clusters {
		var members []string
		err := s.Read(func(db *sql.DB) error {
			rows, err := db.Query(`SELECT symbol_uid FROM cluster_members WHERE cluster_id=?`, c.ID)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var uid string
				if err := rows.Scan(&uid); err != nil {
					return err
				}
				members = append(members, uid)
			}
			return rows.Err()
		})
		if err != nil {
			return nil, err
		}
		clusters[i].Members = members
	}
	return clusters, nil
}

// UpsertProcess inserts or updates a process.
func (s *Store) UpsertProcess(id, name, entryUID, processType string) error {
	return s.Write(func(tx *sql.Tx) error {
		_, err := tx.Exec(`
			INSERT INTO processes(id, name, entry_uid, process_type) VALUES(?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET name=excluded.name, entry_uid=excluded.entry_uid, process_type=excluded.process_type`,
			id, name, entryUID, processType)
		return err
	})
}

// UpsertProcessSteps inserts steps for a process (replaces existing).
func (s *Store) UpsertProcessSteps(processID string, steps []ProcessStep) error {
	return s.Write(func(tx *sql.Tx) error {
		if _, err := tx.Exec(`DELETE FROM process_steps WHERE process_id=?`, processID); err != nil {
			return err
		}
		stmt, err := tx.Prepare(`INSERT INTO process_steps(process_id, step_index, symbol_uid, depth) VALUES(?,?,?,?)`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, step := range steps {
			if _, err := stmt.Exec(processID, step.Index, step.SymbolUID, step.Depth); err != nil {
				return err
			}
		}
		return nil
	})
}

// AllProcesses returns all processes with their steps.
func (s *Store) AllProcesses() ([]ProcessRow, error) {
	var processes []ProcessRow
	err := s.Read(func(db *sql.DB) error {
		rows, err := db.Query(`SELECT id, COALESCE(name,''), COALESCE(entry_uid,''), COALESCE(process_type,'') FROM processes`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p ProcessRow
			if err := rows.Scan(&p.ID, &p.Name, &p.EntryUID, &p.ProcessType); err != nil {
				return err
			}
			processes = append(processes, p)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}

	for i, p := range processes {
		var steps []ProcessStep
		err := s.Read(func(db *sql.DB) error {
			rows, err := db.Query(
				`SELECT step_index, symbol_uid, depth FROM process_steps WHERE process_id=? ORDER BY step_index`, p.ID)
			if err != nil {
				return err
			}
			defer rows.Close()
			for rows.Next() {
				var step ProcessStep
				if err := rows.Scan(&step.Index, &step.SymbolUID, &step.Depth); err != nil {
					return err
				}
				steps = append(steps, step)
			}
			return rows.Err()
		})
		if err != nil {
			return nil, err
		}
		processes[i].Steps = steps
	}
	return processes, nil
}

// DeleteAllClusters removes all cluster data (used before re-clustering).
func (s *Store) DeleteAllClusters() error {
	return s.Write(func(tx *sql.Tx) error {
		tx.Exec(`DELETE FROM cluster_members`)
		tx.Exec(`DELETE FROM clusters`)
		_, err := tx.Exec(`UPDATE nodes SET cluster_id=NULL`)
		return err
	})
}
