// Package handlers contains the HTTP layer: routing, JSON encoding and input validation.
package handlers

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"go-carwash-api/database"
	"go-carwash-api/models"
)

// WashHandler serves the /washes endpoints.
type WashHandler struct {
	store *database.Store
}

// NewWashHandler wires a handler to a store.
func NewWashHandler(store *database.Store) *WashHandler {
	return &WashHandler{store: store}
}

// Register mounts all routes on the given mux (Go 1.22+ method/pattern routing).
func (h *WashHandler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /washes", h.create)
	mux.HandleFunc("GET /washes", h.list)
	mux.HandleFunc("GET /washes/{id}", h.get)
	mux.HandleFunc("PATCH /washes/{id}/status", h.updateStatus)
	mux.HandleFunc("DELETE /washes/{id}", h.delete)
}

type createWashRequest struct {
	RegistrationNumber string `json:"registration_number"`
	WashType           string `json:"wash_type"`
}

type updateStatusRequest struct {
	Status string `json:"status"`
}

// POST /washes
func (h *WashHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createWashRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	reg := models.NormalizeRegistration(req.RegistrationNumber)
	if err := models.ValidateRegistration(reg); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := models.ValidateWashType(req.WashType); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	wash, err := h.store.Create(r.Context(), reg, req.WashType)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, wash)
}

// GET /washes?status=queued&registration_number=ABC123
func (h *WashHandler) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filter := database.ListFilter{Status: q.Get("status")}

	if filter.Status != "" {
		if err := models.ValidateStatus(filter.Status); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if raw := q.Get("registration_number"); raw != "" {
		// Same normalisation as on create, so "abc 123" finds "ABC123".
		filter.RegistrationNumber = models.NormalizeRegistration(raw)
		if err := models.ValidateRegistration(filter.RegistrationNumber); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	washes, err := h.store.List(r.Context(), filter)
	if err != nil {
		writeInternalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, washes)
}

// GET /washes/{id}
func (h *WashHandler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	wash, err := h.store.Get(r.Context(), id)
	if err != nil {
		handleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, wash)
}

// PATCH /washes/{id}/status
func (h *WashHandler) updateStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	var req updateStatusRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := models.ValidateStatus(req.Status); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	current, err := h.store.Get(r.Context(), id)
	if err != nil {
		handleStoreError(w, err)
		return
	}
	if !models.CanTransition(current.Status, req.Status) {
		writeError(w, http.StatusConflict,
			"cannot change status from "+current.Status+" to "+req.Status)
		return
	}

	wash, err := h.store.UpdateStatus(r.Context(), id, req.Status)
	if err != nil {
		handleStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, wash)
}

// DELETE /washes/{id}
func (h *WashHandler) delete(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	if err := h.store.Delete(r.Context(), id); err != nil {
		handleStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "id must be a positive integer")
		return 0, false
	}
	return id, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB is plenty for these payloads
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return errors.New("invalid JSON body: " + err.Error())
	}
	return nil
}

func handleStoreError(w http.ResponseWriter, err error) {
	if errors.Is(err, models.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeInternalError(w, err)
}

func writeInternalError(w http.ResponseWriter, err error) {
	log.Printf("internal error: %v", err)
	writeError(w, http.StatusInternalServerError, "internal server error")
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}
