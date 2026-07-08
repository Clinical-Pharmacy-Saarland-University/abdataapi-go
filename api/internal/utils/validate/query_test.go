package validate_test

import (
	"strings"
	"testing"

	"observeddb-go-api/internal/utils/validate"
)

func TestPZN(t *testing.T) {
	t.Parallel()

	// KNOWN-VALID PZNs (verified via weighted-checksum: weights 1..7 over
	// the first 7 digits must equal the 8th digit; a remainder of 10 is invalid).
	validPZNs := []string{
		"03041347",
		"05538454",
		"13880764",
		"00189747",
		"01970060",
		"00054065",
		"17145955",
		"00592733",
		"13981502",
	}

	tests := []struct {
		name    string
		pzn     string
		wantErr bool
	}{
		{name: "too short (7 digits)", pzn: "1234567", wantErr: true},
		{name: "non-digit character", pzn: "1234567a", wantErr: true},
		{name: "empty", pzn: "", wantErr: true},
		{name: "too long (9 digits)", pzn: "030413470", wantErr: true},
		// Same-length numeric with a wrong check digit: 03041347 is valid,
		// so 03041348 has the wrong 8th digit and must fail the checksum test.
		{name: "wrong check digit", pzn: "03041348", wantErr: true},
		// 03041340 also fails the checksum (different wrong check digit).
		{name: "another wrong check digit", pzn: "03041340", wantErr: true},
	}

	for _, p := range validPZNs {
		tests = append(tests, struct {
			name    string
			pzn     string
			wantErr bool
		}{name: "valid " + p, pzn: p, wantErr: false})
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validate.PZN(tt.pzn)
			if tt.wantErr && err == nil {
				t.Errorf("PZN(%q) = nil, want error", tt.pzn)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("PZN(%q) = %v, want nil", tt.pzn, err)
			}
		})
	}
}

func TestPZNs(t *testing.T) {
	t.Parallel()

	const (
		minDrugs = 2
		maxDrugs = 4
	)

	tests := []struct {
		name    string
		pzns    []string
		wantErr bool
		errSub  string
	}{
		{
			name:    "too few (below min)",
			pzns:    []string{"03041347"},
			wantErr: true,
			errSub:  "at least",
		},
		{
			name:    "empty (below min)",
			pzns:    []string{},
			wantErr: true,
			errSub:  "at least",
		},
		{
			name:    "too many (above max)",
			pzns:    []string{"03041347", "05538454", "13880764", "00189747", "01970060"},
			wantErr: true,
			errSub:  "too many",
		},
		{
			name:    "invalid member",
			pzns:    []string{"03041347", "1234567a"},
			wantErr: true,
			errSub:  "invalid PZNs",
		},
		{
			name:    "duplicate members",
			pzns:    []string{"03041347", "03041347"},
			wantErr: true,
			errSub:  "duplicate",
		},
		{
			name:    "valid at min",
			pzns:    []string{"03041347", "05538454"},
			wantErr: false,
		},
		{
			name:    "valid at max",
			pzns:    []string{"03041347", "05538454", "13880764", "00189747"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validate.PZNs(tt.pzns, minDrugs, maxDrugs)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("PZNs(%v) = nil, want error", tt.pzns)
				}
				if tt.errSub != "" && !strings.Contains(err.Error(), tt.errSub) {
					t.Errorf("PZNs(%v) error = %q, want substring %q", tt.pzns, err.Error(), tt.errSub)
				}
				return
			}
			if err != nil {
				t.Errorf("PZNs(%v) = %v, want nil", tt.pzns, err)
			}
		})
	}
}

func TestCompounds(t *testing.T) {
	t.Parallel()

	const maxDrugs = 3

	tests := []struct {
		name      string
		compounds []string
		wantErr   bool
		errSub    string
	}{
		{
			name:      "too few (only one)",
			compounds: []string{"Apixaban"},
			wantErr:   true,
			errSub:    "at least two",
		},
		{
			name:      "empty (too few)",
			compounds: []string{},
			wantErr:   true,
			errSub:    "at least two",
		},
		{
			name:      "too many (above max)",
			compounds: []string{"Apixaban", "Bisoprolol", "Mirtazapin", "Ibuprofen"},
			wantErr:   true,
			errSub:    "too many",
		},
		{
			name:      "duplicate compounds",
			compounds: []string{"Apixaban", "Apixaban"},
			wantErr:   true,
			errSub:    "duplicate",
		},
		{
			name:      "valid two compounds",
			compounds: []string{"Apixaban", "Bisoprolol"},
			wantErr:   false,
		},
		{
			name:      "valid at max with comma-containing name",
			compounds: []string{"Apixaban", "Mirtazapin-0,5-Wasser", "Bisoprolol"},
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validate.Compounds(tt.compounds, maxDrugs)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Compounds(%v) = nil, want error", tt.compounds)
				}
				if tt.errSub != "" && !strings.Contains(err.Error(), tt.errSub) {
					t.Errorf("Compounds(%v) error = %q, want substring %q", tt.compounds, err.Error(), tt.errSub)
				}
				return
			}
			if err != nil {
				t.Errorf("Compounds(%v) = %v, want nil", tt.compounds, err)
			}
		})
	}
}
