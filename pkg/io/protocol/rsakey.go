package protocol

import "math/big"

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
	Modulus = modulus

	publicExponent, ok := new(big.Int).SetString("81f390b2cf8ca7039ee507975951d5a0b15a87bf8b3f99c966834118c50fd94d", 16)
	if !ok {
		panic("bad public exponent")
	}
	PublicExponent = publicExponent

	privateExponent, ok := new(big.Int).SetString("571fb062048b61721ebfcf1e877153241b70c3aa26edb0f9f06a1b2be07c4e45eaba4fc356ea806cbed298d38613590a53fde0383c3a411758516293240925e5", 16)
	if !ok {
		panic("bad private exponent")
	}
	PrivateExponent = privateExponent
}
