package logx_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"gscacco.com/cronx/internal/logx"
)

func TestNewLoggerHonoursTheConfiguredLevel(t *testing.T) {
	tests := []struct {
		name       string
		level      string
		emit       func(*slog.Logger)
		wantLogged bool
	}{
		{"a debug record is shown at debug level", "debug", func(l *slog.Logger) { l.Debug("detail") }, true},
		{"a debug record is hidden at info level", "info", func(l *slog.Logger) { l.Debug("detail") }, false},
		{"an info record is hidden at warn level", "warn", func(l *slog.Logger) { l.Info("routine") }, false},
		{"an error record is always shown", "error", func(l *slog.Logger) { l.Error("trouble") }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// SETUP
			var output bytes.Buffer
			logger, err := logx.NewLogger(&output, tt.level)
			if err != nil {
				t.Fatalf("NewLogger(%q) returned an unexpected error: %v", tt.level, err)
			}

			// EXERCISE
			tt.emit(logger)

			// VERIFY
			got := output.String()
			if tt.wantLogged && got == "" {
				t.Errorf("NewLogger(%q) produced no output, want a record", tt.level)
			}
			if !tt.wantLogged && got != "" {
				t.Errorf("NewLogger(%q) output = %q, want nothing", tt.level, got)
			}
		})
	}
}

func TestNewLoggerRecordsTheMessagesItAccepts(t *testing.T) {
	// SETUP
	var output bytes.Buffer
	logger, err := logx.NewLogger(&output, "info")
	if err != nil {
		t.Fatalf("NewLogger() returned an unexpected error: %v", err)
	}

	// EXERCISE
	logger.Info("starting the scheduler", "jobs", 2)

	// VERIFY
	got := output.String()
	if !strings.Contains(got, "starting the scheduler") {
		t.Errorf("logger output = %q, want it to contain the message", got)
	}
	if !strings.Contains(got, "jobs=2") {
		t.Errorf("logger output = %q, want it to contain the attribute", got)
	}
}

func TestNewLoggerRejectsAnUnknownLevel(t *testing.T) {
	// SETUP
	var output bytes.Buffer

	// EXERCISE
	_, err := logx.NewLogger(&output, "loud")

	// VERIFY
	if err == nil {
		t.Fatalf("NewLogger() succeeded, want an error for an unknown level")
	}
	if !strings.Contains(err.Error(), "loud") {
		t.Errorf("NewLogger() error = %q, want it to mention the level", err.Error())
	}
}
