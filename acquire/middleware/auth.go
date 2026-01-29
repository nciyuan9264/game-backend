package middleware

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

type AuthCenterResponse struct {
	UserID uint   `json:"user_id"`
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
		log.Printf("[AUTH] 处理请求: %s %s", c.Request.Method, c.Request.URL.Path)
		var token string

		// 尝试从 Cookie 获取
		tokenCookie, err := c.Cookie("access_token")
		if err == nil && tokenCookie != "" {
			token = tokenCookie
			log.Printf("[AUTH] 从Cookie获取token成功")
		} else {
			log.Printf("[AUTH] 错误: Cookie 中不存在 access_token")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token", "details": "access_token cookie is empty"})
			return
		}

		// 调用登录中心验证 token
		log.Printf("[AUTH] 准备调用认证中心验证token: %s/verify-token", authCenterURL)
		req, _ := http.NewRequest("POST", authCenterURL+"/verify-token", nil)
		// 只透传 Cookie，不使用 Authorization 头
		cookieHeader := c.Request.Header.Get("Cookie")
		if cookieHeader != "" {
			req.Header.Set("Cookie", cookieHeader)
			log.Printf("[AUTH] 透传Cookie到认证中心")
		} else {
			// 兜底：如果没有 Cookie 头但拿到了 token，则以 Cookie 形式附加
			if token != "" {
				req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
				log.Printf("[AUTH] 无原始Cookie头，附加 access_token Cookie")
			}
		}
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Printf("[AUTH] 错误: 调用认证中心失败: %v", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token", "details": "auth server connection error"})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			log.Printf("[AUTH] 错误: 认证中心返回非200状态码")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token", "details": "auth server rejected token"})
			return
		}

		var wrapped AuthCenterWrappedResponse
		if err := json.NewDecoder(resp.Body).Decode(&wrapped); err != nil {
			log.Printf("[AUTH] 错误: 解析认证响应失败: %v", err)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token", "details": "failed to parse auth response"})
			return
		}
		log.Printf("[AUTH] 认证中心业务状态码: %d, 消息: %s, 数据: %+v", wrapped.StatusCode, wrapped.Message, wrapped.Data)
		if wrapped.StatusCode != http.StatusOK {
			log.Printf("[AUTH] 错误: 认证中心业务状态码非200: %d, 消息: %s", wrapped.StatusCode, wrapped.Message)
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token", "details": wrapped.Message})
			return
		}

		if wrapped.Data.UserID == 0 {
			log.Printf("[AUTH] 错误: 认证响应中UserID为0，token无效")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token", "details": "invalid user id"})
			return
		}

		// 保存用户信息到上下文
		log.Printf("[AUTH] 认证成功: UserID = %d, Email = %s, Name = %s", wrapped.Data.UserID, wrapped.Data.Email, wrapped.Data.Name)
		c.Set("user_id", wrapped.Data.UserID)
		c.Set("email", wrapped.Data.Email)
		c.Set("avatar", wrapped.Data.Avatar)
		c.Set("name", wrapped.Data.Name)
		c.Next()
	}
}
