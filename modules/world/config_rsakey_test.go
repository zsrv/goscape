package world

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
)

func writeTestPrivatePEM(t *testing.T, bits int) string {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	path := filepath.Join(t.TempDir(), "private.pem")
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestConfigValidate_RSAKeyPath(t *testing.T) {
	// Valid key path validates clean.
	good := Config{Enable: true, TCPListenPort: 40000, CachePath: "x", RSAPrivateKeyPath: writeTestPrivatePEM(t, 1024)}
	if err := good.Validate(); err != nil {
		t.Errorf("valid key path: unexpected error: %v", err)
	}

	// Missing file fails validation.
	bad := Config{Enable: true, TCPListenPort: 40000, CachePath: "x", RSAPrivateKeyPath: "/no/such/key.pem"}
	if err := bad.Validate(); err == nil {
		t.Error("missing key path: expected validation error, got nil")
	}

	// Empty path is allowed (use built-in default).
	none := Config{Enable: true, TCPListenPort: 40000, CachePath: "x"}
	if err := none.Validate(); err != nil {
		t.Errorf("empty key path: unexpected error: %v", err)
	}
}
