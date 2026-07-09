package log

import (
	"os"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/rs/zerolog"
)

var globalLevel zerolog.Level

func init() {
	globalLevel = parseLogLevel(os.Getenv("P2P_LOG_LEVEL"))
	zerolog.SetGlobalLevel(globalLevel)
}

func parseLogLevel(level string) zerolog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return zerolog.DebugLevel
	case "info":
		return zerolog.InfoLevel
	case "warn":
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

var correlationCounter uint64

func NextCorrelationID() string {
	n := atomic.AddUint64(&correlationCounter, 1)
	return strconv.FormatUint(n, 36)
}

func NewLogger(component string) *zerolog.Logger {
	l := zerolog.New(os.Stderr).
		With().
		Timestamp().
		Str("component", component).
		Caller().
		Logger()
	return &l
}

func SetLogLevel(l zerolog.Level) {
	globalLevel = l
	zerolog.SetGlobalLevel(l)
}

func LogLevel() zerolog.Level {
	return globalLevel
}