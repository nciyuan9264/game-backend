package debugserver

import (
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/nciyuan9264/game-backend/pkg/logger"
)

const debugAddrEnv = "ACQUIRE_DEBUG_ADDR"

type Config struct {
	Addr string
}

func ConfigFromEnv() (Config, bool) {
	addr := os.Getenv(debugAddrEnv)
	if addr == "" {
		return Config{}, false
	}
	return Config{Addr: addr}, true
}

func StartFromEnv() {
	cfg, ok := ConfigFromEnv()
	if !ok {
		return
	}
	go func() {
		logger.Info("debug server started", logger.F("addr", cfg.Addr))
		if err := http.ListenAndServe(cfg.Addr, nil); err != nil {
			logger.Error("debug server stopped", logger.F("addr", cfg.Addr), logger.F("error", err))
		}
	}()
}
