// logging/logging.go
package logging

import (
	"log"
	"os"
)

var logger *log.Logger

// Init initializes the global logger.
func Init() {
	logger = log.New(
		os.Stdout,
		"[golang-mail-billing] ",
		log.LstdFlags|log.Lshortfile,
	)
}

// Infof logs an informational message.
func Infof(format string, args ...interface{}) {
	if logger == nil {
		Init()
	}
	logger.Printf("INFO: "+format, args...)
}

// Errorf logs an error message.
func Errorf(format string, args ...interface{}) {
	if logger == nil {
		Init()
	}
	logger.Printf("ERROR: "+format, args...)
}

// Fatalf logs a fatal error message and exits.
func Fatalf(format string, args ...interface{}) {
	if logger == nil {
		Init()
	}
	logger.Fatalf("FATAL: "+format, args...)
}
