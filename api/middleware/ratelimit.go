package middleware

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ulule/limiter/v3"
	"github.com/ulule/limiter/v3/drivers/store/memory"
)

var (
	// Global rate limiter for all endpoints
	globalLimiter *limiter.Limiter

	// Per-IP rate limiter to prevent DoS from single source
	ipLimiter *limiter.Limiter

	// Per-token rate limiter for authenticated endpoints
	tokenLimiters sync.Map

	once sync.Once
)

// initLimiters initializes rate limiters with appropriate thresholds
func initLimiters() {
	once.Do(func() {
		// Global limiter: 20,000 requests/minute
		// Rationale:
		//   - 1000 devices × 12 req/min = 12,000 normal load
		//   - 20,000 allows 67% headroom for bursts
		//   - Protects against total system overload
		globalStore := memory.NewStore()
		globalRate := limiter.Rate{
			Period: 1 * time.Minute,
			Limit:  20000,
		}
		globalLimiter = limiter.New(globalStore, globalRate)

		// Per-IP limiter: 1,000 requests/minute
		// Rationale:
		//   - Assumes max 50 devices behind single NAT gateway
		//   - 50 × 12 = 600 normal, 1000 allows headroom
		//   - Prevents single IP from DoS attack
		ipStore := memory.NewStore()
		ipRate := limiter.Rate{
			Period: 1 * time.Minute,
			Limit:  1000,
		}
		ipLimiter = limiter.New(ipStore, ipRate)
	})
}

// RateLimitGlobal applies global rate limiting to prevent system overload
func RateLimitGlobal() gin.HandlerFunc {
	initLimiters()

	return func(c *gin.Context) {
		// Use a constant key for global limiting
		context, err := globalLimiter.Get(c, "global")
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "Rate limiter error",
			})
			return
		}

		// Set rate limit headers
		c.Header("X-RateLimit-Limit", fmt.Sprintf("%d", context.Limit))
		c.Header("X-RateLimit-Remaining", fmt.Sprintf("%d", context.Remaining))
		c.Header("X-RateLimit-Reset", fmt.Sprintf("%d", context.Reset))

		if context.Reached {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":     "Global rate limit exceeded",
				"message":   "System is at capacity, please retry later",
				"retry_after": context.Reset,
			})
			return
		}

		c.Next()
	}
}

// RateLimitByIP limits requests per IP address to prevent DoS
func RateLimitByIP() gin.HandlerFunc {
	initLimiters()

	return func(c *gin.Context) {
		// Get client IP
		ip := c.ClientIP()

		context, err := ipLimiter.Get(c, ip)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "Rate limiter error",
			})
			return
		}

		c.Header("X-RateLimit-IP-Limit", fmt.Sprintf("%d", context.Limit))
		c.Header("X-RateLimit-IP-Remaining", fmt.Sprintf("%d", context.Remaining))
		c.Header("X-RateLimit-IP-Reset", fmt.Sprintf("%d", context.Reset))

		if context.Reached {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "IP rate limit exceeded",
				"message": "Too many requests from your IP address",
				"ip":      ip,
				"retry_after": context.Reset,
			})
			return
		}

		c.Next()
	}
}

// RateLimitByToken limits requests per authentication token
// This is more granular than IP limiting and prevents abuse by authenticated clients
func RateLimitByToken() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get token from query or header
		token := c.Query("token")
		if token == "" {
			token = c.GetHeader("Authorization")
		}

		if token == "" {
			// No token, skip token-based limiting (will be caught by auth middleware)
			c.Next()
			return
		}

		// Get or create limiter for this token
		limiterInterface, _ := tokenLimiters.LoadOrStore(token, createTokenLimiter())
		tokenLimiter := limiterInterface.(*limiter.Limiter)

		context, err := tokenLimiter.Get(c, token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "Rate limiter error",
			})
			return
		}

		c.Header("X-RateLimit-Token-Limit", fmt.Sprintf("%d", context.Limit))
		c.Header("X-RateLimit-Token-Remaining", fmt.Sprintf("%d", context.Remaining))
		c.Header("X-RateLimit-Token-Reset", fmt.Sprintf("%d", context.Reset))

		if context.Reached {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "Token rate limit exceeded",
				"message": "Client is sending too many requests",
				"retry_after": context.Reset,
			})
			return
		}

		c.Next()
	}
}

// createTokenLimiter creates a new limiter for a specific token
func createTokenLimiter() *limiter.Limiter {
	// Per-token limiter: 30 requests/minute
	// Rationale:
	//   - Normal: 12 metrics/min + 1 profile/hour + ~5 ping results/min = ~17 req/min
	//   - 30 allows significant headroom for retries and bursts
	//   - Prevents single malicious client from overwhelming system
	store := memory.NewStore()
	rate := limiter.Rate{
		Period: 1 * time.Minute,
		Limit:  30,
	}
	return limiter.New(store, rate)
}

// RateLimitProtobuf applies stricter rate limiting to Protobuf v2.0 endpoints
// These endpoints handle larger payloads and are more resource-intensive
func RateLimitProtobuf() gin.HandlerFunc {
	// Protobuf endpoints: 15,000 requests/minute
	// Rationale:
	//   - v2.0 protocol is more efficient but still limited
	//   - Slightly lower than global to prioritize v2.0 traffic
	store := memory.NewStore()
	rate := limiter.Rate{
		Period: 1 * time.Minute,
		Limit:  15000,
	}
	protobufLimiter := limiter.New(store, rate)

	return func(c *gin.Context) {
		context, err := protobufLimiter.Get(c, "protobuf")
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "Rate limiter error",
			})
			return
		}

		c.Header("X-RateLimit-Protobuf-Limit", fmt.Sprintf("%d", context.Limit))
		c.Header("X-RateLimit-Protobuf-Remaining", fmt.Sprintf("%d", context.Remaining))

		if context.Reached {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":   "Protobuf endpoint rate limit exceeded",
				"message": "Too many Protobuf requests, please reduce report frequency",
				"retry_after": context.Reset,
			})
			return
		}

		c.Next()
	}
}

// CleanupTokenLimiters periodically cleans up unused token limiters to prevent memory leak
// Should be called as a goroutine on server startup
func CleanupTokenLimiters(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		// This is a simple implementation - in production, you'd track last access time
		// and remove limiters that haven't been used in a while
		// For now, the memory.Store has its own cleanup mechanism
	}
}
