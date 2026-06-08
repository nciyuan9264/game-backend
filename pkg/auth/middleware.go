package auth

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nciyuan9264/game-backend/pkg/logger"
)

type AuthCenterResponse struct {
	UserID uint   `json:"id"`
	Email  string `json:"email"`
	Avatar string `json:"avatar"`
	Name   string `json:"name"`
}

// 封装后的认证中心返回结构
type AuthCenterWrappedResponse struct {
	StatusCode int                `json:"status_code"`
	Message    string             `json:"message"`
	Data       AuthCenterResponse `json:"data"`
}

func JWTMiddlewareViaAuthCenter(authCenterURL string) gin.HandlerFunc {
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

		req, err := http.NewRequest("POST", authCenterURL+"/verify-token", nil)
		if err != nil {
			logger.Error("[AUTH] create auth request failed", logger.F("error", err.Error()))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token", "details": "failed to create auth request"})
			return
		}
		req.Header.Set("Authorization", "Bearer "+token)

		client := &http.Client{}
		resp, err := client.Do(req)
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

		if wrapped.Data.UserID == 0 {
			logger.Error("[AUTH] invalid auth user", logger.F("reason", "invalid user id"))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token", "details": "invalid user id"})
			return
		}

		// 保存用户信息到上下文
		logger.Info("[AUTH] verify success", logger.F("user_id", wrapped.Data.UserID), logger.F("email", wrapped.Data.Email), logger.F("name", wrapped.Data.Name))
		c.Set("user_id", wrapped.Data.UserID)
		c.Set("email", wrapped.Data.Email)
		c.Set("avatar", wrapped.Data.Avatar)
		c.Set("name", wrapped.Data.Name)
		c.Next()
	}
}
