package tests

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"go-carwash-api/database"
	"go-carwash-api/handlers"
	"go-carwash-api/models"
)

// newTestServer spins up the API against a fresh SQLite file in a temp dir.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	store, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	mux := http.NewServeMux()
	handlers.NewWashHandler(store).Register(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// doJSON sends a request with an optional JSON body and returns the response plus its body.
func doJSON(t *testing.T, method, url string, body any) (*http.Response, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req, err := http.NewRequest(method, url, &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	var out bytes.Buffer
	out.ReadFrom(resp.Body)
	return resp, out.Bytes()
}

func TestCreateAndGetWash(t *testing.T) {
	srv := newTestServer(t)

	resp, body := doJSON(t, http.MethodPost, srv.URL+"/washes", map[string]string{
		"registration_number": "abc 123",
		"wash_type":           "premium",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("POST /washes: want 201, got %d: %s", resp.StatusCode, body)
	}

	var created models.Wash
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ID == 0 {
		t.Error("expected a non-zero id")
	}
	if created.RegistrationNumber != "ABC123" {
		t.Errorf("registration should be normalised to ABC123, got %q", created.RegistrationNumber)
	}
	if created.Status != models.StatusQueued {
		t.Errorf("new wash should be queued, got %q", created.Status)
	}
	if created.CreatedAt.IsZero() {
		t.Error("expected created_at to be set")
	}

	resp, body = doJSON(t, http.MethodGet, srv.URL+"/washes/1", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /washes/1: want 200, got %d: %s", resp.StatusCode, body)
	}
	var fetched models.Wash
	if err := json.Unmarshal(body, &fetched); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if fetched.ID != created.ID || fetched.RegistrationNumber != created.RegistrationNumber ||
		fetched.WashType != created.WashType || fetched.Status != created.Status ||
		!fetched.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("GET returned %+v, want %+v", fetched, created)
	}
}

func TestCreateWashValidation(t *testing.T) {
	srv := newTestServer(t)

	cases := []struct {
		name string
		body map[string]string
	}{
		{"missing registration", map[string]string{"wash_type": "basic"}},
		{"registration too long", map[string]string{"registration_number": "ABCDEFGHIJKLMNOP", "wash_type": "basic"}},
		{"unknown wash type", map[string]string{"registration_number": "ABC123", "wash_type": "deluxe"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := doJSON(t, http.MethodPost, srv.URL+"/washes", tc.body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Errorf("want 400, got %d: %s", resp.StatusCode, body)
			}
		})
	}
}

func TestStatusLifecycle(t *testing.T) {
	srv := newTestServer(t)

	doJSON(t, http.MethodPost, srv.URL+"/washes", map[string]string{
		"registration_number": "XYZ789",
		"wash_type":           "basic",
	})

	patch := func(status string) (*http.Response, []byte) {
		return doJSON(t, http.MethodPatch, srv.URL+"/washes/1/status", map[string]string{"status": status})
	}

	// queued -> done is not allowed; the wash has to go through in_progress first.
	if resp, body := patch(models.StatusDone); resp.StatusCode != http.StatusConflict {
		t.Errorf("queued->done: want 409, got %d: %s", resp.StatusCode, body)
	}

	// queued -> in_progress -> done is the happy path.
	for _, s := range []string{models.StatusInProgress, models.StatusDone} {
		resp, body := patch(s)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("->%s: want 200, got %d: %s", s, resp.StatusCode, body)
		}
		var w models.Wash
		json.Unmarshal(body, &w)
		if w.Status != s {
			t.Errorf("status after patch: want %q, got %q", s, w.Status)
		}
		if w.UpdatedAt.IsZero() || w.UpdatedAt.Before(w.CreatedAt) {
			t.Errorf("updated_at (%v) should be set and not before created_at (%v)", w.UpdatedAt, w.CreatedAt)
		}
	}

	// done is terminal.
	if resp, body := patch(models.StatusCancelled); resp.StatusCode != http.StatusConflict {
		t.Errorf("done->cancelled: want 409, got %d: %s", resp.StatusCode, body)
	}

	// Filtering by status works.
	resp, body := doJSON(t, http.MethodGet, srv.URL+"/washes?status=done", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /washes?status=done: want 200, got %d: %s", resp.StatusCode, body)
	}
	var list []models.Wash
	json.Unmarshal(body, &list)
	if len(list) != 1 {
		t.Errorf("want 1 done wash, got %d", len(list))
	}
}

func TestDeleteWash(t *testing.T) {
	srv := newTestServer(t)

	doJSON(t, http.MethodPost, srv.URL+"/washes", map[string]string{
		"registration_number": "DEL001",
		"wash_type":           "standard",
	})

	if resp, body := doJSON(t, http.MethodDelete, srv.URL+"/washes/1", nil); resp.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE: want 204, got %d: %s", resp.StatusCode, body)
	}
	if resp, _ := doJSON(t, http.MethodGet, srv.URL+"/washes/1", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET after delete: want 404, got %d", resp.StatusCode)
	}
	if resp, _ := doJSON(t, http.MethodDelete, srv.URL+"/washes/1", nil); resp.StatusCode != http.StatusNotFound {
		t.Errorf("second DELETE: want 404, got %d", resp.StatusCode)
	}
}

// TestMigrateOldDatabase opens a database created with the first schema version
// (no updated_at column) and checks that the store upgrades it in place.
func TestMigrateOldDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.db")

	old, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw sqlite: %v", err)
	}
	_, err = old.Exec(`
		CREATE TABLE washes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			registration_number TEXT NOT NULL,
			wash_type TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'queued',
			created_at TEXT NOT NULL
		);
		INSERT INTO washes (registration_number, wash_type, status, created_at)
		VALUES ('OLD123', 'basic', 'queued', '2026-01-01T10:00:00Z');`)
	if err != nil {
		t.Fatalf("seed old schema: %v", err)
	}
	old.Close()

	store, err := database.Open(path)
	if err != nil {
		t.Fatalf("open store on old db: %v", err)
	}
	defer store.Close()

	w, err := store.Get(t.Context(), 1)
	if err != nil {
		t.Fatalf("get migrated row: %v", err)
	}
	if !w.UpdatedAt.Equal(w.CreatedAt) {
		t.Errorf("updated_at should be backfilled from created_at, got %v vs %v", w.UpdatedAt, w.CreatedAt)
	}

	// Opening again must not fail or re-run the ALTER.
	again, err := database.Open(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	again.Close()
}
