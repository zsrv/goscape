package protocol

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"os"
)

// ParseRSAKeyPEM decodes a PEM-encoded RSA private key — PKCS#1
// ("RSA PRIVATE KEY") or PKCS#8 ("PRIVATE KEY") — into an RSAKey. It mirrors
// Engine-TS forge.pki.privateKeyFromPem(...) at World.ts:104.
func ParseRSAKeyPEM(pemBytes []byte) (*RSAKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("protocol: no PEM block found")
	}

	var priv *rsa.PrivateKey
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		priv = k
	} else {
		k8, err8 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err8 != nil {
			return nil, fmt.Errorf("protocol: parse RSA private key: %w", err8)
		}
		rk, ok := k8.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("protocol: PEM is not an RSA private key")
		}
		priv = rk
	}

	return &RSAKey{
		Modulus:         priv.N,
		PublicExponent:  big.NewInt(int64(priv.E)),
		PrivateExponent: priv.D,
	}, nil
}

// LoadRSAKeyPEM reads path and parses it via ParseRSAKeyPEM.
func LoadRSAKeyPEM(path string) (*RSAKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("protocol: read RSA private key: %w", err)
	}
	return ParseRSAKeyPEM(b)
}
