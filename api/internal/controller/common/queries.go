package common

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

func FamToPznMap(db *sqlx.DB, pzns []string) (map[uint64]string, error) {
	n := len(pzns)
	queryBuilder := squirrel.Select("PZN", "Key_FAM").From("PAE_DB").Where(squirrel.Eq{"PZN": pzns}).Limit(uint64(n))
	query, args, _ := queryBuilder.ToSql()

	var paePairs []struct {
		PZN    string `db:"PZN"`
		KeyFAM uint64 `db:"Key_FAM"`
	}

	err := db.Select(&paePairs, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error fetching FAM-PZN pairs: %w", err)
	}
	famPZNMap := make(map[uint64]string, len(paePairs))
	for _, paePair := range paePairs {
		famPZNMap[paePair.KeyFAM] = paePair.PZN
	}

	return famPZNMap, nil
}

// StoToCompoundsMap returns a map of STO keys to a list of compounds.
// The compounds are normalized and matched with the input compounds.
//  1. This means that the compounds in the map are the same as the input compounds if they exist in the database.
//  2. The Sto Key is key of the ACTIVE substance in the database.
//     e.g. for verapamil hydrochloride, the STO key would be the key of verapamil.
func StoToCompoundsMap(db *sqlx.DB, compounds []string) (map[uint64][]string, error) {
	queryBuilder := squirrel.Select(
		"Name",
		"CASE WHEN Typ = 100 THEN Key_STO_1 ELSE Key_STO END AS DDI_Key_STO").
		Distinct().
		From("SNA_DB").
		LeftJoin("VSS_DB ON SNA_DB.Key_STO = VSS_DB.Key_STO_2").
		Where(squirrel.Eq{"Name": compounds})

	type SnaPair struct {
		Name      string `db:"Name"`
		DDIKeySTO uint64 `db:"DDI_Key_STO"`
	}

	var snaPairs []SnaPair
	query, args, _ := queryBuilder.ToSql()
	err := db.Select(&snaPairs, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error fetching Compound-STO pairs: %w", err)
	}

	// normalize and match the compounds to the input compounds
	dbNames := make([]string, 0, len(snaPairs))
	for _, pair := range snaPairs {
		dbNames = append(dbNames, pair.Name)
	}
	normalizeAndMatch(dbNames, compounds)
	for i := range snaPairs {
		snaPairs[i].Name = dbNames[i]
	}

	result := make(map[uint64][]string)
	for _, pair := range snaPairs {
		result[pair.DDIKeySTO] = append(result[pair.DDIKeySTO], pair.Name)
	}

	return result, nil
}

func normalizeAndMatch(db, input []string) {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

	// Map to store normalized input strings for quick comparison
	inputMap := make(map[string]int)
	for i, in := range input {
		normalized, _, _ := transform.String(t, in)
		inputMap[strings.ToLower(normalized)] = i
	}

	// Transform and compare db strings in place
	for i, dbStr := range db {
		normalized, _, _ := transform.String(t, dbStr)
		normalized = strings.ToLower(normalized)
		if inputIndex, exists := inputMap[normalized]; exists {
			// Replace with matched input string
			db[i] = input[inputIndex]
		}
	}
}
