package pemtoken

import "testing"

// TS PemUtil.ts:10-21: sha1 over n-hex + e-hex + hostname, hex-encoded.
// Fixture: 512-bit RSA key (test-only, committed).
const testPubPEM = `-----BEGIN PUBLIC KEY-----
MFwwDQYJKoZIhvcNAQEBBQADSwAwSAJBAMD4gBEo1ChD4uMpgWEkRgco2A5UouBI
vnvgytcThUAgXeRnq5/zxZ6Bj+h/m5XsgR1OrNRxdaKpDFk2+q25uZkCAwEAAQ==
-----END PUBLIC KEY-----`

func TestTokenDeterministic(t *testing.T) {
	tok1, err := Token([]byte(testPubPEM), "hosta")
	if err != nil {
		t.Fatal(err)
	}
	tok2, _ := Token([]byte(testPubPEM), "hosta")
	if tok1 != tok2 {
		t.Fatal("token not deterministic")
	}
	if len(tok1) != 40 {
		t.Fatalf("token length = %d, want 40 (sha1 hex)", len(tok1))
	}
	tok3, _ := Token([]byte(testPubPEM), "hostb")
	if tok1 == tok3 {
		t.Fatal("token must vary by hostname")
	}
}

func TestTokenBadPEM(t *testing.T) {
	if _, err := Token([]byte("not a pem"), "h"); err == nil {
		t.Fatal("want error for invalid PEM")
	}
}

// TestTokenKnownValue pins the sha1 of (n-hex + e-hex + "fixedhost") for the
// fixture key above.
//
// Derivation:
//
//	N_HEX=$(openssl rsa -pubin -noout -modulus <fixture.pem 2>/dev/null | sed 's/Modulus=//' | tr 'A-F' 'a-f')
//	# => c0f8801128d42843e2e329816124460728d80e54a2e048be7be0cad7138540205de467ab9ff3c59e818fe87f9b95ec811d4eacd47175a2a90c5936faadb9b999
//	E_HEX="10001"  # 65537 in hex (forge BigInteger.toString(16))
//	printf '%s%s%s' "$N_HEX" "$E_HEX" "fixedhost" | sha1sum
//	# => c48fe73d16b53cc7c59a1eb76d2a84e4b150cb41
func TestTokenKnownValue(t *testing.T) {
	const wantToken = `c48fe73d16b53cc7c59a1eb76d2a84e4b150cb41`
	got, err := Token([]byte(testPubPEM), "fixedhost")
	if err != nil {
		t.Fatal(err)
	}
	if got != wantToken {
		t.Fatalf("Token = %s, want %s", got, wantToken)
	}
}
