// Package pemtoken ports src/io/PemUtil.ts (Engine-TS 9aadcec4):
// the per-deployment public token = sha1(rsa.N hex + rsa.E hex + hostname).
// The consumer (web layer) supplies the PEM bytes and hostname.
//
// NOTE: sha1 is used because it is the TS contract (non-security token,
// browser-visible identifier). Do NOT switch hashes.
package pemtoken

import (
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // TS contract: non-security deployment token
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"strconv"
)

// Token mirrors PemUtil.ts:10-21. n and e are lowercase hex without
// leading zeros (forge BigInteger.toString(16) semantics — big.Int.Text(16)
// matches for positive values).
func Token(pubPEM []byte, hostname string) (string, error) {
	block, _ := pem.Decode(pubPEM)
	if block == nil {
		return "", errors.New("pemtoken: no PEM block")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("pemtoken: %w", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return "", errors.New("pemtoken: not an RSA public key")
	}
	h := sha1.New() //nolint:gosec // TS contract: non-security deployment token
	h.Write([]byte(rsaPub.N.Text(16)))
	h.Write([]byte(strconv.FormatInt(int64(rsaPub.E), 16)))
	h.Write([]byte(hostname))
	return hex.EncodeToString(h.Sum(nil)), nil
}
