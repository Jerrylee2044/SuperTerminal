// Package logger provides unified logging for SuperTerminal.
package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Level represents log level.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// String returns the level name.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Field represents a log field.
type Field struct {
	Key   string
	Value interface{}
}

// F creates a log field.
func F(key string, value interface{}) Field {
	return Field{Key: key, Value: value}
}

// Logger is the main logger.
type Logger struct {
	name       string
	level      Level
	output     io.Writer
	fileOutput io.Writer
	mu         sync.Mutex
	fields     []Field
	auditLog   *AuditLog
}

// AuditLog records security-sensitive operations.
type AuditLog struct {
	file   *os.File
	mu     sync.Mutex
	path   string
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// Init initializes the default logger.
func Init(opts Options) *Logger {
	once.Do(func() {
		defaultLogger = NewLogger(opts)
	})
	return defaultLogger
}

// Get returns the default logger.
func Get() *Logger {
	if defaultLogger == nil {
		defaultLogger = NewLogger(Options{Level: LevelInfo})
	}
	return defaultLogger
}

// Options configures the logger.
type Options struct {
	Name      string
	Level     Level
	Output    io.Writer
	File      string // Log file path
	AuditFile string // Audit log file path
}

// NewLogger creates a new logger.
func NewLogger(opts Options) *Logger {
	if opts.Output == nil {
		opts.Output = os.Stdout
	}
	if opts.Name == "" {
		opts.Name = "superterminal"
	}

	l := &Logger{
		name:   opts.Name,
		level:  opts.Level,
		output: opts.Output,
		fields: []Field{},
	}

	// Setup file output
	if opts.File != "" {
		if err := l.setupFileOutput(opts.File); err != nil {
			log.Printf("Failed to setup log file: %v", err)
		}
	}

	// Setup audit log
	if opts.AuditFile != "" {
		if err := l.setupAuditLog(opts.AuditFile); err != nil {
			log.Printf("Failed to setup audit log: %v", err)
		}
	}

	return l
}

// setupFileOutput creates log file.
func (l *Logger) setupFileOutput(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	l.fileOutput = file
	return nil
}

// setupAuditLog creates audit log file.
func (l *Logger) setupAuditLog(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}

	l.auditLog = &AuditLog{
		file: file,
		path: path,
	}
	return nil
}

// WithFields returns a logger with additional fields.
func (l *Logger) WithFields(fields ...Field) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()

	newLogger := &Logger{
		name:       l.name,
		level:      l.level,
		output:     l.output,
		fileOutput: l.fileOutput,
		auditLog:   l.auditLog,
		fields:     append(l.fields, fields...),
	}
	return newLogger
}

// WithName returns a logger with a new name.
func (l *Logger) WithName(name string) *Logger {
	l.mu.Lock()
	defer l.mu.Unlock()

	newLogger := &Logger{
		name:       name,
		level:      l.level,
		output:     l.output,
		fileOutput: l.fileOutput,
		auditLog:   l.auditLog,
		fields:     l.fields,
	}
	return newLogger
}

// SetLevel changes the log level.
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// log writes a log message.
func (l *Logger) log(level Level, msg string, fields ...Field) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Format timestamp
	ts := time.Now().Format("2006-01-02 15:04:05.000")

	// Format fields
	allFields := append(l.fields, fields...)
	fieldStr := ""
	for _, f := range allFields {
		fieldStr += fmt.Sprintf(" %s=%v", f.Key, f.Value)
	}

	// Build log line
	line := fmt.Sprintf("[%s] [%s] [%s] %s%s\n", ts, level, l.name, msg, fieldStr)

	// Write to outputs
	l.output.Write([]byte(line))
	if l.fileOutput != nil {
		l.fileOutput.Write([]byte(line))
	}
}

// Debug logs a debug message.
func (l *Logger) Debug(msg string, fields ...Field) {
	l.log(LevelDebug, msg, fields...)
}

// Debugf logs a formatted debug message.
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.log(LevelDebug, fmt.Sprintf(format, args...))
}

// Info logs an info message.
func (l *Logger) Info(msg string, fields ...Field) {
	l.log(LevelInfo, msg, fields...)
}

// Infof logs a formatted info message.
func (l *Logger) Infof(format string, args ...interface{}) {
	l.log(LevelInfo, fmt.Sprintf(format, args...))
}

