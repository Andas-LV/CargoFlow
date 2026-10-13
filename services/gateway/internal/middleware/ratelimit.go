package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc/codes"
)

// RateLimit allows at most limit requests per client IP per window (fixed window counter
// in Redis, shared by all gateway replicas). If Redis is unavailable the request is let
// through: availability matters more than strict limiting here.
func RateLimit(rdb *redis.Client, limit int, window time.Duration, log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			bucket := time.Now().UnixNano() / int64(window)
			key := "rl:" + clientIP(r) + ":" + strconv.FormatInt(bucket, 10)

			pipe := rdb.TxPipeline()
			incr := pipe.Incr(ctx, key)
			pipe.Expire(ctx, key, window)
			if _, err := pipe.Exec(ctx); err != nil {
				log.WarnContext(ctx, "rate limit: redis error, allowing request", "err", err)
				next.ServeHTTP(w, r)
				return
			}

			count := incr.Val()
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(max(int64(limit)-count, 0), 10))
			if count > int64(limit) {
				w.Header().Set("Retry-After", strconv.Itoa(int(window.Seconds())))
				writeError(w, http.StatusTooManyRequests, codes.ResourceExhausted, "rate limit exceeded")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// clientIP prefers the first X-Forwarded-For entry (set by Render's proxy) over RemoteAddr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ip, _, _ := strings.Cut(xff, ",")
		return strings.TrimSpace(ip)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
