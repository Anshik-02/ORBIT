package observability

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// LogLevel indicates severity of a structured log record.
type LogLevel string

const (
	LogLevelDebug LogLevel = "DEBUG"
	LogLevelInfo  LogLevel = "INFO"
	LogLevelWarn  LogLevel = "WARN"
	LogLevelError LogLevel = "ERROR"
)

// LogRecord represents a single structured JSON log message.
type LogRecord struct {
	Timestamp     time.Time      `json:"timestamp"`
	Level         LogLevel       `json:"level"`
	CorrelationID string         `json:"correlation_id,omitempty"`
	Subsystem     string         `json:"subsystem"`
	Message       string         `json:"message"`
	Fields        map[string]any `json:"fields,omitempty"`
}

// StructuredLogger writes formatted JSON log records safely across concurrent goroutines.
type StructuredLogger struct {
	mu        sync.Mutex
	out       io.Writer
	subsystem string
	minLevel  LogLevel
}

// NewStructuredLogger creates a logger with an output destination and subsystem tag.
func NewStructuredLogger(out io.Writer, subsystem string) *StructuredLogger {
	if out == nil {
		out = os.Stdout
	}
	return &StructuredLogger{
		out:       out,
		subsystem: subsystem,
		minLevel:  LogLevelDebug,
	}
}

// WithCorrelation returns a scoped logger bound to correlationID.
func (l *StructuredLogger) Log(level LogLevel, correlationID, msg string, fields map[string]any) {
	record := LogRecord{
		Timestamp:     time.Now().UTC(),
		Level:         level,
		CorrelationID: correlationID,
		Subsystem:     l.subsystem,
		Message:       msg,
		Fields:        fields,
	}

	data, err := json.Marshal(record)
	if err != nil {
		data = []byte(fmt.Sprintf(`{"level":"ERROR","message":%q}`, err.Error()))
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	_, _ = l.out.Write(append(data, '\n'))
}

func (l *StructuredLogger) Info(correlationID, msg string, fields map[string]any) {
	l.Log(LogLevelInfo, correlationID, msg, fields)
}

func (l *StructuredLogger) Warn(correlationID, msg string, fields map[string]any) {
	l.Log(LogLevelWarn, correlationID, msg, fields)
}

func (l *StructuredLogger) Error(correlationID, msg string, fields map[string]any) {
	l.Log(LogLevelError, correlationID, msg, fields)
}

func (l *StructuredLogger) Debug(correlationID, msg string, fields map[string]any) {
	l.Log(LogLevelDebug, correlationID, msg, fields)
}
