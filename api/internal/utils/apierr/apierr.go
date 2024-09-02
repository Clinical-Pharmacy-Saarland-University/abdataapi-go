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
	Status  int    `json:"status"`
	Message string `json:"message"`
}

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
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

func ValidationErrors(verr validator.ValidationErrors, obj interface{}) []ValidationError {
	var errors []ValidationError

	for _, fe := range verr {
		// Get the JSON tag or query tag from the struct field
		field, _ := reflect.TypeOf(obj).Elem().FieldByName(fe.StructField())
		fieldTag := field.Tag.Get("json")
		if fieldTag == "" {
			fieldTag = field.Tag.Get("form") // For query binding
		}
		if fieldTag == "" {
			fieldTag = fe.StructField() // Fallback to struct field name
		}

		err := fe.ActualTag()
		if fe.Param() != "" {
			err = fmt.Sprintf("%s=%s", err, fe.Param())
		}

		errors = append(errors, ValidationError{Field: fieldTag, Reason: err})
	}

	return errors
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
