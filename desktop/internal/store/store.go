// Package store opens the SQLite database and manages its schema.
package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type Store struct {
	db   *sql.DB
	Path string
}

var schemaStatements = []string{
	`CREATE TABLE IF NOT EXISTS questions (
		id VARCHAR(100) NOT NULL,
		owner_id VARCHAR(32) NOT NULL,
		ord INTEGER NOT NULL,
		trashed BOOLEAN NOT NULL,
		subject TEXT NOT NULL,
		title TEXT NOT NULL,
		answer TEXT NOT NULL,
		reason TEXT NOT NULL,
		tag TEXT NOT NULL,
		cause TEXT NOT NULL,
		difficulty TEXT NOT NULL,
		image TEXT NOT NULL,
		favorite BOOLEAN NOT NULL,
		stage INTEGER NOT NULL,
		due VARCHAR(10) NOT NULL,
		created VARCHAR(10) NOT NULL,
		demo BOOLEAN NOT NULL,
		PRIMARY KEY (id)
	)`,
	`CREATE INDEX IF NOT EXISTS ix_questions_owner_id ON questions (owner_id)`,
	`CREATE INDEX IF NOT EXISTS ix_questions_trashed ON questions (trashed)`,
	`CREATE TABLE IF NOT EXISTS logs (
		id VARCHAR(100) NOT NULL,
		owner_id VARCHAR(32) NOT NULL,
		ord INTEGER NOT NULL,
		date VARCHAR(10) NOT NULL,
		at VARCHAR(40) NOT NULL,
		type VARCHAR(20) NOT NULL,
		label TEXT NOT NULL,
		points FLOAT NOT NULL,
		qid VARCHAR(100),
		grade VARCHAR(20),
		minutes FLOAT,
		PRIMARY KEY (id)
	)`,
	`CREATE INDEX IF NOT EXISTS ix_logs_owner_id ON logs (owner_id)`,
	`CREATE INDEX IF NOT EXISTS ix_logs_date ON logs (date)`,
	`CREATE TABLE IF NOT EXISTS rewards (
		id VARCHAR(100) NOT NULL,
		owner_id VARCHAR(32) NOT NULL,
		ord INTEGER NOT NULL,
		name TEXT NOT NULL,
		cost INTEGER NOT NULL,
		PRIMARY KEY (id)
	)`,
	`CREATE INDEX IF NOT EXISTS ix_rewards_owner_id ON rewards (owner_id)`,
	`CREATE TABLE IF NOT EXISTS redemptions (
		id VARCHAR(100) NOT NULL,
		owner_id VARCHAR(32) NOT NULL,
		ord INTEGER NOT NULL,
		name TEXT NOT NULL,
		cost INTEGER NOT NULL,
		date VARCHAR(10) NOT NULL,
		PRIMARY KEY (id)
	)`,
	`CREATE INDEX IF NOT EXISTS ix_redemptions_owner_id ON redemptions (owner_id)`,
	`CREATE TABLE IF NOT EXISTS prefs (
		owner_id VARCHAR(32) NOT NULL,
		name TEXT NOT NULL,
		goal INTEGER NOT NULL,
		minutes INTEGER NOT NULL,
		theme VARCHAR(20) NOT NULL,
		PRIMARY KEY (owner_id)
	)`,
	`CREATE TABLE IF NOT EXISTS timers (
		owner_id VARCHAR(32) NOT NULL,
		running BOOLEAN NOT NULL,
		remaining FLOAT NOT NULL,
		duration FLOAT NOT NULL,
		"end" FLOAT,
		PRIMARY KEY (owner_id)
	)`,
	`CREATE TABLE IF NOT EXISTS ai_config (
		owner_id VARCHAR(32) NOT NULL,
		provider VARCHAR(20) NOT NULL,
		base TEXT NOT NULL,
		vision VARCHAR(120) NOT NULL,
		text VARCHAR(120) NOT NULL,
		deepseek_base TEXT NOT NULL,
		deepseek_vision VARCHAR(120) NOT NULL,
		deepseek_text VARCHAR(120) NOT NULL,
		PRIMARY KEY (owner_id)
	)`,
	`CREATE TABLE IF NOT EXISTS ai_keys (
		owner_id VARCHAR(32) NOT NULL,
		provider VARCHAR(20) NOT NULL,
		ciphertext TEXT NOT NULL,
		updated VARCHAR(20) NOT NULL,
		PRIMARY KEY (owner_id, provider)
	)`,
	`CREATE TABLE IF NOT EXISTS ai_sets (
		id VARCHAR(100) NOT NULL,
		owner_id VARCHAR(32) NOT NULL,
		ord INTEGER NOT NULL,
		source_id VARCHAR(100) NOT NULL,
		source_title TEXT NOT NULL,
		subject TEXT NOT NULL,
		created VARCHAR(10) NOT NULL,
		ai_reviewed BOOLEAN NOT NULL,
		PRIMARY KEY (id)
	)`,
	`CREATE INDEX IF NOT EXISTS ix_ai_sets_owner_id ON ai_sets (owner_id)`,
	`CREATE TABLE IF NOT EXISTS ai_set_questions (
		id VARCHAR(100) NOT NULL,
		set_id VARCHAR(100) NOT NULL,
		ord INTEGER NOT NULL,
		title TEXT NOT NULL,
		answer TEXT NOT NULL,
		explanation TEXT NOT NULL,
		knowledge TEXT NOT NULL,
		note TEXT NOT NULL,
		revealed BOOLEAN NOT NULL,
		grade VARCHAR(20) NOT NULL,
		PRIMARY KEY (id),
		FOREIGN KEY(set_id) REFERENCES ai_sets (id) ON DELETE CASCADE
	)`,
	`CREATE INDEX IF NOT EXISTS ix_ai_set_questions_set_id ON ai_set_questions (set_id)`,
}

// Open creates the database and its schema when needed.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	dsn, err := dataSourceName(path)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	for _, pragma := range []string{
		"PRAGMA foreign_keys=ON",
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, err
		}
	}
	for _, statement := range schemaStatements {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			return nil, fmt.Errorf("create schema: %w", err)
		}
	}
	return &Store{db: db, Path: path}, nil
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) Close() error {
	_, _ = s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	return s.db.Close()
}

// WithTx runs fn inside one transaction; any error rolls everything back.
func (s *Store) WithTx(fn func(*sql.Tx) error) (err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = tx.Rollback()
			panic(recovered)
		}
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Exists() bool {
	info, err := os.Stat(s.Path)
	return err == nil && !info.IsDir()
}

func dataSourceName(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	slash := "/" + strings.TrimPrefix(filepath.ToSlash(absolute), "/")
	uri := url.URL{Scheme: "file", Path: slash}
	query := url.Values{}
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "journal_mode(WAL)")
	query.Add("_pragma", "busy_timeout(5000)")
	uri.RawQuery = query.Encode()
	return uri.String(), nil
}
