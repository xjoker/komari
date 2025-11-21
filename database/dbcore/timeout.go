package dbcore

import (
	"context"
	"time"

	"gorm.io/gorm"
)

const (
	// DefaultQueryTimeout is the default timeout for database queries
	// 5 seconds is reasonable for most queries with proper indexes
	DefaultQueryTimeout = 5 * time.Second

	// FastQueryTimeout for simple lookups (e.g., get by ID)
	FastQueryTimeout = 2 * time.Second

	// SlowQueryTimeout for complex aggregations or batch operations
	SlowQueryTimeout = 30 * time.Second
)

// WithTimeout returns a GORM DB instance with a context timeout
// This prevents slow queries from blocking indefinitely
//
// Usage:
//
//	db := dbcore.WithTimeout(dbcore.GetDBInstance(), dbcore.DefaultQueryTimeout)
//	db.Where("client = ?", uuid).Find(&records)
func WithTimeout(db *gorm.DB, timeout time.Duration) *gorm.DB {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	// Store cancel function in context for potential cleanup
	// In most cases, the context will timeout naturally
	_ = cancel

	return db.WithContext(ctx)
}

// WithDefaultTimeout returns a DB with the default 5-second timeout
func WithDefaultTimeout(db *gorm.DB) *gorm.DB {
	return WithTimeout(db, DefaultQueryTimeout)
}

// WithFastTimeout returns a DB with 2-second timeout for simple queries
func WithFastTimeout(db *gorm.DB) *gorm.DB {
	return WithTimeout(db, FastQueryTimeout)
}

// WithSlowTimeout returns a DB with 30-second timeout for complex operations
func WithSlowTimeout(db *gorm.DB) *gorm.DB {
	return WithTimeout(db, SlowQueryTimeout)
}

// WithCustomContext returns a DB with a custom context
// Useful when you need to propagate cancellation from HTTP request context
func WithCustomContext(db *gorm.DB, ctx context.Context) *gorm.DB {
	return db.WithContext(ctx)
}
