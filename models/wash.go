// Package models defines the domain types shared by the database and HTTP layers.
package models

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

// Wash is a single car wash job.
type Wash struct {
	ID                 int64     `json:"id"`
	RegistrationNumber string    `json:"registration_number"`
	WashType           string    `json:"wash_type"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"created_at"`
}

// Wash types a customer can order.
const (
	WashTypeBasic    = "basic"
	WashTypeStandard = "standard"
	WashTypePremium  = "premium"
)

// Lifecycle of a wash job: queued -> in_progress -> done, or cancelled at any point before done.
const (
	StatusQueued     = "queued"
	StatusInProgress = "in_progress"
	StatusDone       = "done"
	StatusCancelled  = "cancelled"
)

var (
	ErrInvalidRegistration = errors.New("registration_number must be 2-10 letters or digits")
	ErrInvalidWashType     = errors.New("wash_type must be one of: basic, standard, premium")
	ErrInvalidStatus       = errors.New("status must be one of: queued, in_progress, done, cancelled")
	ErrNotFound            = errors.New("wash not found")
)

var registrationPattern = regexp.MustCompile(`^[A-Z0-9]{2,10}$`)

// NormalizeRegistration uppercases and strips spaces/dashes so "abc 123" and "ABC-123" both become "ABC123".
func NormalizeRegistration(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	return s
}

// ValidateRegistration reports whether a normalized registration number is acceptable.
func ValidateRegistration(reg string) error {
	if !registrationPattern.MatchString(reg) {
		return ErrInvalidRegistration
	}
	return nil
}

// ValidateWashType reports whether the wash type is one we offer.
func ValidateWashType(t string) error {
	switch t {
	case WashTypeBasic, WashTypeStandard, WashTypePremium:
		return nil
	}
	return ErrInvalidWashType
}

// ValidateStatus reports whether the status value is a known one.
func ValidateStatus(s string) error {
	switch s {
	case StatusQueued, StatusInProgress, StatusDone, StatusCancelled:
		return nil
	}
	return ErrInvalidStatus
}

// CanTransition reports whether a wash may move from one status to another.
// Finished (done) and cancelled washes are terminal.
func CanTransition(from, to string) bool {
	if from == to {
		return true
	}
	switch from {
	case StatusQueued:
		return to == StatusInProgress || to == StatusCancelled
	case StatusInProgress:
		return to == StatusDone || to == StatusCancelled
	}
	return false
}
