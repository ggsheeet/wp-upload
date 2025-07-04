package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
)

const (
	INFO    = "INFO"
	WARNING = "WARNING"
	ERROR   = "ERROR"
	DEBUG   = "DEBUG"
)

type ColoredLogger struct {
	*log.Logger
}

func NewColoredLogger(prefix string, out io.Writer) *ColoredLogger {
	if out == nil {
		out = os.Stdout
	}
	return &ColoredLogger{
		Logger: log.New(out, prefix, log.Ltime),
	}
}

func getCallerInfo(skip int) (string, int, string) {
	pc, file, line, ok := runtime.Caller(skip)
	if !ok {
		return "unknown", 0, "unknown"
	}

	fn := runtime.FuncForPC(pc)
	var funcName string
	if fn == nil {
		funcName = "unknown"
	} else {
		funcName = fn.Name()
		if lastSlash := strings.LastIndex(funcName, "/"); lastSlash >= 0 {
			funcName = funcName[lastSlash+1:]
		}
		if lastDot := strings.LastIndex(funcName, "."); lastDot >= 0 {
			funcName = funcName[lastDot+1:]
		}
	}

	if wd, err := os.Getwd(); err == nil {
		if relPath, err := filepath.Rel(wd, file); err == nil {
			file = relPath
		}
	}

	return file, line, funcName
}

func (l *ColoredLogger) Info(format string, v ...any) {
	l.Printf("%s[%s]%s %s", colorGreen, INFO, colorReset, fmt.Sprintf(format, v...))
}

func (l *ColoredLogger) Warning(format string, v ...any) {
	file, line, fn := getCallerInfo(2)
	l.Printf("%s[%s]%s %s %s[%s:%d @ %s]%s",
		colorYellow, WARNING, colorReset,
		fmt.Sprintf(format, v...),
		colorCyan, file, line, fn, colorReset)
}

func (l *ColoredLogger) Error(format string, v ...any) {
	file, line, fn := getCallerInfo(2)
	l.Printf("%s[%s]%s %s %s[%s:%d @ %s]%s",
		colorRed, ERROR, colorReset,
		fmt.Sprintf(format, v...),
		colorPurple, file, line, fn, colorReset)
}

func (l *ColoredLogger) Debug(format string, v ...any) {
	file, line, fn := getCallerInfo(2)
	l.Printf("%s[%s]%s %s %s[%s:%d @ %s]%s",
		colorBlue, DEBUG, colorReset,
		fmt.Sprintf(format, v...),
		colorCyan, file, line, fn, colorReset)
}

var Default = NewColoredLogger("", os.Stdout)
