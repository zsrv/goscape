// Provenance-includes-location: https://github.com/grafana/dskit/blob/main/grpcutil/health_check_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: Grafana Labs.

package grpcutil

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"

	"github.com/zsrv/goscape/pkg/dskit/services"
)

func TestHealthCheck_Check_ServiceManager(t *testing.T) {
	tests := map[string]struct {
		states   []services.State
		expected grpc_health_v1.HealthCheckResponse_ServingStatus
	}{
		"all services are new": {
			states:   []services.State{services.New, services.New},
			expected: grpc_health_v1.HealthCheckResponse_NOT_SERVING,
		},
		"all services are starting": {
			states:   []services.State{services.Starting, services.Starting},
			expected: grpc_health_v1.HealthCheckResponse_NOT_SERVING,
		},
		"some services are starting and some running": {
			states:   []services.State{services.Starting, services.Running},
			expected: grpc_health_v1.HealthCheckResponse_NOT_SERVING,
		},
		"all services are running": {
			states:   []services.State{services.Running, services.Running},
			expected: grpc_health_v1.HealthCheckResponse_SERVING,
		},
		"some services are stopping": {
			states:   []services.State{services.Running, services.Stopping},
			expected: grpc_health_v1.HealthCheckResponse_SERVING,
		},
		"some services are terminated while others running": {
			states:   []services.State{services.Running, services.Terminated},
			expected: grpc_health_v1.HealthCheckResponse_SERVING,
		},
		"all services are stopping": {
			states:   []services.State{services.Stopping, services.Stopping},
			expected: grpc_health_v1.HealthCheckResponse_SERVING,
		},
		"some services are terminated while others stopping": {
			states:   []services.State{services.Stopping, services.Terminated},
			expected: grpc_health_v1.HealthCheckResponse_SERVING,
		},
		"a service has failed while others are running": {
			states:   []services.State{services.Running, services.Failed},
			expected: grpc_health_v1.HealthCheckResponse_NOT_SERVING,
		},
		"all services are terminated": {
			states:   []services.State{services.Terminated, services.Terminated},
			expected: grpc_health_v1.HealthCheckResponse_NOT_SERVING,
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			var svcs []services.Service
			for range testData.states {
				svcs = append(svcs, &mockService{})
			}

			ctx := t.Context()
			req := &grpc_health_v1.HealthCheckRequest{}
			sm, err := services.NewManager(svcs...)
			require.NoError(t, err)

			// Switch the state of each mocked services.
			for i, s := range svcs {
				s.(*mockService).switchState(testData.states[i])
			}

			h := NewHealthCheckFrom(WithManager(sm))
			res, err := h.Check(ctx, req)

			require.NoError(t, err)
			require.Equal(t, testData.expected, res.Status)
		})
	}
}

func TestHealthCheck_List_ServiceManager(t *testing.T) {
	tests := map[string]struct {
		states   []services.State
		expected map[string]*grpc_health_v1.HealthCheckResponse
	}{
		"all services are new": {
			states: []services.State{services.New, services.New},
			expected: map[string]*grpc_health_v1.HealthCheckResponse{
				"server": {Status: grpc_health_v1.HealthCheckResponse_NOT_SERVING},
			},
		},
		"all services are running": {
			states: []services.State{services.Running, services.Running},
			expected: map[string]*grpc_health_v1.HealthCheckResponse{
				"server": {Status: grpc_health_v1.HealthCheckResponse_SERVING},
			},
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			var svcs []services.Service
			for range testData.states {
				svcs = append(svcs, &mockService{})
			}

			ctx := t.Context()
			req := &grpc_health_v1.HealthListRequest{}
			sm, err := services.NewManager(svcs...)
			require.NoError(t, err)

			// Switch the state of each mocked services.
			for i, s := range svcs {
				s.(*mockService).switchState(testData.states[i])
			}

			h := NewHealthCheckFrom(WithManager(sm))
			res, err := h.List(ctx, req)

			require.NoError(t, err)
			require.Len(t, res.Statuses, len(testData.expected))
			for name, want := range testData.expected {
				require.Contains(t, res.Statuses, name)
				require.Equal(t, want.Status, res.Statuses[name].Status)
			}
		})
	}
}

func TestHealthCheck_Check_ShutdownRequested(t *testing.T) {
	tests := map[string]struct {
		requested bool
		expected  grpc_health_v1.HealthCheckResponse_ServingStatus
	}{
		"shutdown not requested": {
			requested: false,
			expected:  grpc_health_v1.HealthCheckResponse_SERVING,
		},
		"shutdown is requested": {
			requested: true,
			expected:  grpc_health_v1.HealthCheckResponse_NOT_SERVING,
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			ctx := t.Context()
			req := &grpc_health_v1.HealthCheckRequest{}

			requested := new(atomic.Bool)
			requested.Store(testData.requested)

			h := NewHealthCheckFrom(WithShutdownRequested(requested))
			res, err := h.Check(ctx, req)

			require.NoError(t, err)
			require.Equal(t, testData.expected, res.Status)
		})
	}
}

