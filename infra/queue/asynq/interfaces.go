package asynq

import "sync/atomic"

// Logger decouples queue runtime logging from the host application.
type Logger interface {
	Log(msg string)
	LogSkip(skip int, msg string)
	Error(msg string)
	ErrorSkip(skip int, msg string)
}

var atomicLogger atomic.Value

type loggerHolder struct {
	logger Logger
}

// SetLogger sets the process-wide default logger used by new managers.
func SetLogger(logger Logger) {
	if logger != nil {
		atomicLogger.Store(&loggerHolder{logger: logger})
	}
}

// GetLogger returns the configured logger or a standard fallback.
func GetLogger() Logger {
	if value := atomicLogger.Load(); value != nil {
		if holder, ok := value.(*loggerHolder); ok && holder.logger != nil {
			return holder.logger
		}
	}
	return &StdLogger{}
}

// StdLogger is the zero-configuration logger.
type StdLogger struct{}

// Log writes an informational message.
func (l *StdLogger) Log(msg string) {
	println("[INFO]", msg)
}

// LogSkip writes an informational message; the standard logger ignores skip.
func (l *StdLogger) LogSkip(_ int, msg string) {
	println("[INFO]", msg)
}

// Error writes an error message.
func (l *StdLogger) Error(msg string) {
	println("[ERROR]", msg)
}

// ErrorSkip writes an error message; the standard logger ignores skip.
func (l *StdLogger) ErrorSkip(_ int, msg string) {
	println("[ERROR]", msg)
}
