// Package storage owns local application data. It never executes source scripts.
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

const schemaVersion = 4

// Store uses a single SQLite connection so transactions and connection pragmas
// have predictable behavior. Callers must close it after stopping producers.
type Store struct {
	db       *sql.DB
	defaults Settings
}

type Settings struct {
	DownloadDirectory string  `json:"downloadDirectory"`
	DownloadQuality   string  `json:"downloadQuality"`
	PlaybackQuality   string  `json:"playbackQuality"`
	Volume            float64 `json:"volume"`
}

type Artist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Song struct {
	Artists  []Artist `json:"artists,omitempty"`
	AlbumMid string   `json:"albumMid,omitempty"`
	SongID   string   `json:"songId,omitempty"`
	MediaMid string   `json:"mediaMid,omitempty"`
	Duration float64  `json:"duration,omitempty"`
	ID       string   `json:"id"`
	Source   string   `json:"source"`
	Title    string   `json:"title"`
	Artist   string   `json:"artist"`
	Album    string   `json:"album"`
	CoverURL string   `json:"coverUrl"`
}

type Playback struct {
	Queue []Song `json:"queue"`
	// Index is -1 when no song is selected. Repeated songs in a queue are allowed.
	Index      int     `json:"index"`
	Position   float64 `json:"position"`
	Mode       string  `json:"mode"`
	QueuedNext *int    `json:"queuedNext,omitempty"`
}

// Open creates a private directory and database. downloadDirectory is supplied
// separately to make defaults testable without consulting the real user's home.
func Open(directory, downloadDirectory string) (*Store, error) {
	if !filepath.IsAbs(directory) || !filepath.IsAbs(downloadDirectory) {
		return nil, errors.New("storage paths must be absolute")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(directory, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(directory, "miaoyin.sqlite3")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0600); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, defaults: Settings{downloadDirectory, "highest", "highest", 1}}
	if err := s.initialize(); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize local database: %w", err)
	}
	return s, nil
}

