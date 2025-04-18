package websockethandler

import (
	"runtime"
	"strings"
)

func getFunctionName() string {
	pc, _, _, _ := runtime.Caller(1)
	fullFuncName := runtime.FuncForPC(pc).Name()
	funcName := strings.Split(fullFuncName, "/")
	return funcName[len(funcName)-1]
}
