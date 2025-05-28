package logger

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// ContextKey is the type for context keys
type ContextKey string

const (
	// CorrelationIDKey is the context key for correlation ID
	CorrelationIDKey ContextKey = "correlationId"
)

// Logger wraps zap.Logger with additional functionality
type Logger struct {
	*zap.Logger
	serviceName string
}

// Config represents logger configuration
type Config struct {
	Level       string // debug, info, warn, error
	Format      string // json, console
	Output      string // stdout, stderr, file
	ServiceName string
}

// New creates a new logger instance
func New(config Config) (*Logger, error) {
	// Parse log level
	level, err := zapcore.ParseLevel(config.Level)
	if err != nil {
		return nil, fmt.Errorf("invalid log level %s: %w", config.Level, err)
	}

	// Create encoder config with required fields
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// Create encoder based on format
	var encoder zapcore.Encoder
	if config.Format == "console" {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	}

	// Create core
	core := zapcore.NewCore(encoder, zapcore.AddSync(getWriter(config.Output)), level)

	// Create logger with caller information
	zapLogger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	// Add service name as a permanent field
	zapLogger = zapLogger.With(zap.String("service", config.ServiceName))

	return &Logger{
		Logger:      zapLogger,
		serviceName: config.ServiceName,
	}, nil
}

// getWriter returns the appropriate writer based on output configuration
func getWriter(output string) zapcore.WriteSyncer {
	switch output {
	case "stderr":
		return zapcore.Lock(zapcore.AddSync(os.Stderr))
	case "file":
		// For now, default to stdout. File output would require additional configuration
		fallthrough
	default:
		return zapcore.Lock(zapcore.AddSync(os.Stdout))
	}
}

// WithCorrelationID adds correlation ID to the logger
func (l *Logger) WithCorrelationID(correlationID string) *Logger {
	return &Logger{
		Logger:      l.Logger.With(zap.String("correlationId", correlationID)),
		serviceName: l.serviceName,
	}
}

// WithContext extracts correlation ID from context and adds it to the logger
func (l *Logger) WithContext(ctx context.Context) *Logger {
	if correlationID := GetCorrelationID(ctx); correlationID != "" {
		return l.WithCorrelationID(correlationID)
	}
	return l
}

// WithFields adds additional fields to the logger
func (l *Logger) WithFields(fields ...zap.Field) *Logger {
	return &Logger{
		Logger:      l.Logger.With(fields...),
		serviceName: l.serviceName,
	}
}

// LogKafkaMessage logs a Kafka message with standard fields
func (l *Logger) LogKafkaMessage(ctx context.Context, action string, topic string, partition int, offset int64, processingTime time.Duration) {
	l.WithContext(ctx).Info("Kafka message processed",
		zap.String("action", action),
		zap.String("topic", topic),
		zap.Int("partition", partition),
		zap.Int64("offset", offset),
		zap.Duration("processing_time", processingTime),
	)
}

// LogAPICall logs an external API call with standard fields
func (l *Logger) LogAPICall(ctx context.Context, method string, url string, statusCode int, duration time.Duration, err error) {
	fields := []zap.Field{
		zap.String("action", "api_call"),
		zap.String("method", method),
		zap.String("url", url),
		zap.Int("status_code", statusCode),
		zap.Duration("duration", duration),
	}

	if err != nil {
		fields = append(fields, zap.Error(err))
		l.WithContext(ctx).Error("API call failed", fields...)
	} else {
		l.WithContext(ctx).Info("API call completed", fields...)
	}
}

// LogError logs an error with context and additional fields
func (l *Logger) LogError(ctx context.Context, err error, message string, fields ...zap.Field) {
	allFields := append(fields, zap.Error(err))
	l.WithContext(ctx).Error(message, allFields...)
}

// LogProcessingMetrics logs message processing metrics
func (l *Logger) LogProcessingMetrics(ctx context.Context, messageCount int, successCount int, errorCount int, avgProcessingTime time.Duration) {
	l.WithContext(ctx).Info("Processing metrics",
		zap.String("action", "processing_metrics"),
		zap.Int("message_count", messageCount),
		zap.Int("success_count", successCount),
		zap.Int("error_count", errorCount),
		zap.Duration("avg_processing_time", avgProcessingTime),
		zap.Float64("success_rate", float64(successCount)/float64(messageCount)*100),
	)
}

// GetCorrelationID extracts correlation ID from context
func GetCorrelationID(ctx context.Context) string {
	if correlationID, ok := ctx.Value(CorrelationIDKey).(string); ok {
		return correlationID
	}
	return ""
}

// WithCorrelationIDContext adds correlation ID to context
func WithCorrelationIDContext(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, CorrelationIDKey, correlationID)
}
