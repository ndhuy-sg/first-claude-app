package auth

import (
	"bytes"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Idempotency replays the stored response when a write request is retried with
// the same Idempotency-Key header by the same user (test P-08). Requests
// without the header pass through. Only 2xx responses are stored.
func Idempotency(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader("Idempotency-Key")
		p := From(c)
		if key == "" || p == nil || c.Request.Method == http.MethodGet {
			c.Next()
			return
		}
		ctx := c.Request.Context()
		var status int
		var body []byte
		err := pool.QueryRow(ctx,
			`SELECT response_status, response_body FROM idempotency_keys WHERE user_id = $1 AND key = $2`,
			p.UserID, key).Scan(&status, &body)
		if err == nil {
			c.Header("Idempotent-Replay", "true")
			c.Data(status, "application/json; charset=utf-8", body)
			c.Abort()
			return
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			_ = c.Error(err)
		}
		rec := &recorder{ResponseWriter: c.Writer}
		c.Writer = rec
		c.Next()
		if s := rec.Status(); s >= 200 && s < 300 {
			_, _ = pool.Exec(ctx,
				`INSERT INTO idempotency_keys (user_id, key, method, path, response_status, response_body)
				 VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT DO NOTHING`,
				p.UserID, key, c.Request.Method, c.Request.URL.Path, s, rec.buf.Bytes())
		}
	}
}

type recorder struct {
	gin.ResponseWriter
	buf bytes.Buffer
}

func (r *recorder) Write(b []byte) (int, error) {
	r.buf.Write(b)
	return r.ResponseWriter.Write(b)
}

func (r *recorder) WriteString(s string) (int, error) {
	r.buf.WriteString(s)
	return r.ResponseWriter.WriteString(s)
}
