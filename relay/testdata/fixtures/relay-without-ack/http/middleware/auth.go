package middleware

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"basegraph.app/relay/internal/model"
	"basegraph.app/relay/internal/service"
	"github.com/gin-gonic/gin"
)

type contextKey string

const (
	sessionCookieName              = "relay_session"
	userContextKey      contextKey = "user"
	sessionIDContextKey contextKey = "session_id"
)

// ! TODO: @nithinsj -- Add this middleware to all routes that require authentication.
func RequireAuth(authService service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID, err := getSessionID(c)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		user, _, err := authService.ValidateSession(c.Request.Context(), sessionID)
		if err != nil {
			if errors.Is(err, service.ErrSessionExpired) || errors.Is(err, service.ErrUserNotFound) {
				clearSessionCookie(c)
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "session expired"})
				return
			}
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to validate session"})
			return
		}

		ctx := context.WithValue(c.Request.Context(), userContextKey, user)
		ctx = context.WithValue(ctx, sessionIDContextKey, sessionID)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

// OptionalAuth attaches the user to context if a valid session exists, but never aborts.
// Use for routes that work for both guests and authenticated users.
func OptionalAuth(authService service.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID, err := getSessionID(c)
		if err != nil {
			c.Next()
			return
		}

		user, _, err := authService.ValidateSession(c.Request.Context(), sessionID)
		if err != nil {
			c.Next()
			return
		}

		ctx := context.WithValue(c.Request.Context(), userContextKey, user)
		ctx = context.WithValue(ctx, sessionIDContextKey, sessionID)
		c.Request = c.Request.WithContext(ctx)

		c.Next()
	}
}

func GetUser(ctx context.Context) *model.User {
	user, _ := ctx.Value(userContextKey).(*model.User)
	return user
}

func GetSessionID(ctx context.Context) int64 {
	sessionID, _ := ctx.Value(sessionIDContextKey).(int64)
	return sessionID
}

func getSessionID(c *gin.Context) (int64, error) {
	cookie, err := c.Cookie(sessionCookieName)
	if err != nil {
		return 0, err
	}
	return strconv.ParseInt(cookie, 10, 64)
}

func clearSessionCookie(c *gin.Context) {
	c.SetCookie(
		sessionCookieName,
		"",
		-1,
		"/",
		"",
		false,
		true,
	)
}
