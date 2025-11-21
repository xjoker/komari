package logger

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var (
	// Global logger instance
	Logger zerolog.Logger
)

// Config holds logger configuration
type Config struct {
	// Level: trace, debug, info, warn, error, fatal, panic
	Level string
	// Format: json or console (human-readable)
	Format string
	// Output: stdout, stderr, or file path
	Output string
	// EnableCaller adds file:line info to logs
	EnableCaller bool
	// TimeFormat: unix, unixms, or rfc3339
	TimeFormat string
}

// DefaultConfig returns default logger configuration
func DefaultConfig() Config {
	return Config{
		Level:        getEnv("KOMARI_LOG_LEVEL", "info"),
		Format:       getEnv("KOMARI_LOG_FORMAT", "json"), // json for Loki compatibility
		Output:       getEnv("KOMARI_LOG_OUTPUT", "stdout"),
		EnableCaller: getEnvBool("KOMARI_LOG_CALLER", false),
		TimeFormat:   getEnv("KOMARI_LOG_TIME_FORMAT", "unix"), // unix timestamp for Loki
	}
}

// Init initializes the global logger with the given configuration
func Init(cfg Config) {
	// Set global log level
	level := parseLevel(cfg.Level)
	zerolog.SetGlobalLevel(level)

	// Configure time format
	switch cfg.TimeFormat {
	case "unix":
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	case "unixms":
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnixMs
	case "rfc3339":
		zerolog.TimeFieldFormat = time.RFC3339
	default:
		zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	}

	// Configure output writer
	var output io.Writer
	switch cfg.Output {
	case "stdout":
		output = os.Stdout
	case "stderr":
		output = os.Stderr
	default:
		// File output
		file, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			log.Fatal().Err(err).Str("path", cfg.Output).Msg("Failed to open log file")
		}
		output = file
	}

	// Configure format
	if cfg.Format == "console" {
		// Human-readable console output for development
		output = zerolog.ConsoleWriter{
			Out:        output,
			TimeFormat: time.RFC3339,
		}
	}

	// Create logger
	Logger = zerolog.New(output).With().Timestamp().Logger()

	// Enable caller info if requested
	if cfg.EnableCaller {
		Logger = Logger.With().Caller().Logger()
	}

	// Set as global logger
	log.Logger = Logger

	Logger.Info().
		Str("level", cfg.Level).
		Str("format", cfg.Format).
		Str("output", cfg.Output).
		Bool("caller", cfg.EnableCaller).
		Msg("Logger initialized")
}

// InitDefault initializes the logger with default configuration
func InitDefault() {
	Init(DefaultConfig())
}

// parseLevel converts string to zerolog.Level
func parseLevel(level string) zerolog.Level {
	switch strings.ToLower(level) {
	case "trace":
		return zerolog.TraceLevel
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn", "warning":
		return zerolog.WarnLevel
	case "error":
		return zerolog.ErrorLevel
	case "fatal":
		return zerolog.FatalLevel
	case "panic":
		return zerolog.PanicLevel
	default:
		return zerolog.InfoLevel
	}
}

// getEnv gets environment variable with fallback
func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

// getEnvBool gets boolean environment variable with fallback
func getEnvBool(key string, fallback bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1" || value == "yes"
	}
	return fallback
}

// Convenience functions for global logger

// Debug logs a debug message
func Debug() *zerolog.Event {
	return Logger.Debug()
}

// Info logs an info message
func Info() *zerolog.Event {
	return Logger.Info()
}

// Warn logs a warning message
func Warn() *zerolog.Event {
	return Logger.Warn()
}

// Error logs an error message
func Error() *zerolog.Event {
	return Logger.Error()
}

// Fatal logs a fatal message and exits
func Fatal() *zerolog.Event {
	return Logger.Fatal()
}

// Panic logs a panic message and panics
func Panic() *zerolog.Event {
	return Logger.Panic()
}

// WithContext creates a new logger with context fields
func WithContext() zerolog.Context {
	return Logger.With()
}

// Common structured fields for consistency
// These functions return a logger with contextual fields pre-populated

// WithClient returns a logger with client_uuid field
func WithClient(uuid string) zerolog.Logger {
	return Logger.With().Str("client_uuid", uuid).Logger()
}

// WithTask returns a logger with task_id field
func WithTask(id uint) zerolog.Logger {
	return Logger.With().Uint("task_id", id).Logger()
}

// WithSession returns a logger with session_id field
func WithSession(id string) zerolog.Logger {
	return Logger.With().Str("session_id", id).Logger()
}

// WithUser returns a logger with user_uuid field
func WithUser(uuid string) zerolog.Logger {
	return Logger.With().Str("user_uuid", uuid).Logger()
}

// WithComponent returns a logger with component field
func WithComponent(name string) zerolog.Logger {
	return Logger.With().Str("component", name).Logger()
}