func (s *Store) initialize() error {
	ctx := context.Background()
	for _, pragma := range []string{"PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON", "PRAGMA secure_delete=ON"} {
		if _, err := s.db.ExecContext(ctx, pragma); err != nil {
			return err
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > schemaVersion {
		return fmt.Errorf("database version %d is newer than supported version %d", version, schemaVersion)
	}
	if version == 0 {
		for _, statement := range []string{
			`CREATE TABLE state (key TEXT PRIMARY KEY, payload TEXT NOT NULL)`,
			`CREATE TABLE favorites (song_id TEXT PRIMARY KEY, payload TEXT NOT NULL, sequence INTEGER NOT NULL UNIQUE)`,
			`PRAGMA user_version=1`,
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
	}
	if version < 2 {
		if _, err := tx.ExecContext(ctx, `CREATE TABLE sources (
            id TEXT PRIMARY KEY,
            name TEXT NOT NULL, author TEXT NOT NULL, version TEXT NOT NULL,
            created_at TEXT NOT NULL,
            digest TEXT NOT NULL UNIQUE,
            ciphertext BLOB NOT NULL
        )`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `PRAGMA user_version=2`); err != nil {
			return err
		}
	}
	if version < 3 {
		for _, statement := range []string{
			`ALTER TABLE sources ADD COLUMN check_status TEXT NOT NULL DEFAULT 'unchecked'`,
			`ALTER TABLE sources ADD COLUMN check_error TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE sources ADD COLUMN check_engine TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE sources ADD COLUMN qualities TEXT NOT NULL DEFAULT '[]'`,
			`ALTER TABLE sources ADD COLUMN active INTEGER NOT NULL DEFAULT 0 CHECK(active IN (0,1))`,
			`CREATE UNIQUE INDEX sources_one_active ON sources(active) WHERE active=1`,
			`PRAGMA user_version=3`,
		} {
			if _, err := tx.ExecContext(ctx, statement); err != nil {
				return err
			}
		}
	}
	if version < 4 {
		if _, err := tx.ExecContext(ctx, `CREATE TABLE search_history (query TEXT PRIMARY KEY NOT NULL, sequence INTEGER NOT NULL UNIQUE)`); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `PRAGMA user_version=4`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Close() error { return s.db.Close() }
func (s *Store) read(ctx context.Context, key string, target any) error {
	var data string
	err := s.db.QueryRowContext(ctx, `SELECT payload FROM state WHERE key=?`, key).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(data), target); err != nil {
		return fmt.Errorf("decode %s: %w", key, err)
	}
	return nil
}
func (s *Store) save(ctx context.Context, key string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO state(key,payload) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET payload=excluded.payload`, key, string(data))
	return err
}
func validQuality(q string) bool { return q == "highest" || q == "flac" || q == "320k" || q == "128k" }
func validateSettings(v Settings) error {
	if !filepath.IsAbs(v.DownloadDirectory) {
		return errors.New("download directory must be absolute")
	}
	if !validQuality(v.DownloadQuality) || !validQuality(v.PlaybackQuality) {
		return errors.New("unsupported quality")
	}
	if math.IsNaN(v.Volume) || math.IsInf(v.Volume, 0) || v.Volume < 0 || v.Volume > 1 {
		return errors.New("volume must be between 0 and 1")
	}
	return nil
}
func (s *Store) Settings(ctx context.Context) (Settings, error) {
	v := s.defaults
	if err := s.read(ctx, "settings", &v); err != nil {
		return v, err
	}
	return v, validateSettings(v)
}
func (s *Store) SaveSettings(ctx context.Context, v Settings) error {
	if err := validateSettings(v); err != nil {
		return err
	}
	return s.save(ctx, "settings", v)
}
func validateSong(song Song) error {
	if len(song.Artists) > 32 || len(song.AlbumMid) > 64 {
		return errors.New("invalid song credits")
	}
	for _, artist := range song.Artists {
		if len(artist.ID) > 64 || len([]rune(artist.Name)) > 256 {
			return errors.New("invalid song credits")
		}
	}
	if math.IsNaN(song.Duration) || math.IsInf(song.Duration, 0) || song.Duration < 0 {
		return errors.New("invalid song duration")
	}
	if song.Source != "tx" || song.ID == "" || song.Title == "" {
		return errors.New("song requires QQ music source, ID and title")
	}
	return nil
}
func validatePlayback(v Playback) error {
	if len(v.Queue) > 10000 {
		return errors.New("queue exceeds 10000 songs")
	}
	for _, song := range v.Queue {
		if err := validateSong(song); err != nil {
			return err
		}
	}
	if v.Index < -1 || v.Index >= len(v.Queue) {
		return errors.New("invalid queue index")
	}
	if math.IsNaN(v.Position) || math.IsInf(v.Position, 0) || v.Position < 0 {
		return errors.New("invalid playback position")
	}
	if v.Index == -1 && v.Position != 0 {
		return errors.New("position requires a selected song")
	}
	if v.QueuedNext != nil && (v.Index < 0 || *v.QueuedNext < 0 || *v.QueuedNext >= len(v.Queue) || *v.QueuedNext == v.Index) {
		return errors.New("invalid queued next occurrence")
	}
	if v.Mode != "sequential" && v.Mode != "shuffle" && v.Mode != "repeat-one" {
		return errors.New("invalid playback mode")
	}
	return nil
}
func (s *Store) Playback(ctx context.Context) (Playback, error) {
	v := Playback{Queue: []Song{}, Index: -1, Mode: "sequential"}
	if err := s.read(ctx, "playback", &v); err != nil {
		return v, err
	}
	return v, validatePlayback(v)
}
func (s *Store) SavePlayback(ctx context.Context, v Playback) error {
	if err := validatePlayback(v); err != nil {
		return err
	}
	if v.Queue == nil {
		v.Queue = []Song{}
	}
	return s.save(ctx, "playback", v)
}

// AddFavorite is idempotent: adding an existing song does not change its order.
func (s *Store) AddFavorite(ctx context.Context, song Song) error {
	if err := validateSong(song); err != nil {
		return err
	}
	data, err := json.Marshal(song)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO favorites(song_id,payload,sequence) SELECT ?,?,COALESCE(MAX(sequence),0)+1 FROM favorites WHERE true ON CONFLICT(song_id) DO NOTHING`, song.ID, string(data))
	return err
}
func (s *Store) RemoveFavorite(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM favorites WHERE song_id=?`, id)
	return err
}
func (s *Store) Favorites(ctx context.Context) ([]Song, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT payload FROM favorites ORDER BY sequence DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Song{}
	for rows.Next() {
		var data string
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var song Song
		if err := json.Unmarshal([]byte(data), &song); err != nil {
			return nil, err
		}
		if err := validateSong(song); err != nil {
			return nil, err
		}
		result = append(result, song)
	}
	return result, rows.Err()
}
