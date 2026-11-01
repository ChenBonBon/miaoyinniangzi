package storage

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
)

func downloadKey(id string) (string, error) {
	if len(id) != 32 {
		return "", errors.New("invalid download ID")
	}
	if _, err := hex.DecodeString(id); err != nil {
		return "", errors.New("invalid download ID")
	}
	return "download:" + id, nil
}
func (s *Store) SaveDownload(ctx context.Context, id string, data json.RawMessage) error {
	key, err := downloadKey(id)
	if err != nil || len(data) > 64*1024 || !json.Valid(data) {
		return errors.New("invalid download record")
	}
	return s.save(ctx, key, data)
}
func (s *Store) SaveDownloads(ctx context.Context, records map[string]json.RawMessage) error {
	for id, data := range records {
		if _, err := downloadKey(id); err != nil || len(data) > 64*1024 || !json.Valid(data) {
			return errors.New("invalid download record")
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statement, err := tx.PrepareContext(ctx, `INSERT INTO state(key,payload) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET payload=excluded.payload`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for id, data := range records {
		key, _ := downloadKey(id)
		if _, err = statement.ExecContext(ctx, key, string(data)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
func (s *Store) DownloadRecords(ctx context.Context) ([]json.RawMessage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM state WHERE key LIKE 'download:%' ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []json.RawMessage{}
	for rows.Next() {
		if len(result) >= 10000 {
			return nil, errors.New("download record limit exceeded")
		}
		var data string
		if err = rows.Scan(&data); err != nil {
			return nil, err
		}
		if len(data) > 64*1024 || !json.Valid([]byte(data)) {
			return nil, errors.New("invalid download record")
		}
		result = append(result, json.RawMessage(data))
	}
	return result, rows.Err()
}
func (s *Store) RemoveDownload(ctx context.Context, id string) error {
	key, err := downloadKey(id)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM state WHERE key=?`, key)
	return err
}
