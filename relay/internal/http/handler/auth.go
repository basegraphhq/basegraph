package handler

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"basegraph.app/relay/internal/service"
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
	authService       service.AuthService
	invitationService service.InvitationService
	dashboardURL      string
	isProduction      bool
}

func NewAuthHandler(
	authService service.AuthService,
	invitationService service.InvitationService,
	dashboardURL string,
	isProduction bool,
) *AuthHandler {
	return &AuthHandler{
		authService:       authService,
		invitationService: invitationService,
		dashboardURL:      dashboardURL,
		isProduction:      isProduction,
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

	result, err := h.authService.HandleCallback(ctx, code)
	if err != nil {
		slog.ErrorContext(ctx, "failed to handle callback", "error", err)
		if errors.Is(err, service.ErrInvalidCode) {
			c.Redirect(http.StatusTemporaryRedirect, h.dashboardURL+"?auth_error=invalid_code")
			return
		}
		c.Redirect(http.StatusTemporaryRedirect, h.dashboardURL+"?auth_error=callback_failed")
		return
	}

	h.setSessionCookie(c, result.Session.ID)

	slog.InfoContext(ctx, "user logged in", "user_id", result.User.ID, "email", result.User.Email)

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

	// Build auth URL options
	var opts []service.AuthURLOption
	if loginHint := c.Query("login_hint"); loginHint != "" {
		opts = append(opts, service.WithLoginHint(loginHint))
	}

	authURL, err := h.authService.GetAuthorizationURL(state, opts...)
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
	Code        string  `json:"code" binding:"required"`
	InviteToken *string `json:"invite_token,omitempty"`
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

	var result *service.CallbackResult
	var err error

	if req.InviteToken != nil && *req.InviteToken != "" {
		result, err = h.authService.HandleCallback(ctx, req.Code)
		if err != nil {
			slog.ErrorContext(ctx, "failed to exchange code", "error", err)
			if errors.Is(err, service.ErrInvalidCode) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid authorization code"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to exchange code"})
			return
		}

		user := result.User
		session := result.Session

		_, err := h.invitationService.Accept(ctx, *req.InviteToken, user)
		if err != nil {
			slog.WarnContext(ctx, "failed to accept invitation",
				"error", err,
				"user_email", user.Email,
			)

			// Map specific errors to user-friendly messages
			switch {
			case errors.Is(err, service.ErrEmailMismatch):
				// For email mismatch, KEEP the session so user can logout properly.
				// The session contains the WorkOS session ID needed for full logout.
				// Return session_id so dashboard can set the cookie for logout flow.
				c.JSON(http.StatusForbidden, gin.H{
					"error":      "The email you signed in with doesn't match the invitation",
					"code":       "email_mismatch",
					"session_id": strconv.FormatInt(session.ID, 10),
				})
				return
			default:
				// For all other invite errors, delete the session since user can't proceed
				if delErr := h.authService.Logout(ctx, session.ID); delErr != nil {
					slog.WarnContext(ctx, "failed to delete session after invite failure",
						"error", delErr,
						"session_id", session.ID,
					)
				}

				switch {
				case errors.Is(err, service.ErrInviteExpired):
					c.JSON(http.StatusGone, gin.H{
						"error": "This invitation has expired",
						"code":  "invite_expired",
					})
				case errors.Is(err, service.ErrInviteAlreadyUsed):
					c.JSON(http.StatusGone, gin.H{
						"error": "This invitation has already been used",
						"code":  "invite_used",
					})
				case errors.Is(err, service.ErrInviteRevoked):
					c.JSON(http.StatusGone, gin.H{
						"error": "This invitation has been revoked",
						"code":  "invite_revoked",
					})
				case errors.Is(err, service.ErrInviteNotFound):
					c.JSON(http.StatusNotFound, gin.H{
						"error": "Invitation not found",
						"code":  "invite_not_found",
					})
				default:
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process invitation"})
				}
				return
			}
		}
		slog.InfoContext(ctx, "invitation accepted during auth exchange",
			"user_id", user.ID,
			"email", user.Email,
		)
	} else {
		result, err = h.authService.HandleSignIn(ctx, req.Code)
		if err != nil {
			slog.WarnContext(ctx, "sign-in failed", "error", err)
			if errors.Is(err, service.ErrInvalidCode) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid authorization code"})
				return
			}
			if errors.Is(err, service.ErrUserNotFound) {
				// User doesn't exist - they need an invite
				c.JSON(http.StatusForbidden, gin.H{
					"error": "Relay is invite-only. Please use an invitation link to sign up.",
					"code":  "invite_only",
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sign in"})
			return
		}
	}

	slog.InfoContext(ctx, "user authenticated via exchange", "user_id", result.User.ID, "email", result.User.Email)

	c.JSON(http.StatusOK, ExchangeResponse{
		User: UserResponse{
			ID:        strconv.FormatInt(result.User.ID, 10),
			Name:      result.User.Name,
			Email:     result.User.Email,
			AvatarURL: result.User.AvatarURL,
		},
		SessionID: strconv.FormatInt(result.Session.ID, 10),
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
	ReturnTo  string `json:"return_to,omitempty"`
}

type LogoutResponse struct {
	Message   string `json:"message"`
	LogoutURL string `json:"logout_url,omitempty"`
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

	// Get session first to retrieve WorkOS session ID for logout URL
	// We fetch before deleting so we can build the WorkOS logout URL
	session, err := h.authService.GetSessionByID(ctx, sessionID)
	if err != nil {
		slog.DebugContext(ctx, "session not found for logout URL", "error", err, "session_id", sessionID)
	}

	// Delete the session
	if err := h.authService.Logout(ctx, sessionID); err != nil {
		slog.WarnContext(ctx, "failed to delete session", "error", err, "session_id", sessionID)
	}

	// Build response with optional WorkOS logout URL
	resp := LogoutResponse{Message: "logged out"}
	if session != nil && session.WorkOSSessionID != nil && *session.WorkOSSessionID != "" {
		returnTo := req.ReturnTo
		if returnTo == "" {
			returnTo = h.dashboardURL
		}
		resp.LogoutURL = h.authService.GetLogoutURL(*session.WorkOSSessionID, returnTo)
	}

	c.JSON(http.StatusOK, resp)
}
