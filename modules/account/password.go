package account

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// SentinelGamePassword is stored in the game `account.password` column
// for rows created by portal character creation. It is not a valid
// bcrypt hash, so if a deployment is ever flipped back to
// login.auth_mode=local, bcrypt comparison always fails — portal
// characters cannot be logged into through the legacy path.
const SentinelGamePassword = "!portal-managed!"

const (
	saltLen = 16
	keyLen  = 32
)

// HashPassword derives an argon2id hash and encodes it as a PHC string:
// $argon2id$v=19$m=<KiB>,t=<time>,p=<lanes>$<b64salt>$<b64key>.
func HashPassword(password string, p Argon2Config) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("account: salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, uint32(p.Time), uint32(p.MemoryKiB), uint8(p.Parallelism), keyLen)
	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.MemoryKiB, p.Time, p.Parallelism, b64(salt), b64(key)), nil
}

// VerifyPassword re-derives the key with the parameters embedded in the
// PHC string and compares in constant time. A well-formed PHC string
// that doesn't match returns (false, nil); a malformed string returns
// (false, error).
func VerifyPassword(password, phc string) (bool, error) {
	parts := strings.Split(phc, "$")
	// ["", "argon2id", "v=19", "m=..,t=..,p=..", salt, key]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, fmt.Errorf("account: malformed password hash")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, fmt.Errorf("account: unsupported argon2 version %q", parts[2])
	}
	var m, t, p int
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false, fmt.Errorf("account: malformed argon2 params %q", parts[3])
	}
	if m < 1 || t < 1 || p < 1 {
		return false, fmt.Errorf("account: invalid argon2 params %q", parts[3])
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("account: malformed salt: %w", err)
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("account: malformed key: %w", err)
	}
	got := argon2.IDKey([]byte(password), salt, uint32(t), uint32(m), uint8(p), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// ValidPortalPassword enforces the portal password policy. The account
// password is also typed into the Java client's login screen, which
// caps passwords at 20 characters and cannot enter spaces or
// non-ASCII — hence the unusual upper bound.
func ValidPortalPassword(password string) error {
	if len(password) < 8 || len(password) > 20 {
		return fmt.Errorf("password must be 8-20 characters (the game client caps passwords at 20)")
	}
	for _, r := range password {
		if r <= ' ' || r > '~' {
			return fmt.Errorf("password may only contain printable ASCII characters (no spaces) so it can be typed in the game client")
		}
	}
	return nil
}
