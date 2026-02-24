package middleware

import (
	"encoding/json"
	"go-game/utils"
	"net/http"

	"github.com/gin-gonic/gin"
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
		utils.Info("[AUTH] 处理请求: %s %s", utils.F("method", c.Request.Method), utils.F("path", c.Request.URL.Path))
		var token string

		// 尝试从 Cookie 获取
		tokenCookie, err := c.Cookie("access_token")
		if err == nil && tokenCookie != "" {
			token = tokenCookie
			utils.Info("[AUTH] 从Cookie获取token成功")
		} else {
			utils.Info("[AUTH] 错误: Cookie 中不存在 access_token")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token", "details": "access_token cookie is empty"})
			return
		}

		// 调用登录中心验证 token
		utils.Info("[AUTH] 准备调用认证中心验证token: %s/verify-token", utils.F("url", authCenterURL))
		req, _ := http.NewRequest("POST", authCenterURL+"/verify-token", nil)
		// 只透传 Cookie，不使用 Authorization 头
		cookieHeader := c.Request.Header.Get("Cookie")
		if cookieHeader != "" {
			req.Header.Set("Cookie", cookieHeader)
			utils.Info("[AUTH] 透传Cookie到认证中心")
		} else {
			// 兜底：如果没有 Cookie 头但拿到了 token，则以 Cookie 形式附加
			if token != "" {
				req.AddCookie(&http.Cookie{Name: "access_token", Value: token})
				utils.Info("[AUTH] 无原始Cookie头，附加 access_token Cookie")
			}
		}
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			utils.Error("[AUTH] 错误: 调用认证中心失败: %v", utils.F("error", err))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token", "details": "auth server connection error"})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			utils.Error("[AUTH] 错误: 认证中心返回非200状态码: %d", utils.F("status_code", resp.StatusCode))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token", "details": "auth server rejected token"})
			return
		}

		var wrapped AuthCenterWrappedResponse
		if err := json.NewDecoder(resp.Body).Decode(&wrapped); err != nil {
			utils.Error("[AUTH] 错误: 解析认证响应失败: %v", utils.F("error", err))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token", "details": "failed to parse auth response"})
			return
		}
		utils.Info("[AUTH] 认证中心业务状态码: %d, 消息: %s, 数据: %+v", utils.F("status_code", wrapped.StatusCode), utils.F("message", wrapped.Message), utils.F("data", wrapped.Data))
		if wrapped.StatusCode != http.StatusOK {
			utils.Error("[AUTH] 错误: 认证中心业务状态码非200: %d, 消息: %s", utils.F("status_code", wrapped.StatusCode), utils.F("message", wrapped.Message))
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token", "details": wrapped.Message})
			return
		}

		if wrapped.Data.UserID == 0 {
			utils.Error("[AUTH] 错误: 认证响应中UserID为0，token无效")
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token", "details": "invalid user id"})
			return
		}

		// 保存用户信息到上下文
		utils.Info("[AUTH] 认证成功: UserID = %d, Email = %s, Name = %s", utils.F("user_id", wrapped.Data.UserID), utils.F("email", wrapped.Data.Email), utils.F("name", wrapped.Data.Name))
		c.Set("user_id", wrapped.Data.UserID)
		c.Set("email", wrapped.Data.Email)
		c.Set("avatar", wrapped.Data.Avatar)
		c.Set("name", wrapped.Data.Name)
		c.Next()
	}
}
