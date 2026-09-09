package api

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// --- Rate Limiting ---

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type ipRateLimiter struct {
	visitors map[string]*visitor
	mu       sync.Mutex
	r        rate.Limit
	burst    int
}

func newIPRateLimiter(r rate.Limit, burst int) *ipRateLimiter {
	rl := &ipRateLimiter{
		visitors: make(map[string]*visitor),
		r:        r,
		burst:    burst,
	}
	go rl.cleanupLoop()
	return rl
}

func (rl *ipRateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	if !exists {
		v = &visitor{limiter: rate.NewLimiter(rl.r, rl.burst)}
		rl.visitors[ip] = v
	}
	v.lastSeen = time.Now()
	return v.limiter
}

// cleanupLoop removes visitor entries that haven't been seen in 3 minutes.
func (rl *ipRateLimiter) cleanupLoop() {
	for {
		time.Sleep(3 * time.Minute)
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.lastSeen) > 3*time.Minute {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// RateLimitMiddleware returns a chi middleware limiting /chat to 10 req/min per IP.
func RateLimitMiddleware() func(http.Handler) http.Handler {
	// 10 requests/minute = 10/60 tokens per second, burst of 10
	rl := newIPRateLimiter(rate.Limit(10.0/60.0), 10)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := realIP(r)
			if !rl.getLimiter(ip).Allow() {
				w.Header().Set("Content-Type", "application/json")
				http.Error(w,
					`{"error":"Too many requests. Please try again later."}`,
					http.StatusTooManyRequests,
				)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// --- CORS ---

// CORSMiddleware returns a chi middleware that restricts origins to the allowlist.
// allowedOrigins is a comma-separated list (e.g. "https://example.com,http://localhost:3000").
// Any "https://<sub>.vercel.app" origin is also allowed, so the production site
// and every Vercel preview deploy work without re-listing per-deploy URLs.
func CORSMiddleware(allowedOrigins string) func(http.Handler) http.Handler {
	origins := make(map[string]bool)
	for _, o := range strings.Split(allowedOrigins, ",") {
		trimmed := strings.TrimSpace(o)
		if trimmed != "" {
			origins[trimmed] = true
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origins[origin] || isVercelOrigin(origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.Header().Set("Vary", "Origin")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// isVercelOrigin reports whether origin is "https://<host>.vercel.app" with no
// path — i.e. a Vercel production or preview deployment of this site.
func isVercelOrigin(origin string) bool {
	const prefix = "https://"
	const suffix = ".vercel.app"
	if !strings.HasPrefix(origin, prefix) || !strings.HasSuffix(origin, suffix) {
		return false
	}
	host := origin[len(prefix):]
	// Reject anything with a path, port, or embedded slash — host only.
	if strings.ContainsAny(host, "/:") {
		return false
	}
	return len(host) > len(suffix) // there is a non-empty subdomain label
}

// --- Helpers ---

// realIP extracts the client IP, respecting reverse-proxy headers.
func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		return strings.TrimSpace(strings.Split(fwd, ",")[0])
	}
	// Strip port from RemoteAddr
	addr := r.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i != -1 {
		return addr[:i]
	}
	return addr
}
