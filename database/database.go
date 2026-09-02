// Package database owns the SQLite connection and all SQL for the washes table.
package database

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"strings"
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
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &Store{db: db}, nil
}

// migrate brings databases created by older versions of the schema up to date.
// Each step is idempotent so it is safe to run on every start.
func migrate(db *sql.DB) error {
	hasUpdatedAt, err := columnExists(db, "washes", "updated_at")
	if err != nil {
		return err
	}
	if !hasUpdatedAt {
		// SQLite cannot add a NOT NULL column without a default, so add it with a
		// placeholder and then backfill from created_at.
		if _, err := db.Exec(`ALTER TABLE washes ADD COLUMN updated_at TEXT NOT NULL DEFAULT ''`); err != nil {
			return fmt.Errorf("add updated_at: %w", err)
		}
		if _, err := db.Exec(`UPDATE washes SET updated_at = created_at WHERE updated_at = ''`); err != nil {
			return fmt.Errorf("backfill updated_at: %w", err)
		}
	}
	return nil
}

func columnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// Close releases the underlying connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Create inserts a new wash in the queued state and returns it with its ID and timestamp.
func (s *Store) Create(ctx context.Context, registration, washType string) (models.Wash, error) {
	now := time.Now().UTC().Truncate(time.Second)
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO washes (registration_number, wash_type, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		registration, washType, models.StatusQueued, now.Format(time.RFC3339), now.Format(time.RFC3339),
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
		UpdatedAt:          now,
	}, nil
}

// ListFilter narrows a List call. Empty fields are ignored.
type ListFilter struct {
	Status             string
	RegistrationNumber string
}

// List returns washes matching the filter, newest first.
func (s *Store) List(ctx context.Context, f ListFilter) ([]models.Wash, error) {
	query := `SELECT id, registration_number, wash_type, status, created_at, updated_at FROM washes`
	var where []string
	var args []any
	if f.Status != "" {
		where = append(where, `status = ?`)
		args = append(args, f.Status)
	}
	if f.RegistrationNumber != "" {
		where = append(where, `registration_number = ?`)
		args = append(args, f.RegistrationNumber)
	}
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, ` AND `)
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
		`SELECT id, registration_number, wash_type, status, created_at, updated_at FROM washes WHERE id = ?`, id)
	w, err := scanWash(row)
	if errors.Is(err, sql.ErrNoRows) {
		return models.Wash{}, models.ErrNotFound
	}
	return w, err
}

// UpdateStatus changes the status of a wash and returns the updated row.
// The caller is responsible for validating the transition.
func (s *Store) UpdateStatus(ctx context.Context, id int64, status string) (models.Wash, error) {
	now := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `UPDATE washes SET status = ?, updated_at = ? WHERE id = ?`, status, now, id)
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
	var createdAt, updatedAt string
	if err := sc.Scan(&w.ID, &w.RegistrationNumber, &w.WashType, &w.Status, &createdAt, &updatedAt); err != nil {
		return models.Wash{}, err
	}
	var err error
	if w.CreatedAt, err = time.Parse(time.RFC3339, createdAt); err != nil {
		return models.Wash{}, fmt.Errorf("parse created_at %q: %w", createdAt, err)
	}
	if w.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt); err != nil {
		return models.Wash{}, fmt.Errorf("parse updated_at %q: %w", updatedAt, err)
	}
	return w, nil
}
