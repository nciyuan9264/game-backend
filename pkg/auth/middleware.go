package auth

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nciyuan9264/game-backend/pkg/logger"
)

const defaultAuthCenterURL = "https://api.gamebus.online/pam-api/platform/auth"

var authHTTPClient = &http.Client{Timeout: 5 * time.Second}

type AuthCenterUser struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Avatar string `json:"avatar"`
	Name   string `json:"name"`
}

type AuthCenterResponse struct {
	Valid bool            `json:"valid"`
	User  *AuthCenterUser `json:"user"`
}

// 封装后的认证中心返回结构
type AuthCenterWrappedResponse struct {
	StatusCode int                `json:"status_code"`
	Message    string             `json:"message"`
	Data       AuthCenterResponse `json:"data"`
}

func CenterURL() string {
	if value := strings.TrimRight(strings.TrimSpace(os.Getenv("AUTH_CENTER_URL")), "/"); value != "" {
		return value
	}
	return defaultAuthCenterURL
}

func JWTMiddlewareViaAuthCenter(authCenterURL string) gin.HandlerFunc {
	authCenterURL = strings.TrimRight(strings.TrimSpace(authCenterURL), "/")
	return func(c *gin.Context) {
		method := c.Request.Method
		path := c.Request.URL.Path
		logger.Info("[AUTH] verify request", logger.F("method", method), logger.F("path", path))

		token, err := c.Cookie("access_token")
		if err != nil || token == "" {
			logger.Warn("[AUTH] missing token", logger.F("method", method), logger.F("path", path), logger.F("reason", "access_token cookie is empty"))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token", "details": "access_token cookie is empty"})
			return
		}

		req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, authCenterURL+"/verify-token", nil)
		if err != nil {
			logger.Error("[AUTH] create auth request failed", logger.F("error", err.Error()))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token", "details": "failed to create auth request"})
			return
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err := authHTTPClient.Do(req)
		if err != nil {
			logger.Error("[AUTH] auth center request failed", logger.F("error", err.Error()))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token", "details": "auth server connection error"})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			logger.Error("[AUTH] auth center rejected request", logger.F("status_code", resp.StatusCode))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token", "details": "auth server rejected token"})
			return
		}

		var wrapped AuthCenterWrappedResponse
		if err := json.NewDecoder(resp.Body).Decode(&wrapped); err != nil {
			logger.Error("[AUTH] parse auth response failed", logger.F("error", err.Error()))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token", "details": "failed to parse auth response"})
			return
		}
		if wrapped.StatusCode != http.StatusOK {
			logger.Error("[AUTH] auth center rejected token", logger.F("status_code", wrapped.StatusCode), logger.F("message", wrapped.Message))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token", "details": wrapped.Message})
			return
		}

		if !wrapped.Data.Valid || wrapped.Data.User == nil {
			logger.Error("[AUTH] invalid auth response", logger.F("reason", "token is invalid or user is missing"))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token", "details": "invalid auth response"})
			return
		}
		userID, parseErr := strconv.ParseUint(wrapped.Data.User.UserID, 10, strconv.IntSize)
		if parseErr != nil || userID == 0 {
			logger.Error("[AUTH] invalid auth user", logger.F("reason", "invalid user id"))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token", "details": "invalid user id"})
			return
		}

		// 保存用户信息到上下文
		logger.Info("[AUTH] verify success", logger.F("user_id", userID))
		c.Set("user_id", uint(userID))
		c.Set("email", wrapped.Data.User.Email)
		c.Set("avatar", wrapped.Data.User.Avatar)
		c.Set("name", wrapped.Data.User.Name)
		c.Next()
	}
}
