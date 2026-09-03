package httptransport

import (
	"errors"
	"io"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

func BindJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		code := "invalid_json"
		message := "request body is not valid JSON"
		var fields []FieldError

		var validationErrors validator.ValidationErrors
		if errors.As(err, &validationErrors) {
			code = "validation_failed"
			message = "request validation failed"
			fields = make([]FieldError, 0, len(validationErrors))
			for _, validationError := range validationErrors {
				fields = append(fields, FieldError{
					Field: validationError.Field(),
					Rule:  validationError.Tag(),
				})
			}
			sort.Slice(fields, func(i, j int) bool { return fields[i].Field < fields[j].Field })
		} else if errors.Is(err, io.EOF) {
			code = "empty_body"
			message = "request body is required"
		}

		RespondError(c, http.StatusBadRequest, code, message, fields)
		return false
	}
	return true
}
