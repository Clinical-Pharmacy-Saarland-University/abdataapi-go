package apierr

import (
	"errors"
	"fmt"
	"net/http"
	"observeddb-go-api/internal/utils/logger"
	"reflect"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

type Error struct {
	statusCode int
	message    string
}

func (e Error) Error() string {
	if e.statusCode == http.StatusInternalServerError {
		return "Internal server error"
	}
	return e.message
}

func (e Error) Status() int {
	return e.statusCode
}

func (e Error) Message() string {
	return e.message
}

func (e Error) Log(c *gin.Context) {
	switch e.Status() {
	case http.StatusUnauthorized:
		logger.LogUnauthorizedError(c)
	case http.StatusForbidden:
		logger.LogForbiddenError(c, e.Error())
	case http.StatusInternalServerError:
		logger.LogServerError(c, errors.New(e.Message()))
	}
}

func New(statusCode int, msg string) Error {
	return Error{
		statusCode: statusCode,
		message:    msg,
	}
}

type ResStatus struct {
	Status  int    `json:"status" example:"200"`      // HTTP status code of the query
	Message string `json:"message" example:"Success"` // Status message (e.g. error message)
} //	@name	Status

func (r ResStatus) Ok() bool {
	return r.Status == http.StatusOK
}

func ToResponse(c *gin.Context, err error) ResStatus {
	if err == nil {
		return ResStatus{http.StatusOK, "Success"}
	}

	var apiErr Error
	if !errors.As(err, &apiErr) {
		apiErr = New(http.StatusInternalServerError, err.Error())
	}

	apiErr.Log(c)

	return ResStatus{apiErr.Status(), apiErr.Error()}
}

type ValidationError struct {
	Field  string `json:"field" example:"query_field"` // Field Query or JSON field
	Reason string `json:"reason" example:"reason"`     // Validation error reason
} //	@name	ValidationError

func ValidationErrors(verr validator.ValidationErrors, obj interface{}) []ValidationError {
	var errs []ValidationError

	// Resolve the underlying struct type, unwrapping pointers and slice/array wrappers so
	// this also works for batch endpoints that bind a []Struct body.
	structType := reflect.TypeOf(obj)
	for structType != nil && (structType.Kind() == reflect.Pointer ||
		structType.Kind() == reflect.Slice || structType.Kind() == reflect.Array) {
		structType = structType.Elem()
	}

	for _, fe := range verr {
		// Prefer the JSON (or form) tag from the struct field; fall back to the field name.
		fieldTag := fe.StructField()
		if structType != nil && structType.Kind() == reflect.Struct {
			if field, ok := structType.FieldByName(fe.StructField()); ok {
				if jsonTag := field.Tag.Get("json"); jsonTag != "" {
					fieldTag = jsonTag
				} else if formTag := field.Tag.Get("form"); formTag != "" {
					fieldTag = formTag
				}
			}
		}

		reason := fe.ActualTag()
		if fe.Param() != "" {
			reason = fmt.Sprintf("%s=%s", reason, fe.Param())
		}

		errs = append(errs, ValidationError{Field: fieldTag, Reason: reason})
	}

	return errs
}

func BatchStatusCode(nTotal, nSuccess int) int {
	if nSuccess == nTotal {
		return http.StatusOK
	}

	if nSuccess == 0 {
		return http.StatusMultiStatus
	}
	return http.StatusMultiStatus
}
