package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"unsafe"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Config mirrors config.LoggingConfig to avoid import cycles.
type Config struct {
	Level string // debug | info | warn | error
	File  string // path to log file; "" means stderr only
}

// atomicLevel controls the log level at runtime without rebuilding the logger.
var atomicLevel = zap.NewAtomicLevel()

// global logger pointer - swapped atomically for runtime level changes.
var globalLogger unsafe.Pointer

// New builds a zap logger. If debugMode is true, level is forced to debug and
// output goes to both file (if configured) and stderr, regardless of cfg.
func New(cfg Config, debugMode bool) (*zap.Logger, error) {
	level := parseLevel(cfg.Level)
	if debugMode {
		level = zapcore.DebugLevel
	}
	atomicLevel.SetLevel(level)

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "ts"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderCfg.EncodeLevel = zapcore.CapitalLevelEncoder

	var cores []zapcore.Core

	// File sink
	if cfg.File != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.File), 0o755); err != nil {
			return nil, fmt.Errorf("logging: creating log dir: %w", err)
		}
		f, err := os.OpenFile(cfg.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, fmt.Errorf("logging: opening log file %q: %w", cfg.File, err)
		}
		fileCore := zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderCfg),
			zapcore.AddSync(f),
			atomicLevel,
		)
		cores = append(cores, fileCore)
	}

	// Console sink - always on in debug mode, never in normal mode
	if debugMode || cfg.File == "" {
		consoleCore := zapcore.NewCore(
			zapcore.NewConsoleEncoder(encoderCfg),
			zapcore.AddSync(os.Stderr),
			atomicLevel,
		)
		cores = append(cores, consoleCore)
	}

	log := zap.New(zapcore.NewTee(cores...), zap.AddCaller())
	storeGlobal(log)
	return log, nil
}

// SetLevel changes the active log level at runtime. Takes effect immediately
// on all cores built by New(). Supported values: debug, info, warn, error.
func SetLevel(levelStr string) {
	atomicLevel.SetLevel(parseLevel(levelStr))
}

// GetLevel returns the current log level as a string.
func GetLevel() string {
	return atomicLevel.Level().String()
}

// parseLevel converts a string level to zapcore.Level, defaulting to Info.
func parseLevel(s string) zapcore.Level {
	switch s {
	case "debug":
		return zapcore.DebugLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

func storeGlobal(log *zap.Logger) {
	atomic.StorePointer(&globalLogger, unsafe.Pointer(log))
}

func loadGlobal() *zap.Logger {
	p := atomic.LoadPointer(&globalLogger)
	if p == nil {
		l, _ := zap.NewProduction()
		return l
	}
	return (*zap.Logger)(p)
}

// L returns the global logger.
func L() *zap.Logger {
	return loadGlobal()
}
