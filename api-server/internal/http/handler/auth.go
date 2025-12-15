package handler

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"basegraph.app/api-server/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	sessionCookieName  = "relay_session"
	stateCookieName    = "relay_oauth_state"
	sessionIDHeader    = "X-Session-ID"
	sessionMaxAge      = 7 * 24 * 60 * 60
	sessionMaxAgeHours = 7 * 24
)

type AuthHandler struct {
	authService  service.AuthService
	dashboardURL string
	isProduction bool
}

func NewAuthHandler(authService service.AuthService, dashboardURL string, isProduction bool) *AuthHandler {
	return &AuthHandler{
		authService:  authService,
		dashboardURL: dashboardURL,
		isProduction: isProduction,
	}
}

func (h *AuthHandler) Login(c *gin.Context) {
	state, err := generateState()
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "failed to generate state", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initiate login"})
		return
	}

	authURL, err := h.authService.GetAuthorizationURL(state)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "failed to get authorization URL", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initiate login"})
		return
	}

	c.SetCookie(
		stateCookieName,
		state,
		600,
		"/",
		"",
		h.isProduction,
		true,
	)

	c.Redirect(http.StatusTemporaryRedirect, authURL)
}

func (h *AuthHandler) Callback(c *gin.Context) {
	ctx := c.Request.Context()

	code := c.Query("code")
	state := c.Query("state")
	errorParam := c.Query("error")
	errorDescription := c.Query("error_description")

	if errorParam != "" {
		slog.WarnContext(ctx, "OAuth error", "error", errorParam, "description", errorDescription)
		c.Redirect(http.StatusTemporaryRedirect, h.dashboardURL+"?auth_error="+errorParam)
		return
	}

	storedState, err := c.Cookie(stateCookieName)
	if err != nil || state != storedState {
		slog.WarnContext(ctx, "state mismatch", "expected", storedState, "got", state)
		c.Redirect(http.StatusTemporaryRedirect, h.dashboardURL+"?auth_error=invalid_state")
		return
	}

	h.clearStateCookie(c)

	if code == "" {
		c.Redirect(http.StatusTemporaryRedirect, h.dashboardURL+"?auth_error=no_code")
		return
	}

	user, session, err := h.authService.HandleCallback(ctx, code)
	if err != nil {
		slog.ErrorContext(ctx, "failed to handle callback", "error", err)
		if errors.Is(err, service.ErrInvalidCode) {
			c.Redirect(http.StatusTemporaryRedirect, h.dashboardURL+"?auth_error=invalid_code")
			return
		}
		c.Redirect(http.StatusTemporaryRedirect, h.dashboardURL+"?auth_error=callback_failed")
		return
	}

	h.setSessionCookie(c, session.ID)

	slog.InfoContext(ctx, "user logged in", "user_id", user.ID, "email", user.Email)

	c.Redirect(http.StatusTemporaryRedirect, h.dashboardURL+"/dashboard")
}

func (h *AuthHandler) Logout(c *gin.Context) {
	ctx := c.Request.Context()

	sessionID, err := h.getSessionID(c)
	if err == nil && sessionID > 0 {
		if err := h.authService.Logout(ctx, sessionID); err != nil {
			slog.WarnContext(ctx, "failed to delete session", "error", err, "session_id", sessionID)
		}
	}

	h.clearSessionCookie(c)

	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

func (h *AuthHandler) Me(c *gin.Context) {
	ctx := c.Request.Context()

	sessionID, err := h.getSessionID(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}

	user, _, err := h.authService.ValidateSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, service.ErrSessionExpired) || errors.Is(err, service.ErrUserNotFound) {
			h.clearSessionCookie(c)
			c.JSON(http.StatusUnauthorized, gin.H{"error": "session expired"})
			return
		}
		slog.ErrorContext(ctx, "failed to validate session", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         user.ID,
		"name":       user.Name,
		"email":      user.Email,
		"avatar_url": user.AvatarURL,
	})
}

func (h *AuthHandler) setSessionCookie(c *gin.Context, sessionID int64) {
	c.SetCookie(
		sessionCookieName,
		strconv.FormatInt(sessionID, 10),
		sessionMaxAge,
		"/",
		"",
		h.isProduction,
		true,
	)
}

func (h *AuthHandler) clearSessionCookie(c *gin.Context) {
	c.SetCookie(
		sessionCookieName,
		"",
		-1,
		"/",
		"",
		h.isProduction,
		true,
	)
}

