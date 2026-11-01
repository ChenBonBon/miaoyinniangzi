package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

const SearchHistoryLimit = 20

func normalizeHistoryQuery(query string) (string, error) {
	if len(query) > 400 || !utf8.ValidString(query) {
		return "", errors.New("invalid search history query")
	}
	query = strings.TrimSpace(query)
	if query == "" || utf8.RuneCountInString(query) > 100 {
		return "", errors.New("invalid search history query")
	}
	for _, r := range query {
		if unicode.IsControl(r) {
			return "", errors.New("invalid search history query")
		}
	}
	return query, nil
}

type historyReader interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func readHistory(ctx context.Context, reader historyReader) ([]string, error) {
	rows, err := reader.QueryContext(ctx, `SELECT query FROM search_history ORDER BY sequence DESC LIMIT ?`, SearchHistoryLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []string{}
	for rows.Next() {
		var query string
		if err := rows.Scan(&query); err != nil {
			return nil, err
		}
		result = append(result, query)
	}
	return result, rows.Err()
}
func (s *Store) SearchHistory(ctx context.Context) ([]string, error) { return readHistory(ctx, s.db) }

// Each edit returns a snapshot from the same transaction; records are unique,
// newest first and capped. SQL never interpolates the search text.
func (s *Store) RecordSearchHistory(ctx context.Context, query string) ([]string, error) {
	query, err := normalizeHistoryQuery(query)
	if err != nil {
		return nil, err
	}
	return s.editHistory(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO search_history(query,sequence) VALUES(?,(SELECT COALESCE(MAX(sequence),0)+1 FROM search_history)) ON CONFLICT(query) DO UPDATE SET sequence=excluded.sequence`, query); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM search_history WHERE query NOT IN (SELECT query FROM search_history ORDER BY sequence DESC LIMIT ?)`, SearchHistoryLimit)
		return err
	})
}
func (s *Store) RemoveSearchHistory(ctx context.Context, query string) ([]string, error) {
	query, err := normalizeHistoryQuery(query)
	if err != nil {
		return nil, err
	}
	return s.editHistory(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM search_history WHERE query=?`, query)
		return err
	})
}
func (s *Store) ClearSearchHistory(ctx context.Context) ([]string, error) {
	return s.editHistory(ctx, func(tx *sql.Tx) error { _, err := tx.ExecContext(ctx, `DELETE FROM search_history`); return err })
}
func (s *Store) editHistory(ctx context.Context, edit func(*sql.Tx) error) ([]string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := edit(tx); err != nil {
		return nil, err
	}
	result, err := readHistory(ctx, tx)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}
