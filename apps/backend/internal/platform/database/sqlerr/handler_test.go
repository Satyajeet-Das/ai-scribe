package sqlerr

import (
	"database/sql"
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	httperrs "github.com/Satyajeet-Das/ai-scribe/internal/platform/http/errors"
)

func TestHandleError_SqlNoRows(t *testing.T) {
	err := sql.ErrNoRows
	handled := HandleError(err)

	var httpErr *httperrs.HTTPError
	assert.True(t, errors.As(handled, &httpErr))
	assert.Equal(t, http.StatusNotFound, httpErr.Status)
}

func TestHandleError_AlreadyHTTPError(t *testing.T) {
	original := httperrs.NewBadRequestError("Bad request", false, nil, nil, nil)
	handled := HandleError(original)

	assert.Equal(t, original, handled)
}

func TestMapCode(t *testing.T) {
	assert.Equal(t, UniqueViolation, MapCode("23505"))
	assert.Equal(t, ForeignKeyViolation, MapCode("23503"))
	assert.Equal(t, NotNullViolation, MapCode("23502"))
	assert.Equal(t, CheckViolation, MapCode("23514"))
	assert.Equal(t, Other, MapCode("99999"))
}
