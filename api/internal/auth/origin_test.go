package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func originRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(OriginCheck([]string{"http://localhost:3000", "https://pos.moc.vn"}))
	ok := func(c *gin.Context) { c.Status(204) }
	r.GET("/x", ok)
	r.POST("/x", ok)
	r.PUT("/x", ok)
	r.DELETE("/x", ok)
	return r
}

func TestA15_OriginCheckUnit(t *testing.T) {
	r := originRouter()
	cases := []struct {
		method, origin string
		want           int
	}{
		{"GET", "", 204},
		{"GET", "https://evil.example", 204}, // reads are not checked
		{"POST", "http://localhost:3000", 204},
		{"PUT", "https://pos.moc.vn", 204},
		{"POST", "https://evil.example", 403},
		{"DELETE", "https://evil.example", 403},
		{"POST", "", 403},
		{"POST", "null", 403},
		{"POST", "http://localhost:3000.evil.example", 403},
		{"POST", "HTTPS://POS.MOC.VN", 204}, // scheme/host are case-insensitive
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, "/x", nil)
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("%s Origin=%q: status %d, want %d (%s)", tc.method, tc.origin, w.Code, tc.want, w.Body.String())
		}
		if tc.want == 403 && !strings.Contains(w.Body.String(), `"BAD_ORIGIN"`) {
			t.Errorf("%s Origin=%q: body %s, want BAD_ORIGIN", tc.method, tc.origin, w.Body.String())
		}
	}
}

// A-01: the cookie helper sets HttpOnly, SameSite=Lax and Secure when configured.
func TestA01_CookieFlags(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, secure := range []bool{true, false} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		SetCookie(c, SessionCookie, "abc", time.Hour, secure)
		res := http.Response{Header: w.Header()}
		cs := res.Cookies()
		if len(cs) != 1 {
			t.Fatalf("cookies = %v", cs)
		}
		ck := cs[0]
		if !ck.HttpOnly || ck.SameSite != http.SameSiteLaxMode || ck.Secure != secure || ck.Path != "/" {
			t.Errorf("secure=%v: cookie flags %+v", secure, ck)
		}
	}
}
