package protocol

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
)

func TestParseRSAKeyPEM_PKCS1RoundTrip(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	got, err := ParseRSAKeyPEM(pemBytes)
	if err != nil {
		t.Fatalf("ParseRSAKeyPEM: %v", err)
	}
	if got.Modulus.Cmp(priv.N) != 0 {
		t.Error("modulus mismatch")
	}
	if got.PrivateExponent.Cmp(priv.D) != 0 {
		t.Error("private exponent mismatch")
	}
	if got.PublicExponent.Int64() != int64(priv.E) {
		t.Errorf("public exponent: got %d want %d", got.PublicExponent.Int64(), priv.E)
	}
}

func TestParseRSAKeyPEM_PKCS8RoundTrip(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("MarshalPKCS8PrivateKey: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	got, err := ParseRSAKeyPEM(pemBytes)
	if err != nil {
		t.Fatalf("ParseRSAKeyPEM: %v", err)
	}
	if got.Modulus.Cmp(priv.N) != 0 || got.PrivateExponent.Cmp(priv.D) != 0 {
		t.Error("PKCS8 key material mismatch")
	}
}

func TestParseRSAKeyPEM_Errors(t *testing.T) {
	if _, err := ParseRSAKeyPEM([]byte("not a pem")); err == nil {
		t.Error("expected error for non-PEM input")
	}
	priv, _ := rsa.GenerateKey(rand.Reader, 1024)
	pubDER, _ := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})
	if _, err := ParseRSAKeyPEM(pubPEM); err == nil {
		t.Error("expected error for public-key PEM (not a private key)")
	}
}

func TestDefaultRSAKey_GlobalsAlias(t *testing.T) {
	if Modulus != DefaultRSAKey.Modulus ||
		PublicExponent != DefaultRSAKey.PublicExponent ||
		PrivateExponent != DefaultRSAKey.PrivateExponent {
		t.Error("legacy package globals must alias DefaultRSAKey fields")
	}
}
