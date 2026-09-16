package errors

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewNotFoundError(t *testing.T) {
	err := NewNotFoundError("Resource not found", false, nil)

	assert.Equal(t, http.StatusNotFound, err.Status)
	assert.Equal(t, "NOT_FOUND", err.Code)
	assert.Equal(t, "Resource not found", err.Message)
	assert.False(t, err.Override)
}

func TestNewBadRequestError(t *testing.T) {
	customCode := "INVALID_INPUT"
	fields := []FieldError{
		{Field: "title", Error: "is required"},
	}
	err := NewBadRequestError("Invalid request", true, &customCode, fields, nil)

	assert.Equal(t, http.StatusBadRequest, err.Status)
	assert.Equal(t, "INVALID_INPUT", err.Code)
	assert.Equal(t, "Invalid request", err.Message)
	assert.True(t, err.Override)
	assert.Len(t, err.Errors, 1)
	assert.Equal(t, "title", err.Errors[0].Field)
}

func TestNewUnauthorizedError(t *testing.T) {
	err := NewUnauthorizedError("Unauthorized access", false)

	assert.Equal(t, http.StatusUnauthorized, err.Status)
	assert.Equal(t, "UNAUTHORIZED", err.Code)
	assert.Equal(t, "Unauthorized access", err.Message)
}

func TestMakeUpperCaseWithUnderscores(t *testing.T) {
	assert.Equal(t, "NOT_FOUND", MakeUpperCaseWithUnderscores("Not Found"))
	assert.Equal(t, "BAD_REQUEST", MakeUpperCaseWithUnderscores("Bad Request"))
}