func (h *AuthHandler) clearStateCookie(c *gin.Context) {
	c.SetCookie(
		stateCookieName,
		"",
		-1,
		"/",
		"",
		h.isProduction,
		true,
	)
}

func (h *AuthHandler) getSessionID(c *gin.Context) (int64, error) {
	cookie, err := c.Cookie(sessionCookieName)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(cookie, 10, 64)
}

func generateState() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

type GetAuthURLResponse struct {
	AuthorizationURL string `json:"authorization_url"`
	State            string `json:"state"`
}

func (h *AuthHandler) GetAuthURL(c *gin.Context) {
	state, err := generateState()
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "failed to generate state", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate state"})
		return
	}

	authURL, err := h.authService.GetAuthorizationURL(state)
	if err != nil {
		slog.ErrorContext(c.Request.Context(), "failed to get authorization URL", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get authorization URL"})
		return
	}

	c.JSON(http.StatusOK, GetAuthURLResponse{
		AuthorizationURL: authURL,
		State:            state,
	})
}

type ExchangeRequest struct {
	Code string `json:"code" binding:"required"`
}

type ExchangeResponse struct {
	User      UserResponse `json:"user"`
	SessionID string       `json:"session_id"`
	ExpiresIn int          `json:"expires_in"`
}

type UserResponse struct {
	AvatarURL *string `json:"avatar_url,omitempty"`
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Email     string  `json:"email"`
}

func (h *AuthHandler) Exchange(c *gin.Context) {
	ctx := c.Request.Context()

	var req ExchangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "code is required"})
		return
	}

	user, session, err := h.authService.HandleCallback(ctx, req.Code)
	if err != nil {
		slog.ErrorContext(ctx, "failed to exchange code", "error", err)
		if errors.Is(err, service.ErrInvalidCode) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid authorization code"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to exchange code"})
		return
	}

	slog.InfoContext(ctx, "user authenticated via exchange", "user_id", user.ID, "email", user.Email)

	c.JSON(http.StatusOK, ExchangeResponse{
		User: UserResponse{
			ID:        strconv.FormatInt(user.ID, 10),
			Name:      user.Name,
			Email:     user.Email,
			AvatarURL: user.AvatarURL,
		},
		SessionID: strconv.FormatInt(session.ID, 10),
		ExpiresIn: sessionMaxAgeHours,
	})
}

type ValidateSessionResponse struct {
	OrganizationID  *string      `json:"organization_id,omitempty"`
	WorkspaceID     *string      `json:"workspace_id,omitempty"`
	User            UserResponse `json:"user"`
	HasOrganization bool         `json:"has_organization"`
}

func (h *AuthHandler) ValidateSession(c *gin.Context) {
	ctx := c.Request.Context()

	sessionIDStr := c.GetHeader(sessionIDHeader)
	if sessionIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "session ID required"})
		return
	}

	sessionID, err := strconv.ParseInt(sessionIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session ID"})
		return
	}

	user, userCtx, err := h.authService.ValidateSession(ctx, sessionID)
	if err != nil {
		if errors.Is(err, service.ErrSessionExpired) || errors.Is(err, service.ErrUserNotFound) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "session expired"})
			return
		}
		slog.ErrorContext(ctx, "failed to validate session", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to validate session"})
		return
	}

	resp := ValidateSessionResponse{
		User: UserResponse{
			ID:        strconv.FormatInt(user.ID, 10),
			Name:      user.Name,
			Email:     user.Email,
			AvatarURL: user.AvatarURL,
		},
		HasOrganization: userCtx.HasOrganization,
	}

	if userCtx.OrganizationID != nil {
		orgIDStr := strconv.FormatInt(*userCtx.OrganizationID, 10)
		resp.OrganizationID = &orgIDStr
	}
	if userCtx.WorkspaceID != nil {
		wsIDStr := strconv.FormatInt(*userCtx.WorkspaceID, 10)
		resp.WorkspaceID = &wsIDStr
	}

	c.JSON(http.StatusOK, resp)
}

type LogoutRequest struct {
	SessionID string `json:"session_id" binding:"required"`
}

func (h *AuthHandler) LogoutSession(c *gin.Context) {
	ctx := c.Request.Context()

	var req LogoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}

	sessionID, err := strconv.ParseInt(req.SessionID, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid session ID"})
		return
	}

	if err := h.authService.Logout(ctx, sessionID); err != nil {
		slog.WarnContext(ctx, "failed to delete session", "error", err, "session_id", sessionID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}
