package logger

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("The handle is invalid.")
}

// A GUI-subsystem process has no stdout. The log file must still get every line.
func TestTeeWriterKeepsWritingToTheFileWhenTheConsoleIsGone(t *testing.T) {
	var file bytes.Buffer
	log := slog.New(slog.NewJSONHandler(newTeeWriter(&file, failingWriter{}), nil))

	log.Info("Agent is up to date", "service", "updater")
	log.Info("second line")

	got := file.String()
	if strings.Count(got, "\n") != 2 || !strings.Contains(got, "Agent is up to date") {
		t.Fatalf("log file must receive both lines even though the console write fails, got %q", got)
	}
}

func TestTeeWriterStillEchoesToAWorkingConsole(t *testing.T) {
	var file, console bytes.Buffer

	if _, err := newTeeWriter(&file, &console).Write([]byte("line\n")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if file.String() != "line\n" || console.String() != "line\n" {
		t.Fatalf("both outputs must get the line, file=%q console=%q", file.String(), console.String())
	}
}
