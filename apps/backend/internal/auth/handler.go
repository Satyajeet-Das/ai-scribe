package auth

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"

	platformauth "github.com/Satyajeet-Das/ai-scribe/internal/platform/auth"
	httperrs "github.com/Satyajeet-Das/ai-scribe/internal/platform/http/errors"
	"github.com/Satyajeet-Das/ai-scribe/internal/platform/validation"
	"github.com/Satyajeet-Das/ai-scribe/internal/user"
)

const (
	RefreshTokenCookieName = "refresh_token"
	RefreshTokenPath       = "/api/v1/auth"
)

type Handler struct {
	service Service
	isProd  bool
}

func NewHandler(service Service, isProd bool) *Handler {
	return &Handler{
		service: service,
		isProd:  isProd,
	}
}

func (h *Handler) RegisterRoutes(g *echo.Group, authMiddleware echo.MiddlewareFunc, rateLimitMiddleware ...echo.MiddlewareFunc) {
	authGroup := g.Group("/auth")

	// Public routes
	authGroup.POST("/register", h.Register)
	if len(rateLimitMiddleware) > 0 {
		authGroup.POST("/login", h.Login, rateLimitMiddleware...)
	} else {
		authGroup.POST("/login", h.Login)
	}
	authGroup.POST("/refresh", h.Refresh)

	// Protected routes
	authGroup.POST("/logout", h.Logout, authMiddleware)
	authGroup.GET("/me", h.Me, authMiddleware)
}

func (h *Handler) Register(c echo.Context) error {
	var req RegisterRequest
	if err := validation.BindAndValidate(c, &req); err != nil {
		return err
	}

	userResp, err := h.service.Register(c.Request().Context(), req)
	if err != nil {
		if errors.Is(err, user.ErrUserAlreadyExists) {
			return httperrs.NewConflictError("User with this email already exists", true)
		}
		if errors.Is(err, ErrPasswordTooShort) || errors.Is(err, ErrInvalidRole) {
			return httperrs.NewBadRequestError(err.Error(), true, nil, nil, nil)
		}
		return httperrs.NewInternalServerError()
	}

	return c.JSON(http.StatusCreated, RegisterResponse{User: *userResp})
}

func (h *Handler) Login(c echo.Context) error {
	var req LoginRequest
	if err := validation.BindAndValidate(c, &req); err != nil {
		return err
	}

	loginResp, rawRefreshToken, refreshExpiry, err := h.service.Login(c.Request().Context(), req)
	if err != nil {
		if errors.Is(err, platformauth.ErrInvalidCredentials) {
			return httperrs.NewUnauthorizedError("Invalid email or password", true)
		}
		if errors.Is(err, platformauth.ErrUserDeactivated) {
			return httperrs.NewUnauthorizedError("User account is deactivated", true)
		}
		return httperrs.NewInternalServerError()
	}

	// Set refresh token in HttpOnly cookie
	cookie := &http.Cookie{
		Name:     RefreshTokenCookieName,
		Value:    rawRefreshToken,
		Path:     RefreshTokenPath,
		HttpOnly: true,
		Secure:   c.Scheme() == "https" || h.isProd,
		SameSite: http.SameSiteStrictMode,
		Expires:  refreshExpiry,
		MaxAge:   int(time.Until(refreshExpiry).Seconds()),
	}
	c.SetCookie(cookie)

	return c.JSON(http.StatusOK, loginResp)
}

func (h *Handler) Refresh(c echo.Context) error {
	rawRefreshToken := ""

	// Check cookie first
	if cookie, err := c.Cookie(RefreshTokenCookieName); err == nil && cookie.Value != "" {
		rawRefreshToken = cookie.Value
	}

	// Fallback to X-Refresh-Token header (convenient for API clients / Postman)
	if rawRefreshToken == "" {
		rawRefreshToken = strings.TrimSpace(c.Request().Header.Get("X-Refresh-Token"))
	}

	if rawRefreshToken == "" {
		return httperrs.NewUnauthorizedError("Missing refresh token", true)
	}

	refreshResp, newRawRefreshToken, refreshExpiry, err := h.service.Refresh(c.Request().Context(), rawRefreshToken)
	if err != nil {
		h.clearRefreshCookie(c)
		if errors.Is(err, platformauth.ErrInvalidToken) ||
			errors.Is(err, platformauth.ErrExpiredToken) ||
			errors.Is(err, platformauth.ErrRevokedToken) ||
			errors.Is(err, platformauth.ErrUserNotFound) ||
			errors.Is(err, platformauth.ErrUserDeactivated) {
			return httperrs.NewUnauthorizedError("Invalid or expired refresh token", true)
		}
		return httperrs.NewInternalServerError()
	}

	// Set rotated refresh token cookie
	cookie := &http.Cookie{
		Name:     RefreshTokenCookieName,
		Value:    newRawRefreshToken,
		Path:     RefreshTokenPath,
		HttpOnly: true,
		Secure:   c.Scheme() == "https" || h.isProd,
		SameSite: http.SameSiteStrictMode,
		Expires:  refreshExpiry,
		MaxAge:   int(time.Until(refreshExpiry).Seconds()),
	}
	c.SetCookie(cookie)

	return c.JSON(http.StatusOK, refreshResp)
}

func (h *Handler) Logout(c echo.Context) error {
	rawRefreshToken := ""
	if cookie, err := c.Cookie(RefreshTokenCookieName); err == nil {
		rawRefreshToken = cookie.Value
	}
	if rawRefreshToken == "" {
		rawRefreshToken = strings.TrimSpace(c.Request().Header.Get("X-Refresh-Token"))
	}

	identity, _ := platformauth.GetIdentity(c)

	_ = h.service.Logout(c.Request().Context(), identity, rawRefreshToken)
	h.clearRefreshCookie(c)

	return c.JSON(http.StatusOK, LogoutResponse{Message: "logged out successfully"})
}

func (h *Handler) Me(c echo.Context) error {
	userID, err := platformauth.GetCallerUserID(c)
	if err != nil {
		return httperrs.NewUnauthorizedError("Unauthorized", false)
	}

	userResp, err := h.service.GetMe(c.Request().Context(), userID)
	if err != nil {
		if errors.Is(err, platformauth.ErrUserNotFound) {
			return httperrs.NewUnauthorizedError("User not found", false)
		}
		return httperrs.NewInternalServerError()
	}

	return c.JSON(http.StatusOK, userResp)
}

func (h *Handler) clearRefreshCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     RefreshTokenCookieName,
		Value:    "",
		Path:     RefreshTokenPath,
		HttpOnly: true,
		Secure:   c.Scheme() == "https" || h.isProd,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}
