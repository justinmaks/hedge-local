package store

import "fmt"

func (s *Store) ProjectUpsert(path, name string) (int64, error) {
	_, err := s.db.Exec(
		`INSERT INTO projects (path, name) VALUES (?, ?)
		 ON CONFLICT(path) DO UPDATE SET name = excluded.name`,
		path, name,
	)
	if err != nil {
		return 0, fmt.Errorf("upsert project: %w", err)
	}
	var id int64
	err = s.db.QueryRow(`SELECT id FROM projects WHERE path = ?`, path).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("get project id: %w", err)
	}
	return id, nil
}
