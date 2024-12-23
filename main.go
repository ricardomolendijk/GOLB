package main

import (
	"github.com/joho/godotenv"
	"github.com/ricardomolendijk/GOLB/internal/lb"
	"github.com/ricardomolendijk/GOLB/pkg/l"
	"github.com/ricardomolendijk/GOLB/pkg/util"
)

func main() {
	//* Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		l.Fatal("Error loading .env file!", "error", err)
	}

	//* Retrieve configuration values with defaults
	debug := util.GetEnvAsBool("DEBUG", false)
	listen := util.GetEnv("LISTEN", ":443")
	logDir := util.GetEnv("LOGDIR", "./logs")
	saveLogs := util.GetEnvAsBool("SAVELOGS", true)

	//* Set up logging
	logFile, err := l.NewLogger(debug, logDir, saveLogs)
	if err != nil {
		l.Fatal("Failed to set up logging", "error", err)
	}
	defer func() {
		if logFile != nil {
			logFile.Close()
		}
	}()

	//* Launch the API server
	lb.NewLB(listen)
}

