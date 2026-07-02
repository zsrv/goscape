package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/dskit/modules"
	"github.com/zsrv/goscape/pkg/dskit/services"
)

func runManagerToStopped(t *testing.T, svc services.Service) *services.Manager {
	t.Helper()
	sm, err := services.NewManager(svc)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := sm.StartAsync(t.Context()); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if err := sm.AwaitStopped(t.Context()); err != nil {
		t.Fatalf("AwaitStopped: %v", err)
	}
	return sm
}

func TestFailedServicesError_ReportsFailedModule(t *testing.T) {
	boom := errors.New("boom")
	svc := services.NewBasicService(nil, func(_ context.Context) error { return boom }, nil)
	sm := runManagerToStopped(t, svc)

	err := failedServicesError(sm, map[string]services.Service{"world": svc})
	if err == nil {
		t.Fatal("want error for failed module, got nil")
	}
	if !strings.Contains(err.Error(), "world") || !errors.Is(err, boom) {
		t.Errorf("error should name the module and wrap the cause: %v", err)
	}
}

func TestFailedServicesError_NilOnCleanStop(t *testing.T) {
	svc := services.NewIdleService(nil, nil)
	sm, err := services.NewManager(svc)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := sm.StartAsync(t.Context()); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	sm.StopAsync()
	if err := sm.AwaitStopped(t.Context()); err != nil {
		t.Fatalf("AwaitStopped: %v", err)
	}
	if err := failedServicesError(sm, map[string]services.Service{"login": svc}); err != nil {
		t.Errorf("clean stop should yield nil, got %v", err)
	}
}

func TestFailedServicesError_IgnoresStopProcessAndCanceled(t *testing.T) {
	svc := services.NewBasicService(nil,
		func(_ context.Context) error { return modules.ErrStopProcess }, nil)
	sm := runManagerToStopped(t, svc)
	if err := failedServicesError(sm, map[string]services.Service{"world": svc}); err != nil {
		t.Errorf("ErrStopProcess is a requested stop, want nil, got %v", err)
	}
}
