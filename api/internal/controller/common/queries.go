package common

import (
	"fmt"

	"github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
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

func StoToCompoundMap(db *sqlx.DB, compounds []string) (map[uint64]string, error) {
	queryBuilder := squirrel.Select("Name", "Key_STO").From("SNA_DB").Where(squirrel.Eq{"Name": compounds})
	query, args, _ := queryBuilder.ToSql()

	var snaPairs []struct {
		Name   string `db:"Name"`
		KeySTO uint64 `db:"Key_STO"`
	}

	err := db.Select(&snaPairs, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error fetching Compound-STO pairs: %w", err)
	}

	stoCompoundMap := make(map[uint64]string)
	for _, pair := range snaPairs {
		stoCompoundMap[pair.KeySTO] = pair.Name
	}

	return stoCompoundMap, nil
}
