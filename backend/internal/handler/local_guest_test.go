package handler

import (
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"testing"
)

func TestLocalGuestNetworkBoundary(t *testing.T) {
	t.Setenv("CANVAS_LOCAL_GUEST_ENABLED", "true")
	t.Setenv("CANVAS_LOCAL_GUEST_CIDRS", "10.42.0.0/16,127.0.0.1/32")
	for _, test := range []struct {
		remote, forwarded string
		want              bool
	}{
		{"10.42.2.5:1234", "", true},
		{"203.0.113.2:1234", "", false},
		{"203.0.113.2:1234", "10.42.1.2", false},
		{"127.0.0.1:1234", "10.42.1.9", true},
		{"127.0.0.1:1234", "10.42.1.2, 203.0.113.2", false},
	} {
		router := gin.New()
		_ = router.SetTrustedProxies([]string{"127.0.0.1"})
		router.GET("/test", func(c *gin.Context) {
			if got := localGuestAccessAllowed(c); got != test.want {
				t.Errorf("%s/%s allowed=%v", test.remote, test.forwarded, got)
			}
		})
		request := httptest.NewRequest("GET", "/test", nil)
		request.RemoteAddr = test.remote
		request.Header.Set("X-Forwarded-For", test.forwarded)
		router.ServeHTTP(httptest.NewRecorder(), request)
	}
}

func TestLocalGuestInvalidNetworkFailsClosed(t *testing.T) {
	t.Setenv("CANVAS_LOCAL_GUEST_ENABLED", "true")
	t.Setenv("CANVAS_LOCAL_GUEST_CIDRS", "10.42.0.0/16,invalid")
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/", nil)
	c.Request.RemoteAddr = "10.42.1.2:1"
	if localGuestAccessAllowed(c) {
		t.Fatal("invalid CIDR accepted")
	}
}
