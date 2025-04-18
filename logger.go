package websockethandler

import (
	"fmt"
	"strings"
)

type stdLogger interface {
	Print(...interface{})
	Printf(string, ...interface{})
	Println(...interface{})

	Fatal(...interface{})
	Fatalf(string, ...interface{})
	Fatalln(...interface{})

	Panic(...interface{})
	Panicf(string, ...interface{})
	Panicln(...interface{})
}

type strLog struct {
	UUID   string
	Event  interface{}
	Level  level
	Module string
	Format string
	Body   interface{}
}

type level uint8

const (
	panicLevel level = iota
	fatalLevel
	errorLevel
	warnLevel
	infoLevel
	debugLevel
	traceLevel
)

const (
	PanicLevel string = "panic"
	FatalLevel        = "fatal"
	ErrorLevel        = "error"
	WarnLevel         = "warning"
	InfoLevel         = "info"
	DebugLevel        = "debug"
	TraceLevel        = "trace"
)

func ParseLevel(lvl string) (level, error) {
	switch strings.ToLower(lvl) {
	case "panic":
		return panicLevel, nil
	case "fatal":
		return fatalLevel, nil
	case "error":
		return errorLevel, nil
	case "warn", "warning":
		return warnLevel, nil
	case "info":
		return infoLevel, nil
	case "debug":
		return debugLevel, nil
	case "trace":
		return traceLevel, nil
	}

	var l level
	return l, fmt.Errorf("not a valid Level: %q", lvl)
}
