package app_test

import (
	"log/slog"
	"testing"

	yaml "go.yaml.in/yaml/v2"

	"github.com/zsrv/goscape/cmd/goscape/app"
	"github.com/zsrv/goscape/pkg/util/log"
)

func TestGlobalLogLevelParsesTrace(t *testing.T) {
	var c app.Config
	if err := yaml.Unmarshal([]byte("log_level: trace\n"), &c); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if slog.Level(c.LogLevel) != log.LevelTrace {
		t.Errorf("LogLevel = %v, want trace(-8)", slog.Level(c.LogLevel))
	}
}

func TestDefaultLogLevelIsInfo(t *testing.T) {
	c := app.NewDefaultConfig()
	if slog.Level(c.LogLevel) != slog.LevelInfo {
		t.Errorf("default LogLevel = %v, want INFO", slog.Level(c.LogLevel))
	}
}
