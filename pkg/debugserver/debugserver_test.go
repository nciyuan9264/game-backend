package debugserver

import "testing"

func TestConfigFromEnvDisabledWhenAddressEmpty(t *testing.T) {
	t.Setenv("ACQUIRE_DEBUG_ADDR", "")

	cfg, ok := ConfigFromEnv()

	if ok {
		t.Fatalf("ok = true, want false")
	}
	if cfg.Addr != "" {
		t.Fatalf("addr = %q, want empty", cfg.Addr)
	}
}

func TestConfigFromEnvUsesAcquireDebugAddr(t *testing.T) {
	t.Setenv("ACQUIRE_DEBUG_ADDR", "127.0.0.1:6060")

	cfg, ok := ConfigFromEnv()

	if !ok {
		t.Fatalf("ok = false, want true")
	}
	if cfg.Addr != "127.0.0.1:6060" {
		t.Fatalf("addr = %q, want 127.0.0.1:6060", cfg.Addr)
	}
}