// TestHealthCheck_Check_ShutdownRequestedFlips pins that the Check closure
// reads the flag on every call rather than capturing its value once.
func TestHealthCheck_Check_ShutdownRequestedFlips(t *testing.T) {
	requested := new(atomic.Bool)
	h := NewHealthCheckFrom(WithShutdownRequested(requested))

	res, err := h.Check(t.Context(), &grpc_health_v1.HealthCheckRequest{})
	require.NoError(t, err)
	require.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, res.Status)

	requested.Store(true)

	res, err = h.Check(t.Context(), &grpc_health_v1.HealthCheckRequest{})
	require.NoError(t, err)
	require.Equal(t, grpc_health_v1.HealthCheckResponse_NOT_SERVING, res.Status)
}

// TestHealthCheck_Check_AllChecksMustPass pins the AND semantics across
// several Checks: any false check makes the whole instance NOT_SERVING.
func TestHealthCheck_Check_AllChecksMustPass(t *testing.T) {
	yes := Check(func(context.Context) bool { return true })
	no := Check(func(context.Context) bool { return false })

	for name, tc := range map[string]struct {
		checks   []Check
		expected grpc_health_v1.HealthCheckResponse_ServingStatus
	}{
		"no checks at all":  {nil, grpc_health_v1.HealthCheckResponse_SERVING},
		"every check true":  {[]Check{yes, yes}, grpc_health_v1.HealthCheckResponse_SERVING},
		"first check false": {[]Check{no, yes}, grpc_health_v1.HealthCheckResponse_NOT_SERVING},
		"last check false":  {[]Check{yes, no}, grpc_health_v1.HealthCheckResponse_NOT_SERVING},
	} {
		t.Run(name, func(t *testing.T) {
			res, err := NewHealthCheckFrom(tc.checks...).Check(t.Context(), &grpc_health_v1.HealthCheckRequest{})
			require.NoError(t, err)
			require.Equal(t, tc.expected, res.Status)
		})
	}
}

// TestNewHealthCheck confirms the service-manager convenience constructor
// wires exactly the WithManager check.
func TestNewHealthCheck(t *testing.T) {
	svc := &mockService{}
	sm, err := services.NewManager(svc)
	require.NoError(t, err)

	h := NewHealthCheck(sm)
	res, err := h.Check(t.Context(), &grpc_health_v1.HealthCheckRequest{})
	require.NoError(t, err)
	require.Equal(t, grpc_health_v1.HealthCheckResponse_NOT_SERVING, res.Status)

	svc.switchState(services.Running)

	res, err = h.Check(t.Context(), &grpc_health_v1.HealthCheckRequest{})
	require.NoError(t, err)
	require.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, res.Status)
}

func TestHealthCheck_Watch_IsUnimplemented(t *testing.T) {
	h := NewHealthCheckFrom()
	err := h.Watch(&grpc_health_v1.HealthCheckRequest{}, nil)
	require.Error(t, err)
	require.Equal(t, codes.Unimplemented, status.Code(err))
}

type mockService struct {
	services.Service
	state     services.State
	listeners []services.Listener
}

func (s *mockService) switchState(desiredState services.State) {
	// Simulate all the states between the current state and the desired one.
	orderedStates := []services.State{services.New, services.Starting, services.Running, services.Failed, services.Stopping, services.Terminated}
	simulationStarted := false

	for _, orderedState := range orderedStates {
		// Skip until we reach the current state.
		if !simulationStarted && orderedState != s.state {
			continue
		}

		// Start the simulation once we reach the current state.
		if orderedState == s.state {
			simulationStarted = true
			continue
		}

		// Skip the failed state, unless it's the desired one.
		if orderedState == services.Failed && desiredState != services.Failed {
			continue
		}

		s.state = orderedState

		// Synchronously call listeners to avoid flaky tests.
		for _, listener := range s.listeners {
			switch orderedState {
			case services.Starting:
				listener.Starting()
			case services.Running:
				listener.Running()
			case services.Stopping:
				listener.Stopping(services.Running)
			case services.Failed:
				listener.Failed(services.Running, errors.New("mocked error"))
			case services.Terminated:
				listener.Terminated(services.Stopping)
			}
		}

		if orderedState == desiredState {
			break
		}
	}
}

func (s *mockService) State() services.State {
	return s.state
}

func (s *mockService) AddListener(listener services.Listener) func() {
	s.listeners = append(s.listeners, listener)
	return func() {}
}

func (s *mockService) StartAsync(_ context.Context) error      { return nil }
func (s *mockService) AwaitRunning(_ context.Context) error    { return nil }
func (s *mockService) StopAsync()                              {}
func (s *mockService) AwaitTerminated(_ context.Context) error { return nil }
func (s *mockService) FailureCase() error                      { return nil }
