package protocol

import "math/big"

// RSAKey holds the RSA key material for the login handshake: modulus N,
// public exponent E, and private exponent D. Only N and D are used at
// runtime (login decryption via Packet.RSADec); E is used by the test-only
// client-side RSAEnc path.
type RSAKey struct {
	Modulus         *big.Int // N
	PublicExponent  *big.Int // E
	PrivateExponent *big.Int // D
}

// DefaultRSAKey is the built-in 512-bit key compiled into the binary. It is
// used for login decryption unless the world module is configured with a
// custom key (world.rsa_private_key_path). The matching public key is baked
// into the Java client (Client.java LOGIN_RSAN / LOGIN_RSAE).
var DefaultRSAKey *RSAKey

// Modulus, PublicExponent, and PrivateExponent alias DefaultRSAKey's fields
// for existing callers (e.g. the client-side RSAEnc in login/req). New code
// should thread an *RSAKey explicitly instead of reading these globals.
var (
	Modulus         *big.Int // N
	PublicExponent  *big.Int // E
	PrivateExponent *big.Int // D
)

func init() {
	modulus, ok := new(big.Int).SetString("0088c38748a58228f7261cdc340b5691d7d0975dee0ecdb717609e6bf971eb3fe723ef9d130e4686813739768ad9472eb46d8bfcc042c1a5fcb05e931f632eea5d", 16)
	if !ok {
		panic("bad modulus")
	}

	publicExponent, ok := new(big.Int).SetString("81f390b2cf8ca7039ee507975951d5a0b15a87bf8b3f99c966834118c50fd94d", 16)
	if !ok {
		panic("bad public exponent")
	}

	privateExponent, ok := new(big.Int).SetString("571fb062048b61721ebfcf1e877153241b70c3aa26edb0f9f06a1b2be07c4e45eaba4fc356ea806cbed298d38613590a53fde0383c3a411758516293240925e5", 16)
	if !ok {
		panic("bad private exponent")
	}

	DefaultRSAKey = &RSAKey{
		Modulus:         modulus,
		PublicExponent:  publicExponent,
		PrivateExponent: privateExponent,
	}
	Modulus = DefaultRSAKey.Modulus
	PublicExponent = DefaultRSAKey.PublicExponent
	PrivateExponent = DefaultRSAKey.PrivateExponent
}
