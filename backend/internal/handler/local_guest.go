package handler

import (
	"github.com/gin-gonic/gin"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"yingce/backend/internal/service"
)

func localGuestAccessAllowed(c *gin.Context) bool {
	if os.Getenv("CANVAS_LOCAL_GUEST_ENABLED") != "true" {
		return false
	}
	ip := net.ParseIP(c.ClientIP())
	if ip == nil {
		return false
	}
	entries := strings.Split(os.Getenv("CANVAS_LOCAL_GUEST_CIDRS"), ",")
	matched := false
	for _, entry := range entries {
		_, network, err := net.ParseCIDR(strings.TrimSpace(entry))
		if err != nil {
			return false
		}
		matched = matched || network.Contains(ip)
	}
	return matched
}

func registerLocalGuestRoutes(r *gin.RouterGroup, svc *service.Service) {
	r.POST("/auth/guest", func(c *gin.Context) {
		if !localGuestAccessAllowed(c) {
			c.AbortWithStatusJSON(403, gin.H{"code": 403, "msg": "访客模式仅限配置的可信内网"})
			return
		}
		if origin := c.GetHeader("Origin"); origin != "" {
			parsed, err := url.Parse(origin)
			expected, parseErr := url.Parse(os.Getenv("CANVAS_PUBLIC_BASE_URL"))
			if err != nil || parseErr != nil || parsed.Host == "" || parsed.Host != expected.Host {
				c.AbortWithStatus(403)
				return
			}
		}
		if !enforceRateLimit(c, "guest-create:"+c.ClientIP(), 10, time.Hour) {
			return
		}
		if user, err := currentUser(c, svc); err == nil {
			ok(c, gin.H{"user": user})
			return
		}
		result, err := svc.LocalGuestSession(loginRequestContext(c))
		if err != nil {
			failService(c, err)
			return
		}
		setSessionCookie(c, result.Session, result.MaxAgeSecs)
		ok(c, gin.H{"user": result.User})
	})
}

// Credentials, settings, custom upstreams, billing writes and administration stay protected.
func LocalGuestBoundary(svc *service.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil || !svc.IsLocalGuest(user) {
			c.Next()
			return
		}
		if !localGuestAccessAllowed(c) {
			c.AbortWithStatusJSON(403, gin.H{"code": 403, "msg": "访客访问不在可信内网范围"})
			return
		}
		path := c.Request.URL.Path
		_, _, rawProxy := shortSystemProxyPath(path)
		denied := strings.HasPrefix(path, "/api/admin") || strings.HasPrefix(path, "/api/ai/") || strings.HasPrefix(path, "/api/channels") || strings.HasPrefix(path, "/api/agent") || strings.HasPrefix(path, "/api/cloud-agent") || strings.HasPrefix(path, "/api/plugins") || strings.HasPrefix(path, "/api/oss")
		if strings.HasPrefix(path, "/api/auth/") && path != "/api/auth/session" && path != "/api/auth/settings" && path != "/api/auth/login" && path != "/api/auth/logout" && path != "/api/auth/guest" {
			denied = true
		}
		if (strings.HasPrefix(path, "/api/wallet") || strings.HasPrefix(path, "/api/settings")) && c.Request.Method != http.MethodGet {
			denied = true
		}
		if denied || rawProxy {
			c.AbortWithStatusJSON(403, gin.H{"code": 403, "msg": "此操作需要正式账号权限"})
			return
		}
		c.Next()
	}
}

func localGuestFeatures(value *service.PublicFeatureAvailability) *service.PublicFeatureAvailability {
	clone := *value
	clone.CreditsEnabled = false
	clone.CustomChannelsEnabled = false
	clone.PluginCenterEnabled = false
	clone.SystemPluginsVisibleToUsers = false
	clone.ShortDramaEnabled = false
	clone.TimelineTranscriptionEnabled = false
	return &clone
}
