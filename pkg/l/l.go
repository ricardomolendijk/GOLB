// Logging package, using l for convenience.
package l

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/charmbracelet/log"
)

func NewLogger(d bool, logDir string, saveLogs bool) (*os.File, error) {
	// Set log level based on debug flag
	if d {
		log.SetLevel(log.DebugLevel)
	}

	// If saveLogs is true, set up file logging
	if saveLogs {
		if err := os.MkdirAll(logDir, 0755); err != nil {
			log.Error("failed to create log directory", "error", err)
			return nil, err
		}

		// Create a log file with the current date in the name
		logFileName := fmt.Sprintf("log_%s.log", time.Now().Format("2006-01-02_15-04-05"))
		logFilePath := fmt.Sprintf("%s/%s", logDir, logFileName)

		// Open or create the log file in append mode
		logFile, err := os.OpenFile(logFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Error("failed to open log file", "error", err)
			return nil, err
		}

		// Set up a writer for terminal (supports colors) and the log file (plain text)
		multiWriter := io.MultiWriter(logFile, os.Stdout)

		// Configure the logger
		log.SetOutput(multiWriter)

		return logFile, nil
	}

	// Default to terminal output only if not saving logs
	log.SetOutput(os.Stdout)
	return nil, nil
}


func Warn(message string, keyValues ...interface{}) {
	log.Warn(message, keyValues...)
}

func Debug(message string, keyValues ...interface{}) {
	log.SetReportCaller(true)
	log.Debug(message, keyValues...)
	log.SetReportCaller(false)
}

func Error(message string, keyValues ...interface{}) {
	log.Error(message, keyValues...)
}

func Info(message string, keyValues ...interface{}) {
	log.Info(message, keyValues...)
}

func Fatal(message string, keyValues ...interface{}) {
	log.Fatal(message, keyValues...)
}

func Print(message string, keyValues ...interface{}) {
	log.Print(message, keyValues...)
}




