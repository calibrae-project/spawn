// Package L - multi-level logging
package L

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
)

const (
	logpath = "/tmp/debug.log"
)

var (
	Info, Warn, Error, Debug func(...interface{})
	Infof, Debugf            func(string, ...interface{})
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
		fmt.Println(whereAmI(), "Error", i)
	}
	Warn = func(i ...interface{}) {
		fmt.Println(whereAmI(), "Warning", i)
	}
	Debug = func(i ...interface{}) {
		fmt.Fprintln(logfile, i, "\n     " + whereAmI() + "()")
	}
	Debugf = func(format string, i ...interface{}) {
		fmt.Fprintf(logfile, "[" + format + "]\n     " + whereAmI()+ "()" + "\n", i...)
	}
	Debug("Started logger")
}

func whereAmI(depthList ...int) string {
	var depth int
	if depthList == nil {
		depth = 2
	} else {
		depth = depthList[0]
	}
	function, file, line, _ := runtime.Caller(depth)
	return fmt.Sprintf("%s:%d %s", file, line, chop(runtime.FuncForPC(function).Name(), "."))
}

func chop(original, separator string) string {
	i := strings.LastIndex(original, separator)
	if i == -1 {
		return original
	}
	return original[i+1:]
}
