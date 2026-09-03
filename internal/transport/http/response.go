package httptransport

import "github.com/gin-gonic/gin"

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code      string       `json:"code"`
	Message   string       `json:"message"`
	RequestID string       `json:"requestId,omitempty"`
	Fields    []FieldError `json:"fields,omitempty"`
}

type FieldError struct {
	Field string `json:"field"`
	Rule  string `json:"rule"`
}

func RespondError(c *gin.Context, status int, code, message string, fields []FieldError) {
	c.JSON(status, ErrorResponse{Error: ErrorBody{
		Code:      code,
		Message:   message,
		RequestID: RequestIDFrom(c),
		Fields:    fields,
	}})
}
