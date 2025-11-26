package logging

import (
	"log"
	"os"
)

// Debug is enabled when environment variable DEBUG is set to "1" or "true".
var Debug bool

func init() {
	d := os.Getenv("DEBUG")
	if d == "1" || d == "true" {
		Debug = true
	} else {
		Debug = false
	}
}

// Debugf logs when Debug is enabled.
func Debugf(format string, v ...any) {
	if Debug {
		log.Printf("[DEBUG] "+format, v...)
	}
}

// Infof always logs informational messages.
func Infof(format string, v ...any) {
	log.Printf("[INFO] "+format, v...)
}

// Errorf always logs errors.
func Errorf(format string, v ...any) {
	log.Printf("[ERROR] "+format, v...)
}
