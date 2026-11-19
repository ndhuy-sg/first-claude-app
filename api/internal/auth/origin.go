package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ndhuy-sg/first-claude-app/api/internal/apperr"
)

// OriginCheck rejects state-changing requests (anything but GET/HEAD/OPTIONS)
// whose Origin header is missing or not in allowed, with 403 BAD_ORIGIN (CSRF
// protection, test A-15). Comparison is exact on scheme://host[:port],
// case-insensitive.
func OriginCheck(allowed []string) gin.HandlerFunc {
	set := make(map[string]struct{}, len(allowed))
	for _, o := range allowed {
		set[normalizeOrigin(o)] = struct{}{}
	}
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			c.Next()
			return
		}
		origin := c.GetHeader("Origin")
		if _, ok := set[normalizeOrigin(origin)]; origin == "" || !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": apperr.ErrBadOrigin})
			return
		}
		c.Next()
	}
}

func normalizeOrigin(o string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(o)), "/")
}
