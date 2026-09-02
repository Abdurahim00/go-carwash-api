package models

import "testing"

func TestNormalizeRegistration(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ABC123", "ABC123"},
		{"abc123", "ABC123"},
		{"  abc 123  ", "ABC123"},
		{"ABC-123", "ABC123"},
		{"abc - 12 3", "ABC123"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := NormalizeRegistration(tc.in); got != tc.want {
			t.Errorf("NormalizeRegistration(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestValidateRegistration(t *testing.T) {
	valid := []string{"AB", "ABC123", "1234567890", "MLB007"}
	for _, reg := range valid {
		if err := ValidateRegistration(reg); err != nil {
			t.Errorf("ValidateRegistration(%q) = %v, want nil", reg, err)
		}
	}

	invalid := []string{
		"",            // empty
		"A",           // too short
		"ABCDEFGHIJK", // 11 chars, too long
		"abc123",      // lowercase: caller must normalise first
		"ABC 123",     // space: caller must normalise first
		"ÅBC123",      // non-ASCII letter
		"ABC_123",     // underscore
	}
	for _, reg := range invalid {
		if err := ValidateRegistration(reg); err == nil {
			t.Errorf("ValidateRegistration(%q) = nil, want error", reg)
		}
	}
}

func TestValidateWashType(t *testing.T) {
	for _, wt := range []string{WashTypeBasic, WashTypeStandard, WashTypePremium} {
		if err := ValidateWashType(wt); err != nil {
			t.Errorf("ValidateWashType(%q) = %v, want nil", wt, err)
		}
	}
	for _, wt := range []string{"", "deluxe", "Premium", "PREMIUM"} {
		if err := ValidateWashType(wt); err == nil {
			t.Errorf("ValidateWashType(%q) = nil, want error", wt)
		}
	}
}

func TestValidateStatus(t *testing.T) {
	for _, s := range []string{StatusQueued, StatusInProgress, StatusDone, StatusCancelled} {
		if err := ValidateStatus(s); err != nil {
			t.Errorf("ValidateStatus(%q) = %v, want nil", s, err)
		}
	}
	for _, s := range []string{"", "pending", "in-progress", "Done"} {
		if err := ValidateStatus(s); err == nil {
			t.Errorf("ValidateStatus(%q) = nil, want error", s)
		}
	}
}

// TestCanTransition checks the full state machine: every (from, to) pair.
func TestCanTransition(t *testing.T) {
	all := []string{StatusQueued, StatusInProgress, StatusDone, StatusCancelled}

	allowed := map[string]map[string]bool{
		StatusQueued:     {StatusInProgress: true, StatusCancelled: true},
		StatusInProgress: {StatusDone: true, StatusCancelled: true},
		StatusDone:       {},
		StatusCancelled:  {},
	}

	for _, from := range all {
		for _, to := range all {
			want := from == to || allowed[from][to]
			if got := CanTransition(from, to); got != want {
				t.Errorf("CanTransition(%q, %q) = %v, want %v", from, to, got, want)
			}
		}
	}
}
