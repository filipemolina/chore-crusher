package store

import (
	"fmt"
	"strings"
)

// ResolveID expands an unambiguous prefix of a list or task id into the full
// id, for callers that accept an id from a human or an agent who copied a
// short prefix (docs/DESIGN.md §9). A prefix matching zero rows is a
// not-found error; one matching more than one row is an ambiguous error — a
// caller must never resolve that by guessing.
//
// table is one of a small fixed set ("list", "task"); anything else is
// rejected before it can be interpolated into SQL.
func (s *Store) ResolveID(table, prefix string) (string, error) {
	var tableName string
	switch table {
	case "list":
		tableName = `"list"`
	case "task":
		tableName = `"task"`
	default:
		return "", fmt.Errorf("resolve id: unknown table %q", table)
	}
	if prefix == "" {
		return "", fmt.Errorf("resolve id: empty prefix")
	}

	rows, err := s.db.Query(`SELECT id FROM `+tableName+` WHERE id LIKE (? || '%')`, prefix)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var matches []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		matches = append(matches, id)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("%s not found: %s", table, prefix)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous %s prefix %q (matches %d: %s)",
			table, prefix, len(matches), strings.Join(matches, ", "))
	}
}
