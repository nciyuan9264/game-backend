package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestConfiguredOrigins(t *testing.T) {
	t.Setenv("CORS_ALLOW_ORIGINS", " http://127.0.0.1:5177, , http://localhost:5178 ")
	router := New()
	router.POST("/test", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for _, origin := range []string{"http://localhost:5173", "http://127.0.0.1:5173", "http://127.0.0.1:5177", "http://localhost:5178"} {
		request := httptest.NewRequest(http.MethodPost, "/test", nil)
		request.Header.Set("Origin", origin)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Origin") != origin {
			t.Errorf("origin %s: status %d, headers %v", origin, response.Code, response.Header())
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/test", nil)
	request.Header.Set("Origin", "https://untrusted.invalid")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("unconfigured origin: status %d", response.Code)
	}
}
