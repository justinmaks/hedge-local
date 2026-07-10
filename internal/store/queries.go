package store

import (
	"database/sql"
	"fmt"
	"strings"
)

func (s *Store) QueryRaw(sqlText string) (cols []string, rows [][]string, err error) {
	trimmed := strings.TrimSpace(strings.ToUpper(sqlText))
	if !strings.HasPrefix(trimmed, "SELECT") && !strings.HasPrefix(trimmed, "WITH") {
		return nil, nil, fmt.Errorf("only SELECT or WITH queries are allowed")
	}

	rows2, err := s.db.Query(sqlText)
	if err != nil {
		return nil, nil, err
	}
	defer rows2.Close()

	cols, err = rows2.Columns()
	if err != nil {
		return nil, nil, err
	}

	for rows2.Next() {
		raw := make([]any, len(cols))
		for i := range raw {
			raw[i] = &sql.NullString{}
		}
		if err := rows2.Scan(raw...); err != nil {
			return nil, nil, err
		}
		row := make([]string, len(cols))
		for i, v := range raw {
			ns := v.(*sql.NullString)
			if ns.Valid {
				row[i] = ns.String
			} else {
				row[i] = "NULL"
			}
		}
		rows = append(rows, row)
	}
	return cols, rows, nil
}
