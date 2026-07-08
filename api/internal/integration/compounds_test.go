//go:build integration

package integration_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"observeddb-go-api/internal/controller/common"
	"observeddb-go-api/internal/controller/compoundcontroller"
	"observeddb-go-api/internal/controller/interactioncontroller"
	"observeddb-go-api/internal/utils/format"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

func newCompoundController(db *sqlx.DB) *compoundcontroller.CompoundController {
	return &compoundcontroller.CompoundController{DB: db, Limits: testLimits()}
}

func newInteractionController(db *sqlx.DB) *interactioncontroller.InteractionController {
	return &interactioncontroller.InteractionController{
		DB:                     db,
		Limits:                 testLimits(),
		PlausibilityTranslator: format.NewIntPlausibilityTranslator(),
		RelevanceTranslator:    format.NewIntRelevanceTranslator(),
		FrequencyTranslator:    format.NewIntFrequencyTranslator(),
		CredibilityTranslator:  format.NewIntCredibilityTranslator(),
		DirectionTranslator:    format.NewIntDirectionTranslator(),
		DescriptionStruct:      format.Description(),
	}
}

// resolveCanonicalNames calls GET /compounds/names?names=<base> against the real database
// and returns every canonical compound name it resolves to.
func resolveCanonicalNames(t *testing.T, db *sqlx.DB, base string) []string {
	t.Helper()

	r := gin.New()
	r.GET("/compounds/names", newCompoundController(db).GetSelectCompounds)

	req := httptest.NewRequest(http.MethodGet, "/compounds/names?names="+url.QueryEscape(base), nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /compounds/names?names=%s: status %d, body %s", base, w.Code, w.Body.String())
	}

	var resp struct {
		Data []struct {
			Input   string `json:"input"`
			Matches [][]struct {
				Name string `json:"name"`
			} `json:"matches"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode /compounds/names response: %v (body: %s)", err, w.Body.String())
	}

	var names []string
	for _, d := range resp.Data {
		for _, group := range d.Matches {
			for _, m := range group {
				names = append(names, m.Name)
			}
		}
	}
	return names
}

func firstContainingComma(names []string) string {
	for _, n := range names {
		if strings.Contains(n, ",") {
			return n
		}
	}
	return ""
}

// TestCompoundNamesResolvesCommaBearingCanonical asserts the real-data premise behind the
// whole fix: resolving the base INN "Mirtazapin" yields a canonical name containing a comma
// (e.g. "Mirtazapin-0,5-Wasser").
func TestCompoundNamesResolvesCommaBearingCanonical(t *testing.T) {
	db := dialOrSkip(t)

	names := resolveCanonicalNames(t, db, "Mirtazapin")
	if len(names) == 0 {
		t.Fatal("expected /compounds/names?names=Mirtazapin to resolve at least one name")
	}

	comma := firstContainingComma(names)
	if comma == "" {
		t.Fatalf("expected at least one canonical name containing a comma, got: %v", names)
	}
	t.Logf("resolved comma-bearing canonical name: %q", comma)
}

// TestInteractionsAcceptCommaBearingCompound proves the fix end-to-end against real data: a
// comma-bearing canonical name sent via repeated `compounds` parameters resolves (no 404),
// whereas the same names joined into a single comma-separated value are torn apart and 404.
func TestInteractionsAcceptCommaBearingCompound(t *testing.T) {
	db := dialOrSkip(t)

	comma := firstContainingComma(resolveCanonicalNames(t, db, "Mirtazapin"))
	if comma == "" {
		t.Skip("no comma-bearing canonical name in this dataset; cannot exercise the case")
	}

	// A second, distinct compound that resolves in this dataset. Adjust the candidate list
	// if none of these are present in your ABDA data.
	var partner string
	for _, base := range []string{"Simvastatin", "Ibuprofen", "Metformin", "Bisoprolol", "Apixaban"} {
		for _, n := range resolveCanonicalNames(t, db, base) {
			if n != comma {
				partner = n
				break
			}
		}
		if partner != "" {
			break
		}
	}
	if partner == "" {
		t.Skip("could not resolve a second compound to pair with in this dataset")
	}

	r := gin.New()
	r.GET("/interactions/compounds", newInteractionController(db).GetInterCompounds)

	// Repeated form: the intra-name comma survives, so both compounds resolve -> not a 404.
	repeated := "compounds=" + url.QueryEscape(comma) + "&compounds=" + url.QueryEscape(partner)
	req := httptest.NewRequest(http.MethodGet, "/interactions/compounds?"+repeated, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound || strings.Contains(w.Body.String(), "Compounds not found") {
		t.Fatalf("repeated-param comma name should resolve, got %d: %s", w.Code, w.Body.String())
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for two resolvable compounds, got %d: %s", w.Code, w.Body.String())
	}

	// Contrast: a single comma-joined value splits the comma-bearing name -> 404.
	single := "compounds=" + url.QueryEscape(comma+","+partner)
	req2 := httptest.NewRequest(http.MethodGet, "/interactions/compounds?"+single, nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	if w2.Code != http.StatusNotFound {
		t.Errorf("legacy single comma-joined value carrying a comma-bearing name should 404, got %d: %s",
			w2.Code, w2.Body.String())
	}
}

// TestStoToCompoundsMapExecutesAgainstRealSchema is a query-validity smoke test: the
// squirrel-built SQL executes against the real ABDA schema and resolves a known compound.
func TestStoToCompoundsMapExecutesAgainstRealSchema(t *testing.T) {
	db := dialOrSkip(t)

	comma := firstContainingComma(resolveCanonicalNames(t, db, "Mirtazapin"))
	if comma == "" {
		t.Skip("no comma-bearing canonical name to look up")
	}

	m, err := common.StoToCompoundsMap(db, []string{comma})
	if err != nil {
		t.Fatalf("StoToCompoundsMap failed against the real schema: %v", err)
	}

	found := false
	for _, bucket := range m {
		for _, name := range bucket {
			if name == comma {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("expected %q in StoToCompoundsMap result, got %v", comma, m)
	}
}
