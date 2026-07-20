package account

import (
	"strings"
	"testing"
)

func testArgon2() Argon2Config {
	// Small params for test speed; production defaults live in config.
	return Argon2Config{MemoryKiB: 8 * 1024, Time: 1, Parallelism: 1}
}

func TestHashVerifyRoundTrip(t *testing.T) {
	phc, err := HashPassword("Sw0rdfish!", testArgon2())
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(phc, "$argon2id$v=19$") {
		t.Fatalf("not a PHC argon2id string: %q", phc)
	}
	ok, err := VerifyPassword("Sw0rdfish!", phc)
	if err != nil || !ok {
		t.Fatalf("verify correct password: ok=%v err=%v", ok, err)
	}
	ok, err = VerifyPassword("sw0rdfish!", phc) // case matters
	if err != nil || ok {
		t.Fatalf("verify wrong-case password must fail: ok=%v err=%v", ok, err)
	}
}

func TestHashPassword_UniqueSalts(t *testing.T) {
	a, _ := HashPassword("same", testArgon2())
	b, _ := HashPassword("same", testArgon2())
	if a == b {
		t.Fatal("two hashes of the same password must differ (random salt)")
	}
}

func TestVerifyPassword_Malformed(t *testing.T) {
	for _, phc := range []string{"", "plaintext", SentinelGamePassword, "$argon2id$v=19$m=x$$"} {
		if ok, err := VerifyPassword("anything", phc); ok || err == nil {
			t.Errorf("VerifyPassword(_, %q) = %v, %v; want false, error", phc, ok, err)
		}
	}
}

func TestValidPortalPassword(t *testing.T) {
	cases := []struct {
		pw string
		ok bool
	}{
		{"abcd1234", true},
		{"exactly-twenty-chs20", true},
		{"short7!", false},                        // < 8
		{"this-is-way-longer-than-twenty", false}, // > 20 (client cap)
		{"has space", false},                      // space not client-typable in password field
		{"unicodé-pw", false},                     // non-ASCII
	}
	for _, tc := range cases {
		err := ValidPortalPassword(tc.pw)
		if (err == nil) != tc.ok {
			t.Errorf("ValidPortalPassword(%q) = %v, want ok=%v", tc.pw, err, tc.ok)
		}
	}
}
