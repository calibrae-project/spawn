// Package logg - multi-level logging
package logg

import (
	"log"
	"os"
)

const (
	logpath = "/tmp/debug.log"
)

var (
	Info, Warn, Error, Debug *log.Logger
)

func init() {
	logfile, err := os.OpenFile(logpath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}
	Info = log.New(os.Stdout, "", 0)
	Error = log.New(os.Stderr, "ERROR ", 0)
	Warn = log.New(os.Stderr, "WARNING ", 0)
	Debug = log.New(logfile, "", log.Llongfile|log.Ltime)
	Debug.Println("Started logger")
}
