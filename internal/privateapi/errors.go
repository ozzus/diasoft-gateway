package privateapi

import "fmt"

type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	if e == nil {
		return "api error"
	}
	if e.Message == "" {
		return fmt.Sprintf("api error (%d)", e.StatusCode)
	}
	return e.Message
}

func NewAPIError(statusCode int, message string) *APIError {
	return &APIError{StatusCode: statusCode, Message: message}
}
