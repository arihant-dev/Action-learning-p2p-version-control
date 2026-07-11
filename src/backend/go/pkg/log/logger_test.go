package log

import (
	"testing"

	"github.com/rs/zerolog"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected zerolog.Level
	}{
		{"debug", zerolog.DebugLevel},
		{"info", zerolog.InfoLevel},
		{"warn", zerolog.WarnLevel},
		{"error", zerolog.ErrorLevel},
		{"fatal", zerolog.FatalLevel},
		{"panic", zerolog.PanicLevel},
		{"", zerolog.InfoLevel},
		{"invalid", zerolog.InfoLevel},
		{"  DEBUG  ", zerolog.DebugLevel},
		{"INFO", zerolog.InfoLevel},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseLogLevel(tt.input)
			if got != tt.expected {
				t.Errorf("parseLogLevel(%q) = %v, expected %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestNextCorrelationID(t *testing.T) {
	id1 := NextCorrelationID()
	id2 := NextCorrelationID()

	if id1 == "" {
		t.Error("expected non-empty correlation ID")
	}
	if id1 == id2 {
		t.Error("expected different correlation IDs")
	}
}

func TestSetGetLogLevel(t *testing.T) {
	original := LogLevel()
	defer SetLogLevel(original)

	SetLogLevel(zerolog.DebugLevel)
	if LogLevel() != zerolog.DebugLevel {
		t.Errorf("expected DebugLevel, got %v", LogLevel())
	}

	SetLogLevel(zerolog.ErrorLevel)
	if LogLevel() != zerolog.ErrorLevel {
		t.Errorf("expected ErrorLevel, got %v", LogLevel())
	}
}

func TestNewLogger(t *testing.T) {
	logger := NewLogger("test-component")
	if logger == nil {
		t.Fatal("expected non-nil logger")
	}
}
