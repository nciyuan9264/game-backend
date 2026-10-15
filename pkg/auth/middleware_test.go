package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestCenterURL(t *testing.T) {
	t.Setenv("AUTH_CENTER_URL", "")
	if got := CenterURL(); got != defaultAuthCenterURL {
		t.Fatalf("CenterURL() = %q, want %q", got, defaultAuthCenterURL)
	}

	t.Setenv("AUTH_CENTER_URL", " https://example.com/pam-api/platform/auth/ ")
	if got := CenterURL(); got != "https://example.com/pam-api/platform/auth" {
		t.Fatalf("CenterURL() = %q", got)
	}
}

func TestJWTMiddlewareUsesPAMGatewayVerifyResponse(t *testing.T) {
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/pam-api/platform/auth/verify-token" {
			t.Fatalf("auth request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status_code": 200,
			"message": "ok",
			"data": {
				"user_id": "42",
				"roles": ["player"]
			}
		}`))
	}))
	defer authServer.Close()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/protected", JWTMiddlewareViaAuthCenter(authServer.URL+"/pam-api/platform/auth/"), func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		roles, _ := c.Get("roles")
		if userID != uint(42) || len(roles.([]string)) != 1 || roles.([]string)[0] != "player" {
			t.Fatalf("auth context user_id=%v roles=%v", userID, roles)
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "access-token"})
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", res.Code, res.Body.String())
	}
}

func TestAuthProfileMiddlewareUsesPAMGatewayProfileResponse(t *testing.T) {
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/pam-api/platform/auth/profile" {
			t.Fatalf("auth request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer access-token" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status_code": 200,
			"message": "ok",
			"data": {
				"user_id": "42",
				"email": "player@example.com",
				"name": "Player",
				"avatar": "https://cdn.example/avatar.png",
				"roles": ["player"]
			}
		}`))
	}))
	defer authServer.Close()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/protected", AuthProfileMiddlewareViaAuthCenter(authServer.URL+"/pam-api/platform/auth/"), func(c *gin.Context) {
		userID, _ := c.Get("user_id")
		email, _ := c.Get("email")
		name := c.GetString("name")
		avatar := c.GetString("avatar")
		if userID != uint(42) || email != "player@example.com" || name != "Player" || avatar == "" {
			t.Fatalf("profile context user_id=%v email=%v name=%q avatar=%q", userID, email, name, avatar)
		}
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "access-token"})
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusNoContent {
		t.Fatalf("status = %d body = %s", res.Code, res.Body.String())
	}
}

func TestJWTMiddlewareRejectsInvalidPAMGatewayResponse(t *testing.T) {
	authServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"status_code": 200,
			"message": "ok",
			"data": null
		}`))
	}))
	defer authServer.Close()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/protected", JWTMiddlewareViaAuthCenter(authServer.URL), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "access-token"})
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", res.Code, res.Body.String())
	}
}

func TestJWTMiddlewareTreatsCanceledAuthRequestAsClientClosed(t *testing.T) {
	previousClient := authHTTPClient
	authHTTPClient = &http.Client{
		Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, context.Canceled
		}),
	}
	t.Cleanup(func() {
		authHTTPClient = previousClient
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/protected", JWTMiddlewareViaAuthCenter("https://example.com/pam-api/platform/auth"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: "access_token", Value: "access-token"})
	res := httptest.NewRecorder()
	router.ServeHTTP(res, req)

	if res.Code != clientClosedRequestStatus {
		t.Fatalf("status = %d body = %s", res.Code, res.Body.String())
	}
}
