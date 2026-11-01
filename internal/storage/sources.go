package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// SourceInfo is the only source data exposed to the UI. No path, hash, key or
// script contents are included. Active denotes a locally verified selection,
// not proof that the remote music service is currently available.
type SourceInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Author      string   `json:"author"`
	Version     string   `json:"version"`
	CreatedAt   string   `json:"createdAt"`
	CheckStatus string   `json:"checkStatus"`
	CheckError  string   `json:"checkError"`
	Qualities   []string `json:"qualities"`
	Active      bool     `json:"active"`
}

const sourceColumns = `id,name,author,version,created_at,check_status,check_error,qualities,active`

type rowScanner interface{ Scan(...any) error }

func scanSource(row rowScanner) (SourceInfo, error) {
	var v SourceInfo
	var qualities string
	if err := row.Scan(&v.ID, &v.Name, &v.Author, &v.Version, &v.CreatedAt, &v.CheckStatus, &v.CheckError, &qualities, &v.Active); err != nil {
		return v, err
	}
	if err := json.Unmarshal([]byte(qualities), &v.Qualities); err != nil {
		return v, err
	}
	if v.Qualities == nil {
		v.Qualities = []string{}
	}
	return v, nil
}
func (s *Store) Sources(ctx context.Context) ([]SourceInfo, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+sourceColumns+` FROM sources ORDER BY created_at DESC,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SourceInfo{}
	for rows.Next() {
		v, err := scanSource(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
func (s *Store) SourceByDigest(ctx context.Context, digest string) (SourceInfo, bool, error) {
	v, err := scanSource(s.db.QueryRowContext(ctx, `SELECT `+sourceColumns+` FROM sources WHERE digest=?`, digest))
	if errors.Is(err, sql.ErrNoRows) {
		return v, false, nil
	}
	return v, err == nil, err
}
func (s *Store) Source(ctx context.Context, id string) (SourceInfo, error) {
	return scanSource(s.db.QueryRowContext(ctx, `SELECT `+sourceColumns+` FROM sources WHERE id=?`, id))
}

// InsertSource commits metadata and encrypted content atomically. A concurrent
// duplicate import returns the existing record without replacing its content.
func (s *Store) InsertSource(ctx context.Context, v SourceInfo, digest string, ciphertext []byte) (SourceInfo, bool, error) {
	if len(ciphertext) < 28 || v.ID == "" || v.Name == "" || len(digest) != 64 {
		return SourceInfo{}, false, errors.New("invalid encrypted source")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SourceInfo{}, false, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO sources(id,name,author,version,created_at,digest,ciphertext) VALUES(?,?,?,?,?,?,?) ON CONFLICT(digest) DO NOTHING`, v.ID, v.Name, v.Author, v.Version, v.CreatedAt, digest, ciphertext); err != nil {
		return SourceInfo{}, false, err
	}
	result, err := scanSource(tx.QueryRowContext(ctx, `SELECT `+sourceColumns+` FROM sources WHERE digest=?`, digest))
	if err != nil {
		return SourceInfo{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return SourceInfo{}, false, err
	}
	return result, result.ID != v.ID, nil
}
func (s *Store) SourceCiphertext(ctx context.Context, id string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRowContext(ctx, `SELECT ciphertext FROM sources WHERE id=?`, id).Scan(&data)
	return data, err
}
func (s *Store) DeleteSource(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM sources WHERE id=?`, id)
	if err != nil {
		return err
	}
	return requireSource(result)
}
func requireSource(result sql.Result) error {
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("source not found or not compatible")
	}
	return nil
}

func (s *Store) RecordSourceCheck(ctx context.Context, id, status, message, engine string, qualities []string) error {
	if status != "ready" && status != "failed" {
		return errors.New("invalid check status")
	}
	if qualities == nil {
		qualities = []string{}
	}
	data, err := json.Marshal(qualities)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE sources SET check_status=?,check_error=?,check_engine=?,qualities=?,active=CASE WHEN ?='ready' THEN active ELSE 0 END WHERE id=?`, status, message, engine, string(data), status, id)
	if err != nil {
		return err
	}
	return requireSource(result)
}

// A unique partial index plus a transaction enforces one active source even
// across independent app connections. Failure leaves the old selection intact.
func (s *Store) ActivateSource(ctx context.Context, id, engine string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE sources SET active=0 WHERE active=1`); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE sources SET active=1 WHERE id=? AND check_status='ready' AND check_engine=?`, id, engine)
	if err != nil {
		return err
	}
	if err := requireSource(result); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) DisableSources(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sources SET active=0 WHERE active=1`)
	return err
}

// Cached checks must be repeated when the supported runtime changes.
func (s *Store) InvalidateSourceChecks(ctx context.Context, engine string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sources SET active=0,check_status='unchecked',check_error='',qualities='[]',check_engine='' WHERE check_engine<>?`, engine)
	return err
}
