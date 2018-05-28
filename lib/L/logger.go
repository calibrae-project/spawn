// Package L - multi-level logging
package L

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"github.com/fatih/color"
	"io"
	"time"
)

const (
	logpath = "/tmp/debug.log"
)

var (
	Info, Warn, Error, Debug, DebugNoInfo func(...interface{})
	Infof, Debugf            func(string, ...interface{})
	bold = color.New(color.Bold).FprintfFunc()
	blue = color.New(color.FgBlue).FprintfFunc()
	red = color.New(color.FgRed).FprintfFunc()
)
func init() {
	logfile, err := os.OpenFile(logpath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

	if err != nil {
		log.Fatal(err)
	}
	Info = func(i ...interface{}) {
		fmt.Println(i)
	}
	Infof = func(format string, i ...interface{}) {
		fmt.Printf(format, i...)
	}
	Error = func(i ...interface{}) {
		red(os.Stderr, "Error ")
		fmt.Fprint(os.Stderr, i...)
		source(os.Stderr)
		function(os.Stderr)
	}
	Warn = func(i ...interface{}) {
		red(os.Stderr, "Warning ")
		fmt.Print(i...)
		source(os.Stderr)
		fmt.Fprint(os.Stderr, " ")
		function(os.Stderr)
		fmt.Fprint(os.Stderr, "\n")

	}
	Debug = func(i ...interface{}) {
		fmt.Fprint(logfile, time.Now().Format(time.StampMilli))
		fmt.Fprint(logfile, " ")
		bold(logfile, fmt.Sprint(i...))
		fmt.Fprint(logfile, " ")
		source(logfile)
		fmt.Fprint(logfile, " ")
		function(logfile)
		fmt.Fprint(logfile, "\n")
	}
	DebugNoInfo = func(i ...interface{}) {
		fmt.Fprintf(logfile, "%s\n", fmt.Sprint(i...))
	}
}

func source(out io.Writer) {
	_, file, line, _ := runtime.Caller(2)
	blue(out, fmt.Sprintf("%s:%d", file, line))
}
func function(out io.Writer) {
	function, _, _, _ := runtime.Caller(2)
	fmt.Fprint(out, chop(runtime.FuncForPC(function).Name(), ".")+"()")
}

func chop(original, separator string) string {
	i := strings.LastIndex(original, separator)
	if i == -1 {
		return original
	}
	return original[i+1:]
}
