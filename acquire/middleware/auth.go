package middleware

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type AuthCenterResponse struct {
	UserID uint   `json:"user_id"`
	Error  string `json:"error"`
}

func JWTMiddlewareViaAuthCenter(authCenterURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		var token string

		// 尝试从 Cookie 获取
		tokenCookie, err := c.Cookie("access_token")
		if err == nil && tokenCookie != "" {
			token = tokenCookie
		} else {
			// 尝试从 Authorization Header 获取
			ah := c.GetHeader("Authorization")
			if ah == "" || !strings.HasPrefix(ah, "Bearer ") {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
				return
			}
			token = strings.TrimPrefix(ah, "Bearer ")
		}

		// 调用登录中心验证 token
		req, _ := http.NewRequest("POST", authCenterURL+"/verify-token", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != 200 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		defer resp.Body.Close()

		var authResp AuthCenterResponse
		if err := json.NewDecoder(resp.Body).Decode(&authResp); err != nil || authResp.UserID == 0 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}

		// 保存用户信息到上下文
		c.Set("user_id", authResp.UserID)
		c.Next()
	}
}
