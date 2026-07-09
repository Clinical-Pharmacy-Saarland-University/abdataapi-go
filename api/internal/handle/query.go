package handle

import (
	"errors"
	"observeddb-go-api/internal/utils/apierr"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

// JSONBind binds the JSON body to the query struct and handles any errors.
func JSONBind(c *gin.Context, query interface{}) bool {
	if err := c.ShouldBindJSON(query); err != nil {
		handleBindErrors(c, err, query)
		return false
	}
	return true
}

// QueryBind binds the query parameters to the query struct and handles any errors.
func QueryBind(c *gin.Context, query interface{}) bool {
	if err := c.ShouldBindQuery(query); err != nil {
		handleBindErrors(c, err, query)
		return false
	}
	return true
}

// QueryList reads a list query parameter, supporting two transport formats:
//
//  1. Repeated query parameter, e.g. ?names=A&names=B&names=C. Each occurrence is
//     returned verbatim as its own item, so an item may contain a comma (such as ABDA
//     canonical compound names like "Mirtazapin-0,5-Wasser").
//  2. A single comma-joined value, e.g. ?names=A,B,C, which is split on commas. This is
//     kept for backward compatibility but cannot represent items that contain a comma.
//
// The two formats are indistinguishable on the wire when only one value is present, so a
// single value is always comma-split. A comma inside an item is therefore preserved only
// when at least two values are supplied via the repeated form; a lone comma-bearing value
// (e.g. ?names=Mirtazapin-0,5-Wasser on its own) is still split. The repeated form is
// unambiguous for multi-item lists and should be preferred by clients.
func QueryList(c *gin.Context, key string) []string {
	return NormalizeList(c.QueryArray(key))
}

// NormalizeList applies the list transport rule (see QueryList) to values that have
// already been collected from a query parameter. When exactly one value is present it is
// split on commas (legacy comma-joined list); when two or more values are present they are
// returned verbatim so that individual items may contain commas. Note that a single value
// is always split, even when supplied via the repeated form, because it cannot be told
// apart from a legacy comma-joined list.
func NormalizeList(values []string) []string {
	if len(values) == 1 {
		return strings.Split(values[0], ",")
	}
	return values
}

// IsEmptyList reports whether a normalized list parameter carries no usable items,
// i.e. it is empty or holds only a single empty value (as produced by an empty query
// parameter such as ?names=). It is used to reject a missing required list parameter
// while preserving the previous "missing required parameter" behavior.
func IsEmptyList(values []string) bool {
	return len(values) == 0 || (len(values) == 1 && values[0] == "")
}

func handleBindErrors(c *gin.Context, err error, query interface{}) {
	var verr validator.ValidationErrors
	if errors.As(err, &verr) {
		ferrs := apierr.ValidationErrors(verr, query)
		ValidationError(c, ferrs)
		return
	}

	// A slice/array body (the batch endpoints) reports field-validation failures as a
	// SliceValidationError holding one validator.ValidationErrors per element. Surface those
	// as a 422 too, so batch validation matches the single-object endpoints and the documented
	// "Bad query format" response. Genuinely malformed JSON is not a SliceValidationError and
	// still falls through to a 400.
	var sliceErr binding.SliceValidationError
	if errors.As(err, &sliceErr) {
		var ferrs []apierr.ValidationError
		for _, elemErr := range sliceErr {
			var v validator.ValidationErrors
			if errors.As(elemErr, &v) {
				ferrs = append(ferrs, apierr.ValidationErrors(v, query)...)
			}
		}
		if len(ferrs) > 0 {
			ValidationError(c, ferrs)
			return
		}
	}

	BadRequestError(c, err.Error())
}
