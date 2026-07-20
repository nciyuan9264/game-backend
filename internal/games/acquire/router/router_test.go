package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLoadtestRoutesAreNotRegisteredByDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("ACQUIRE_LOADTEST_ENABLED", "")

	r := gin.New()
	InitRouter(r)

	req := httptest.NewRequest(http.MethodGet, "/__loadtest/acquire/stats", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestLoadtestRoutesAreRegisteredWhenEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("ACQUIRE_LOADTEST_ENABLED", "true")
	t.Setenv("ACQUIRE_LOADTEST_TOKEN", "secret")

	r := gin.New()
	InitRouter(r)

	req := httptest.NewRequest(http.MethodGet, "/__loadtest/acquire/stats", nil)
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}