// Warn logs a warning message.
func (l *Logger) Warn(msg string, fields ...Field) {
	l.log(LevelWarn, msg, fields...)
}

// Warnf logs a formatted warning message.
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.log(LevelWarn, fmt.Sprintf(format, args...))
}

// Error logs an error message.
func (l *Logger) Error(msg string, fields ...Field) {
	l.log(LevelError, msg, fields...)
}

// Errorf logs a formatted error message.
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.log(LevelError, fmt.Sprintf(format, args...))
}

// Fatal logs a fatal message and exits.
func (l *Logger) Fatal(msg string, fields ...Field) {
	l.log(LevelFatal, msg, fields...)
	os.Exit(1)
}

// Fatalf logs a formatted fatal message and exits.
func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.log(LevelFatal, fmt.Sprintf(format, args...))
	os.Exit(1)
}

// Audit records a security-sensitive operation.
func (l *Logger) Audit(action, actor, target string, result string, fields ...Field) {
	if l.auditLog == nil {
		return
	}

	l.auditLog.mu.Lock()
	defer l.auditLog.mu.Unlock()

	ts := time.Now().Format("2006-01-02 15:04:05.000")
	fieldStr := ""
	for _, f := range fields {
		fieldStr += fmt.Sprintf(" %s=%v", f.Key, f.Value)
	}

	line := fmt.Sprintf("[%s] action=%s actor=%s target=%s result=%s%s\n",
		ts, action, actor, target, result, fieldStr)

	l.auditLog.file.Write([]byte(line))
}

// AuditToolCall records a tool execution.
func (l *Logger) AuditToolCall(toolName, input string, success bool, duration int64) {
	result := "success"
	if !success {
		result = "failed"
	}
	l.Audit("tool_call", "engine", toolName, result,
		F("input_size", len(input)),
		F("duration_ms", duration))
}

// AuditAPIRequest records an API request.
func (l *Logger) AuditAPIRequest(model string, inputTokens int, success bool, cost float64) {
	result := "success"
	if !success {
		result = "failed"
	}
	l.Audit("api_request", "engine", model, result,
		F("input_tokens", inputTokens),
		F("cost", cost))
}

// Close closes all log files.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	var err error
	if l.fileOutput != nil {
		if closer, ok := l.fileOutput.(io.Closer); ok {
			err = closer.Close()
		}
	}
	if l.auditLog != nil && l.auditLog.file != nil {
		l.auditLog.file.Close()
	}
	return err
}

// === Global functions ===

// Debug logs to default logger.
func Debug(msg string, fields ...Field) {
	Get().Debug(msg, fields...)
}

// Debugf logs formatted to default logger.
func Debugf(format string, args ...interface{}) {
	Get().Debugf(format, args...)
}

// Info logs to default logger.
func Info(msg string, fields ...Field) {
	Get().Info(msg, fields...)
}

// Infof logs formatted to default logger.
func Infof(format string, args ...interface{}) {
	Get().Infof(format, args...)
}

// Warn logs to default logger.
func Warn(msg string, fields ...Field) {
	Get().Warn(msg, fields...)
}

// Warnf logs formatted to default logger.
func Warnf(format string, args ...interface{}) {
	Get().Warnf(format, args...)
}

// Error logs to default logger.
func Error(msg string, fields ...Field) {
	Get().Error(msg, fields...)
}

// Errorf logs formatted to default logger.
func Errorf(format string, args ...interface{}) {
	Get().Errorf(format, args...)
}

// Fatal logs to default logger and exits.
func Fatal(msg string, fields ...Field) {
	Get().Fatal(msg, fields...)
}

// Fatalf logs formatted to default logger and exits.
func Fatalf(format string, args ...interface{}) {
	Get().Fatalf(format, args...)
}

// Audit records to default logger audit.
func Audit(action, actor, target, result string, fields ...Field) {
	Get().Audit(action, actor, target, result, fields...)
}

// AuditToolCall records tool execution.
func AuditToolCall(toolName, input string, success bool, duration int64) {
	Get().AuditToolCall(toolName, input, success, duration)
}

// AuditAPIRequest records API request.
func AuditAPIRequest(model string, inputTokens int, success bool, cost float64) {
	Get().AuditAPIRequest(model, inputTokens, success, cost)
}

// SetLevel changes default logger level.
func SetLevel(level Level) {
	Get().SetLevel(level)
}

// Close closes default logger.
func Close() error {
	return Get().Close()
}