package logging

import "log"

// Logger is a tiny level-aware wrapper around the standard library logger.
// We keep it intentionally small: this project is about net/http patterns, not logging frameworks.
type Logger struct {
	debug bool
}

func New(level string) *Logger {
	return &Logger{debug: level == "debug"}
}

func (l *Logger) Debugf(format string, args ...any) {
	if !l.debug {
		return
	}
	log.Printf("level=debug "+format, args...)
}

func (l *Logger) Infof(format string, args ...any) {
	log.Printf("level=info "+format, args...)
}

func (l *Logger) Errorf(format string, args ...any) {
	log.Printf("level=error "+format, args...)
}

func (l *Logger) Warnf(format string, args ...any) {
	log.Printf("level=warn "+format, args...)
}
