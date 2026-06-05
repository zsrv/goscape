package ondemand

// rev-244 B3 T22 — failing pins for the rs2.cgi per-deployment token and the
// WS OnDemand gate config field / PORTING-EXCEPTION.
//
// TS contracts verified against Engine-TS 9aadcec4:
//   web.ts:101-105 — per_deployment_token field in client.ejs render
//   web.ts:165-176 — NODE_WS_ONDEMAND gate for state-2 (OnDemand) WS frames
//   view/client.ejs:322-324 — conditional cookie set when token non-empty
//   util/Environment.ts:21 — WEB_SOCKET_TOKEN_PROTECTION default false
//   util/Environment.ts:62 — NODE_WS_ONDEMAND default false
//
// All tests in this file MUST fail before the implementation is added and
// pass after; that exit-code flip is the TDD confirmation.

import (
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestRs2CgiTokenGateOff_EmitsEmptyToken pins web.ts:105:
//
//	per_deployment_token: WEB_SOCKET_TOKEN_PROTECTION ? getPublicPerDeploymentToken() : ''
//
// When WsTokenProtection is false (default), the rendered client template
// must NOT set the document.cookie per-deployment token line.
func TestRs2CgiTokenGateOff_EmitsEmptyToken(t *testing.T) {
	a := &OnDemand{
		log: discardLogger(),
		cfg: Config{
			NodeID:            10,
			Members:           true,
			WsTokenProtection: false, // gate off — default
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/rs2.cgi", nil)
	rr := httptest.NewRecorder()
	a.Rs2CgiHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	body, _ := io.ReadAll(rr.Body)
	got := string(body)

	// When token is empty the conditional block must not be rendered.
	if strings.Contains(got, "per_deployment_token") {
		t.Errorf("token gate off: body must not contain per_deployment_token cookie setter; got:\n%s", got)
	}
	if strings.Contains(got, "document.cookie") {
		t.Errorf("token gate off: body must not set document.cookie; got:\n%s", got)
	}
}

// TestRs2CgiTokenGateOn_EmitsCookieSetter pins web.ts:105 + client.ejs:322-324:
// when WsTokenProtection is true and PubPEM is set, the rendered template must
// contain the document.cookie setter with a non-empty per_deployment_token.
//
// Uses the same 512-bit test PEM from pkg/util/pemtoken/pemtoken_test.go.
const testRSAPEM = `-----BEGIN PUBLIC KEY-----
MFwwDQYJKoZIhvcNAQEBBQADSwAwSAJBAMD4gBEo1ChD4uMpgWEkRgco2A5UouBI
vnvgytcThUAgXeRnq5/zxZ6Bj+h/m5XsgR1OrNRxdaKpDFk2+q25uZkCAwEAAQ==
-----END PUBLIC KEY-----`

func TestRs2CgiTokenGateOn_EmitsCookieSetter(t *testing.T) {
	a := &OnDemand{
		log: discardLogger(),
		cfg: Config{
			NodeID:            10,
			Members:           true,
			WsTokenProtection: true,
			PubPEM:            []byte(testRSAPEM),
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/rs2.cgi", nil)
	rr := httptest.NewRecorder()
	a.Rs2CgiHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}
	body, _ := io.ReadAll(rr.Body)
	got := string(body)

	// When token is non-empty the cookie-setter block must be rendered.
	if !strings.Contains(got, "document.cookie") {
		t.Errorf("token gate on: body must contain document.cookie setter; got:\n%s", got)
	}
	if !strings.Contains(got, "per_deployment_token") {
		t.Errorf("token gate on: body must contain per_deployment_token; got:\n%s", got)
	}
}

// TestRs2CgiTokenGateOn_BadPEM_Serves500 pins the error path: when
// WsTokenProtection is true but PubPEM is invalid, Rs2CgiHandler must return
// 500 rather than silently emitting a bogus token.
func TestRs2CgiTokenGateOn_BadPEM_Serves500(t *testing.T) {
	a := &OnDemand{
		log: discardLogger(),
		cfg: Config{
			NodeID:            10,
			WsTokenProtection: true,
			PubPEM:            []byte("not a PEM block"),
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/rs2.cgi", nil)
	rr := httptest.NewRecorder()
	a.Rs2CgiHandler(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("bad PEM: status = %d, want 500", rr.Code)
	}
}

// TestWsOndemandConfigFieldExists pins the addition of the WsOndemand bool
// field to WebSocketConfig (mirroring NODE_WS_ONDEMAND at Environment.ts:62,
// default false). The field must be accessible; this test exercises the zero
// value (false) and an explicit true assignment.
func TestWsOndemandConfigFieldExists(t *testing.T) {
	cfg := WebSocketConfig{WsOndemand: false}
	if cfg.WsOndemand != false {
		t.Fatalf("WsOndemand zero value = %v, want false", cfg.WsOndemand)
	}
	cfg.WsOndemand = true
	if !cfg.WsOndemand {
		t.Fatalf("WsOndemand set to true but reads %v", cfg.WsOndemand)
	}
}

// TestWsTokenProtectionConfigFieldDefault pins the default value for the
// ondemand.websocket-token-protection CLI flag (false, matching TS
// Environment.ts:21 WEB_SOCKET_TOKEN_PROTECTION default false).
func TestWsTokenProtectionConfigFieldDefault(t *testing.T) {
	var cfg Config
	cfg.RegisterFlagsAndApplyDefaults(flag.NewFlagSet("test", flag.ContinueOnError))
	if cfg.WsTokenProtection != false {
		t.Fatalf("WsTokenProtection default = %v, want false", cfg.WsTokenProtection)
	}
}

// TestWsOndemandConfigFlagDefault pins the default value for the
// ondemand.websocket-ws-ondemand CLI flag (false, matching TS
// Environment.ts:62 NODE_WS_ONDEMAND default false).
func TestWsOndemandConfigFlagDefault(t *testing.T) {
	var cfg Config
	cfg.RegisterFlagsAndApplyDefaults(flag.NewFlagSet("test", flag.ContinueOnError))
	if cfg.WebSocket.WsOndemand != false {
		t.Fatalf("WebSocket.WsOndemand default = %v, want false", cfg.WebSocket.WsOndemand)
	}
}

// TestConfigValidate_PEMStartupFatal pins the startup-fatal PEM validation
// that mirrors PemUtil.ts:10 (Engine-TS 9aadcec4): the PEM is parsed at
// module load; a missing or malformed file is startup-fatal.
//
// Rules:
//   - gate off (WsTokenProtection=false): Validate passes regardless of PubPEM
//   - gate on + PubPEM empty: Validate returns error (missing key)
//   - gate on + PubPEM garbage: Validate returns error (parse failure)
//   - gate on + valid PubPEM: Validate returns nil
func TestConfigValidate_PEMStartupFatal(t *testing.T) {
	t.Run("gate off, empty PEM, passes", func(t *testing.T) {
		cfg := Config{WsTokenProtection: false, PubPEM: nil}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("gate off: Validate() = %v, want nil", err)
		}
	})
	t.Run("gate off, garbage PEM, passes", func(t *testing.T) {
		cfg := Config{WsTokenProtection: false, PubPEM: []byte("not a pem")}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("gate off with garbage PEM: Validate() = %v, want nil", err)
		}
	})
	t.Run("gate on, empty PEM, fails", func(t *testing.T) {
		cfg := Config{WsTokenProtection: true, PubPEM: nil}
		if err := cfg.Validate(); err == nil {
			t.Fatal("gate on + empty PEM: Validate() = nil, want error")
		}
	})
	t.Run("gate on, garbage PEM, fails", func(t *testing.T) {
		cfg := Config{WsTokenProtection: true, PubPEM: []byte("not a pem block")}
		if err := cfg.Validate(); err == nil {
			t.Fatal("gate on + garbage PEM: Validate() = nil, want error")
		}
	})
	t.Run("gate on, valid PEM, passes", func(t *testing.T) {
		cfg := Config{WsTokenProtection: true, PubPEM: []byte(testRSAPEM)}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("gate on + valid PEM: Validate() = %v, want nil", err)
		}
	})
}
