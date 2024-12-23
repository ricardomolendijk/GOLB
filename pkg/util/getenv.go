package util

import (
	"os"
	"strconv"

	"github.com/ricardomolendijk/GOLB/pkg/l"
)

//* getEnv retrieves a string environment variable or returns a default
func GetEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	l.Warn("Variable missing from .env file, using default", "value", key, "default", defaultValue)
	return defaultValue
}

//* getEnvAsBool retrieves a boolean environment variable or returns a default
func GetEnvAsBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
		l.Warn("Invalid boolean in .env file, using default", "value", key, "default", defaultValue)
	} else {
		l.Warn("Variable missing from .env file, using default", "value", key, "default", defaultValue)
	}
	return defaultValue
}