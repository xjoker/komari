# Structured Logging

Komari uses [zerolog](https://github.com/rs/zerolog) for high-performance structured logging with Loki-compatible JSON output.

## Features

- **Zero Allocation**: Optimized for minimal GC pressure
- **JSON Native**: Perfect for Loki, ELK, and other log aggregation systems
- **Context Fields**: Rich structured data (client_uuid, task_id, component, etc.)
- **Multiple Levels**: trace, debug, info, warn, error, fatal, panic
- **Environment Configured**: All settings via environment variables

## Configuration

All logging is controlled via environment variables:

```bash
# Log level (trace, debug, info, warn, error, fatal, panic)
export KOMARI_LOG_LEVEL=info

# Output format: json (for Loki) or console (for development)
export KOMARI_LOG_FORMAT=json

# Output destination: stdout, stderr, or file path
export KOMARI_LOG_OUTPUT=stdout

# Enable caller info (file:line) in logs
export KOMARI_LOG_CALLER=false

# Time format: unix (default), unixms, or rfc3339
export KOMARI_LOG_TIME_FORMAT=unix
```

### Recommended Configurations

**Production (with Loki):**
```bash
KOMARI_LOG_LEVEL=info
KOMARI_LOG_FORMAT=json
KOMARI_LOG_OUTPUT=stdout
KOMARI_LOG_TIME_FORMAT=unix
```

**Development:**
```bash
KOMARI_LOG_LEVEL=debug
KOMARI_LOG_FORMAT=console
KOMARI_LOG_OUTPUT=stdout
KOMARI_LOG_CALLER=true
KOMARI_LOG_TIME_FORMAT=rfc3339
```

**File Output:**
```bash
KOMARI_LOG_OUTPUT=/var/log/komari/komari.log
```

## Usage Examples

### Basic Logging

```go
import "github.com/komari-monitor/komari/utils/logger"

// Simple info log
logger.Info().Msg("Server started")

// With error
logger.Error().Err(err).Msg("Failed to connect to database")

// With fields
logger.Info().
    Str("address", "0.0.0.0:25774").
    Int("port", 25774).
    Msg("Listening on address")

// Warning
logger.Warn().
    Str("client_uuid", uuid).
    Msg("Client connection timeout")
```

### Structured Context Fields

```go
// Client operations
logger.WithClient(uuid).Info().
    Int64("ram_used", ramUsed).
    Float64("cpu_usage", cpuUsage).
    Msg("Metrics received")

// Task operations
logger.WithTask(taskId).Info().
    Str("target", "8.8.8.8").
    Dur("latency", latency).
    Msg("Ping completed")

// Component logging
logger.WithComponent("database").Error().
    Err(err).
    Str("query", "SELECT * FROM clients").
    Msg("Query timeout")

// Session operations
logger.WithSession(sessionId).Info().
    Str("ip", clientIP).
    Str("user_agent", userAgent).
    Msg("User logged in")
```

### Log Levels

```go
// Trace - Very detailed debugging (development only)
logger.Debug().Msg("Entering function")

// Debug - Detailed debugging information
logger.Debug().Interface("config", cfg).Msg("Configuration loaded")

// Info - General informational messages (default)
logger.Info().Msg("Server started successfully")

// Warn - Warning messages (degraded state but still functional)
logger.Warn().Msg("Database connection pool nearly exhausted")

// Error - Error messages (operation failed, needs attention)
logger.Error().Err(err).Msg("Failed to save metrics")

// Fatal - Critical errors that cause application exit
logger.Fatal().Err(err).Msg("Cannot connect to database")

// Panic - Severe errors that cause panic
logger.Panic().Err(err).Msg("Critical system failure")
```

### Complex Structured Logs

```go
logger.Info().
    Str("component", "rate-limiter").
    Str("client_uuid", uuid).
    Str("ip", clientIP).
    Int("limit", 1000).
    Int("remaining", 45).
    Int64("reset_at", resetTime).
    Bool("rate_limited", false).
    Msg("Rate limit check passed")
```

## JSON Output Format

All logs are output as JSON when `KOMARI_LOG_FORMAT=json` (default):

```json
{
  "level": "info",
  "time": 1700000000,
  "message": "Metrics received",
  "client_uuid": "550e8400-e29b-41d4-a716-446655440000",
  "ram_used": 4294967296,
  "cpu_usage": 45.2
}
```

```json
{
  "level": "error",
  "time": 1700000001,
  "error": "connection timeout",
  "component": "database",
  "query": "SELECT * FROM clients",
  "message": "Query timeout"
}
```

## Loki Integration

### Promtail Configuration

```yaml
server:
  http_listen_port: 9080
  grpc_listen_port: 0

positions:
  filename: /tmp/positions.yaml

clients:
  - url: http://loki:3100/loki/api/v1/push

scrape_configs:
  - job_name: komari
    static_configs:
      - targets:
          - localhost
        labels:
          job: komari
          __path__: /var/log/komari/*.log
    pipeline_stages:
      - json:
          expressions:
            level: level
            component: component
            client_uuid: client_uuid
            task_id: task_id
      - labels:
          level:
          component:
      - timestamp:
          source: time
          format: Unix
```

### LogQL Query Examples

```logql
# All error logs
{job="komari"} | json | level="error"

# Logs for specific client
{job="komari"} | json | client_uuid="550e8400-e29b-41d4-a716-446655440000"

# Database component errors
{job="komari",component="database"} | json | level="error"

# Rate limiting events
{job="komari"} | json | component="rate-limiter"

# High CPU usage alerts
{job="komari"} | json | cpu_usage > 80
```

## Migration from log.Printf

**Before:**
```go
log.Printf("Client %s connected from %s", uuid, ip)
log.Printf("Error saving metrics: %v", err)
```

**After:**
```go
logger.Info().
    Str("client_uuid", uuid).
    Str("ip", ip).
    Msg("Client connected")

logger.Error().
    Err(err).
    Str("client_uuid", uuid).
    Msg("Failed to save metrics")
```

## Performance

Zerolog is designed for zero heap allocations:

```
BenchmarkLogEmpty-8            100000000    11.7 ns/op     0 B/op    0 allocs/op
BenchmarkLogInfo-8              30000000    42.6 ns/op     0 B/op    0 allocs/op
BenchmarkContextFields-8        30000000    44.9 ns/op     0 B/op    0 allocs/op
```

This makes it suitable for high-frequency logging in monitoring applications.

## Best Practices

1. **Always use structured fields** instead of string formatting
2. **Use appropriate log levels** (don't log everything as Info)
3. **Add context fields** (component, client_uuid, task_id) for filtering
4. **Keep messages concise** - details go in fields
5. **Use JSON format in production** for Loki compatibility
6. **Avoid logging sensitive data** (passwords, tokens)

## Common Patterns

### HTTP Request Logging
```go
logger.Info().
    Str("method", "POST").
    Str("path", "/api/v2/metrics").
    Str("client_uuid", uuid).
    Str("ip", c.ClientIP()).
    Int("status", 200).
    Dur("latency", elapsed).
    Msg("HTTP request")
```

### Database Query Logging
```go
logger.Debug().
    Str("component", "database").
    Str("table", "records").
    Str("operation", "SELECT").
    Dur("duration", elapsed).
    Int("rows", count).
    Msg("Database query completed")
```

### Error with Stack Context
```go
logger.Error().
    Err(err).
    Str("component", "rate-limiter").
    Str("function", "RateLimitByToken").
    Str("token_hash", hash).
    Msg("Rate limiter initialization failed")
```
