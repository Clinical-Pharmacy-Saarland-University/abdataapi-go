package validate

import (
	"errors"
	"fmt"
	"observeddb-go-api/internal/utils/helper"
	"regexp"
)

func PZNs(pzns []string, mindrugs, maxdrugs int) error {
	if len(pzns) < mindrugs {
		return fmt.Errorf("at least %d PZNs must be provided", mindrugs)
	}

	if len(pzns) > maxdrugs {
		return fmt.Errorf("too many PZNs provided. Maximum is %d", maxdrugs)
	}

	if err := valPZNBatch(pzns); err != nil {
		return fmt.Errorf("invalid PZNs provided: %s", err.Error())
	}

	if !helper.IsUnique(pzns) {
		return errors.New("duplicate PZNs provided")
	}

	return nil
}

func Compounds(compounds []string, maxdrugs int) error {
	if len(compounds) < 2 {
		return errors.New("at least two compounds must be provided")
	}

	if len(compounds) > maxdrugs {
		return fmt.Errorf("too many compounds provided. Maximum is %d", maxdrugs)
	}

	if !helper.IsUnique(compounds) {
		return errors.New("duplicate compounds provided")
	}

	return nil
}

func valPZNBatch(pzns []string) error {
	for _, pzn := range pzns {
		err := valSinglePZN(pzn)
		if err != nil {
			return err
		}
	}

	return nil
}

func valSinglePZN(pzn string) error {
	if len(pzn) != 8 || !regexp.MustCompile(`^\d{8}$`).MatchString(pzn) {
		return fmt.Errorf("PZN `%s` must be 8 digits", pzn)
	}

	// checksum calculation
	sum := 0
	for i := range [7]int{} {
		sum += int(pzn[i]-'0') * (i + 1)
	}

	rem := sum % 11

	if rem == 10 || rem != int(pzn[7]-'0') {
		return fmt.Errorf("checksum test for `%s` failed", pzn)
	}

	return nil
}
