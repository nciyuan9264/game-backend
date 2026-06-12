package roompkg

import (
	"fmt"

	"github.com/nciyuan9264/game-backend/pkg/logger"
)

type coreLogger struct{}

func toFields(kv []any) []logger.Field {
	fs := make([]logger.Field, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		key := fmt.Sprint(kv[i])
		fs = append(fs, logger.F(key, kv[i+1]))
	}
	return fs
}

func (coreLogger) Info(msg string, kv ...any)  { logger.Info(msg, toFields(kv)...) }
func (coreLogger) Warn(msg string, kv ...any)  { logger.Warn(msg, toFields(kv)...) }
func (coreLogger) Error(msg string, kv ...any) { logger.Error(msg, toFields(kv)...) }
