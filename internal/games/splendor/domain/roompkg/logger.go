package roompkg

import (
	"fmt"
	"log"
)

type coreLogger struct{}

func (coreLogger) Info(msg string, kv ...any)  { log.Println("INFO  " + format(msg, kv)) }
func (coreLogger) Warn(msg string, kv ...any)  { log.Println("WARN  " + format(msg, kv)) }
func (coreLogger) Error(msg string, kv ...any) { log.Println("ERROR " + format(msg, kv)) }

func format(msg string, kv []any) string {
	if len(kv) == 0 {
		return msg
	}
	out := msg
	for i := 0; i+1 < len(kv); i += 2 {
		out += fmt.Sprintf(" %v=%v", kv[i], kv[i+1])
	}
	return out
}
