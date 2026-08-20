package hiscore

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestConsumerFromHeaders_Untrusted(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	r.Header.Set("X-Consumer-Username", "partner-a")
	r.Header.Set("X-Anonymous-Consumer", "false")

	got := consumerFromHeaders(r, false)
	if got.Consumer != "" {
		t.Errorf("Consumer = %q, want empty — headers must be ignored when untrusted", got.Consumer)
	}
	if !got.Anonymous {
		t.Error("Anonymous = false, want true when headers are not trusted")
	}
}

func TestConsumerFromHeaders_Trusted(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	r.Header.Set("X-Consumer-Username", "partner-a")
	r.Header.Set("X-Anonymous-Consumer", "false")

	got := consumerFromHeaders(r, true)
	if got.Consumer != "partner-a" {
		t.Errorf("Consumer = %q, want partner-a", got.Consumer)
	}
	if got.Anonymous {
		t.Error("Anonymous = true, want false")
	}
}

func TestConsumerFromHeaders_TrustedAnonymous(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/v1/skills", nil)
	r.Header.Set("X-Consumer-Username", "anonymous-consumer")
	r.Header.Set("X-Anonymous-Consumer", "true")

	got := consumerFromHeaders(r, true)
	if !got.Anonymous {
		t.Error("Anonymous = false, want true — X-Anonymous-Consumer: true is authoritative")
	}
}

func TestBackstop_AllowsUnderLimitThenBlocks(t *testing.T) {
	b := newBackstop(3)
	now := testClock

	for i := range 3 {
		if !b.allow("k", now) {
			t.Fatalf("request %d: blocked under the limit", i+1)
		}
	}
	if b.allow("k", now) {
		t.Error("request 4: allowed above the limit of 3")
	}
}

func TestBackstop_WindowRolls(t *testing.T) {
	b := newBackstop(1)
	if !b.allow("k", testClock) {
		t.Fatal("first request blocked")
	}
	if b.allow("k", testClock.Add(30*time.Second)) {
		t.Error("second request inside the window was allowed")
	}
	if !b.allow("k", testClock.Add(61*time.Second)) {
		t.Error("request after the window rolled was blocked")
	}
}

func TestBackstop_KeysAreIndependent(t *testing.T) {
	b := newBackstop(1)
	if !b.allow("a", testClock) || !b.allow("b", testClock) {
		t.Error("distinct keys must not share a budget")
	}
}

func TestBackstop_ZeroDisables(t *testing.T) {
	b := newBackstop(0)
	for i := range 1000 {
		if !b.allow("k", testClock) {
			t.Fatalf("request %d blocked, but rate 0 disables the limiter", i)
		}
	}
}

func TestCaller_LimiterKey(t *testing.T) {
	withConsumer := caller{Consumer: "partner-a", IP: "203.0.113.1"}
	if got, want := withConsumer.limiterKey(), "consumer:partner-a"; got != want {
		t.Errorf("limiterKey() = %q, want %q", got, want)
	}

	anonymous := caller{Anonymous: true, IP: "203.0.113.1"}
	if got, want := anonymous.limiterKey(), "ip:203.0.113.1"; got != want {
		t.Errorf("limiterKey() = %q, want %q", got, want)
	}

	other := caller{Consumer: "partner-b", IP: "203.0.113.1"}
	if withConsumer.limiterKey() == other.limiterKey() {
		t.Error("callers differing only in consumer name must not share a limiter key — one partner's traffic could consume another's budget")
	}
}
