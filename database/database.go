// Package database owns the SQLite connection and all SQL for the washes table.
package database

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"go-carwash-api/models"

	_ "modernc.org/sqlite" // pure-Go SQLite driver, no cgo needed
)

//go:embed schema.sql
var schema string

// Store wraps a SQLite database and exposes typed queries for washes.
type Store struct {
	db *sql.DB
}

// Open connects to the SQLite file at path (created if missing) and applies the schema.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite handles one writer at a time; a single connection keeps things simple and safe.
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`PRAGMA journal_mode = WAL; PRAGMA foreign_keys = ON;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("set pragmas: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &Store{db: db}, nil
}

// Close releases the underlying connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Create inserts a new wash in the queued state and returns it with its ID and timestamp.
func (s *Store) Create(ctx context.Context, registration, washType string) (models.Wash, error) {
	now := time.Now().UTC().Truncate(time.Second)
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO washes (registration_number, wash_type, status, created_at)
		 VALUES (?, ?, ?, ?)`,
		registration, washType, models.StatusQueued, now.Format(time.RFC3339),
	)
	if err != nil {
		return models.Wash{}, fmt.Errorf("insert wash: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return models.Wash{}, fmt.Errorf("last insert id: %w", err)
	}
	return models.Wash{
		ID:                 id,
		RegistrationNumber: registration,
		WashType:           washType,
		Status:             models.StatusQueued,
		CreatedAt:          now,
	}, nil
}

// List returns all washes, optionally filtered by status, newest first.
func (s *Store) List(ctx context.Context, status string) ([]models.Wash, error) {
	query := `SELECT id, registration_number, wash_type, status, created_at FROM washes`
	var args []any
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at DESC, id DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list washes: %w", err)
	}
	defer rows.Close()

	washes := []models.Wash{} // non-nil so an empty list serialises as [] not null
	for rows.Next() {
		w, err := scanWash(rows)
		if err != nil {
			return nil, err
		}
		washes = append(washes, w)
	}
	return washes, rows.Err()
}

// Get returns a single wash by ID, or models.ErrNotFound.
func (s *Store) Get(ctx context.Context, id int64) (models.Wash, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, registration_number, wash_type, status, created_at FROM washes WHERE id = ?`, id)
	w, err := scanWash(row)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Wash{}, models.ErrNotFound
	}
	return w, err
}

// UpdateStatus changes the status of a wash and returns the updated row.
// The caller is responsible for validating the transition.
func (s *Store) UpdateStatus(ctx context.Context, id int64, status string) (models.Wash, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE washes SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return models.Wash{}, fmt.Errorf("update status: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.Wash{}, models.ErrNotFound
	}
	return s.Get(ctx, id)
}

// Delete removes a wash by ID, or returns models.ErrNotFound.
func (s *Store) Delete(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM washes WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete wash: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return models.ErrNotFound
	}
	return nil
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanWash(sc scanner) (models.Wash, error) {
	var w models.Wash
	var createdAt string
	if err := sc.Scan(&w.ID, &w.RegistrationNumber, &w.WashType, &w.Status, &createdAt); err != nil {
		return models.Wash{}, err
	}
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return models.Wash{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	w.CreatedAt = t
	return w, nil
}
