package pack

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/zsrv/goscape/pkg/objtype"
)

func TestParseVarpConfig_ClientCodeDecimal(t *testing.T) {
	v, ok, err := parseVarpConfig("clientcode", "7")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != 7 {
		t.Fatalf("v=%v, want 7", v)
	}
}

func TestParseVarpConfig_ClientCodeHex(t *testing.T) {
	v, ok, err := parseVarpConfig("clientcode", "0x42")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != 66 {
		t.Fatalf("v=%v, want 66", v)
	}
}

func TestParseVarpConfig_ClientCodeNegative(t *testing.T) {
	v, ok, err := parseVarpConfig("clientcode", "-5")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != -5 {
		t.Fatalf("v=%v, want -5", v)
	}
}

func TestParseVarpConfig_ClientCodeNonNumericRejected(t *testing.T) {
	_, ok, err := parseVarpConfig("clientcode", "abc")
	if err == nil {
		t.Fatal("want err for non-numeric clientcode")
	}
	if !ok {
		t.Fatal("ok=false; want ok=true with err!=nil")
	}
}

func TestParseVarpConfig_ProtectBoolean(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"yes", true},
		{"no", false},
		{"true", true},
		{"false", false},
		{"1", true},
		{"0", false},
	} {
		v, ok, err := parseVarpConfig("protect", tc.in)
		if err != nil {
			t.Fatalf("%s: %v", tc.in, err)
		}
		if !ok {
			t.Fatalf("%s: ok=false", tc.in)
		}
		if v.(bool) != tc.want {
			t.Fatalf("%s: v=%v, want %v", tc.in, v, tc.want)
		}
	}
}

func TestParseVarpConfig_ProtectInvalidRejected(t *testing.T) {
	_, ok, err := parseVarpConfig("protect", "maybe")
	if err == nil {
		t.Fatal("want err for non-boolean protect")
	}
	if !ok {
		t.Fatal("ok=false; want ok=true with err!=nil")
	}
}

func TestParseVarpConfig_TransmitBoolean(t *testing.T) {
	v, ok, err := parseVarpConfig("transmit", "1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(bool) != true {
		t.Fatalf("v=%v, want true", v)
	}
}

func TestParseVarpConfig_ScopePerm(t *testing.T) {
	v, ok, err := parseVarpConfig("scope", "perm")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != objtype.VarpScopePerm {
		t.Fatalf("v=%v, want VarpScopePerm=%d", v, objtype.VarpScopePerm)
	}
}

func TestParseVarpConfig_ScopeTemp(t *testing.T) {
	v, ok, err := parseVarpConfig("scope", "temp")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(int) != objtype.VarpScopeTemp {
		t.Fatalf("v=%v, want VarpScopeTemp=%d", v, objtype.VarpScopeTemp)
	}
}

func TestParseVarpConfig_ScopeUnknownRejected(t *testing.T) {
	_, ok, err := parseVarpConfig("scope", "global")
	if err == nil {
		t.Fatal("want err for unknown scope")
	}
	if !ok {
		t.Fatal("ok=false; want ok=true with err!=nil")
	}
}

func TestParseVarpConfig_TypeAccepted(t *testing.T) {
	v, ok, err := parseVarpConfig("type", "int")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("ok=false")
	}
	if v.(objtype.ScriptVarType) != objtype.ScriptVarTypeInt {
		t.Fatalf("v=%v, want ScriptVarTypeInt", v)
	}
}

func TestParseVarpConfig_TypeUnknownRejected(t *testing.T) {
	_, ok, err := parseVarpConfig("type", "bogus")
	if err == nil {
		t.Fatal("want err for unknown type")
	}
	if !ok {
		t.Fatal("ok=false; want ok=true with err!=nil")
	}
}

func TestParseVarpConfig_UnknownKeyReturnsOkFalse(t *testing.T) {
	v, ok, err := parseVarpConfig("not_a_key", "whatever")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("ok=true for unknown key; want false")
	}
	if v != nil {
		t.Fatalf("v=%v, want nil", v)
	}
}

// Silence unused-import warnings for bytes and path/filepath — used by Task 3.
var _ = bytes.Equal
var _ = filepath.Join
