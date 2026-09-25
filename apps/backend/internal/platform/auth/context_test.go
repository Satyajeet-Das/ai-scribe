package auth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
)

func TestContextHelpers(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	// Initially empty
	identity, ok := platformauth.GetIdentity(c)
	assert.False(t, ok)
	assert.Nil(t, identity)

	_, err := platformauth.GetCallerUserID(c)
	assert.Error(t, err)

	role := platformauth.GetCallerRole(c)
	assert.Empty(t, role)

	// Set identity
	testID := uuid.New()
	testIdentity := &platformauth.AuthenticatedIdentity{
		UserID:    testID,
		Role:      platformauth.RoleTeacher,
		Email:     "teacher@example.com",
		Provider:  "jwt",
		ExpiresAt: time.Now().Add(15 * time.Minute),
	}

	platformauth.SetIdentity(c, testIdentity)

	identity, ok = platformauth.GetIdentity(c)
	require.True(t, ok)
	assert.Equal(t, testID, identity.UserID)
	assert.Equal(t, platformauth.RoleTeacher, identity.Role)

	// Test caller ID retrieval
	callerID, err := platformauth.GetCallerUserID(c)
	require.NoError(t, err)
	assert.Equal(t, testID, callerID)

	// Test backwards compatibility strings in context
	assert.Equal(t, testID.String(), c.Get("user_id"))
	assert.Equal(t, string(platformauth.RoleTeacher), c.Get("user_role"))

	// Test caller role retrieval
	assert.Equal(t, platformauth.RoleTeacher, platformauth.GetCallerRole(c))
}

func TestRoleValidation(t *testing.T) {
	assert.True(t, platformauth.RoleTeacher.IsValid())
	assert.True(t, platformauth.RoleStudent.IsValid())
	assert.True(t, platformauth.RoleAdmin.IsValid())
	assert.True(t, platformauth.RoleProctor.IsValid())
	assert.False(t, platformauth.Role("INVALID").IsValid())
}
