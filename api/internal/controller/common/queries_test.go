package common_test

import (
	"testing"

	"observeddb-go-api/internal/controller/common"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestFamToPZN(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")

	// Two PZNs share FAM 100, one PZN belongs to FAM 200.
	rows := sqlmock.NewRows([]string{"PZN", "Key_FAM"}).
		AddRow("11111111", uint64(100)).
		AddRow("22222222", uint64(100)).
		AddRow("33333333", uint64(200))

	mock.ExpectQuery("FROM PAE_DB").
		WithArgs("11111111", "22222222", "33333333").
		WillReturnRows(rows)

	got, err := common.FamToPZN(sqlxDB, []string{"11111111", "22222222", "33333333"})
	if err != nil {
		t.Fatalf("FamToPZN returned error: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 FAM groups, got %d: %v", len(got), got)
	}

	fam100 := got[100]
	if len(fam100) != 2 {
		t.Fatalf("expected FAM 100 to have 2 PZNs, got %d: %v", len(fam100), fam100)
	}
	// Order is preserved from the row order.
	if fam100[0] != "11111111" || fam100[1] != "22222222" {
		t.Errorf("FAM 100 group = %v, want [11111111 22222222]", fam100)
	}

	fam200 := got[200]
	if len(fam200) != 1 || fam200[0] != "33333333" {
		t.Errorf("FAM 200 group = %v, want [33333333]", fam200)
	}

	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unmet sqlmock expectations: %v", merr)
	}
}

func TestStoToCompoundsMap_CaseAndAccentInsensitiveMatch(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	sqlxDB := sqlx.NewDb(db, "sqlmock")

	// Input uses one spelling; DB rows differ only by case and/or accents.
	input := []string{"Metoprolol", "Amlodipin", "Amiodaron"}

	// DB returns lower-cased names, and an accented variant, all mapping back
	// to the original input spelling via normalizeAndMatch.
	rows := sqlmock.NewRows([]string{"Name", "DDI_Key_STO"}).
		AddRow("metoprolol", uint64(1)). // case-only difference
		AddRow("amlodipin", uint64(2)).  // case-only difference
		AddRow("Amiodàron", uint64(1))   // accent + shares STO with metoprolol

	// StoToCompoundsMap lower-cases and trims the compounds before querying,
	// so the bound arguments are the normalized (lower-cased) input spellings.
	mock.ExpectQuery("FROM SNA_DB").
		WithArgs("metoprolol", "amlodipin", "amiodaron").
		WillReturnRows(rows)

	got, err := common.StoToCompoundsMap(sqlxDB, input)
	if err != nil {
		t.Fatalf("StoToCompoundsMap returned error: %v", err)
	}

	// STO 1 should contain the ORIGINAL input spellings "Metoprolol" and "Amiodaron".
	sto1 := got[1]
	if !containsAll(sto1, []string{"Metoprolol", "Amiodaron"}) {
		t.Errorf("STO 1 = %v, want to contain original spellings [Metoprolol Amiodaron]", sto1)
	}

	// STO 2 should contain the ORIGINAL input spelling "Amlodipin".
	sto2 := got[2]
	if len(sto2) != 1 || sto2[0] != "Amlodipin" {
		t.Errorf("STO 2 = %v, want [Amlodipin]", sto2)
	}

	// None of the map values should retain the raw DB (lowercase/accented) spelling.
	for sto, names := range got {
		for _, n := range names {
			switch n {
			case "metoprolol", "amlodipin", "Amiodàron":
				t.Errorf("STO %d retained raw DB name %q; expected original input spelling", sto, n)
			}
		}
	}

	if merr := mock.ExpectationsWereMet(); merr != nil {
		t.Errorf("unmet sqlmock expectations: %v", merr)
	}
}

func containsAll(haystack, needles []string) bool {
	for _, n := range needles {
		found := false
		for _, h := range haystack {
			if h == n {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
