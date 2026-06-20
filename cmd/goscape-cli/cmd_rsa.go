package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"

	"github.com/zsrv/goscape/pkg/io/protocol"
)

// wireLengthCapBytes is the largest RSA modulus (in bytes) whose ciphertext
// still fits the login block's 1-byte length prefix once Java's BigInteger
// sign byte is accounted for. packet.RSAEnc writes P1(len(ciphertext)); a
// modulus above this overflows that byte. See packet.RSAEnc / packet.RSADec.
const wireLengthCapBytes = 254

// runRSA dispatches the `rsa` verb's sub-verbs: `gen` and `info`.
func runRSA(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "Usage: goscape-cli rsa <gen|info> [flags]")
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "gen":
		return runRSAGen(rest, stdout, stderr)
	case "info":
		return runRSAInfo(rest, stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintln(stdout, "Usage: goscape-cli rsa <gen|info> [flags]")
		fmt.Fprintln(stdout, "  gen   Generate an RSA keypair (private.pem + public.pem).")
		fmt.Fprintln(stdout, "  info  Print N/E/D for a PEM key, or the built-in default key when no path is given.")
		return 0
	}
	fmt.Fprintf(stderr, "unknown rsa sub-verb: %q\n", sub)
	return 2
}

// runRSAGen generates an RSA keypair, writes private.pem (PKCS#1, forge/TS
// compatible) and public.pem (PKIX, ondemand pub_pem compatible), and prints
// the values needed to bake the matching public key into the Java client.
func runRSAGen(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("rsa gen", flag.ContinueOnError)
	fs.SetOutput(stderr)
	bits := fs.Int("bits", 1024, "RSA key size in bits (TS default 1024).")
	outDir := fs.String("out-dir", ".", "Directory to write private.pem and public.pem.")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	priv, err := rsa.GenerateKey(rand.Reader, *bits)
	if err != nil {
		fmt.Fprintf(stderr, "rsa gen: generate key: %v\n", err)
		return 1
	}

	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(priv),
	})
	pubDER, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		fmt.Fprintf(stderr, "rsa gen: marshal public key: %v\n", err)
		return 1
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "rsa gen: create out-dir %s: %v\n", *outDir, err)
		return 1
	}

	privPath := filepath.Join(*outDir, "private.pem")
	pubPath := filepath.Join(*outDir, "public.pem")
	if err := os.WriteFile(privPath, privPEM, 0o600); err != nil {
		fmt.Fprintf(stderr, "rsa gen: write %s: %v\n", privPath, err)
		return 1
	}
	if err := os.WriteFile(pubPath, pubPEM, 0o644); err != nil {
		fmt.Fprintf(stderr, "rsa gen: write %s: %v\n", pubPath, err)
		return 1
	}

	fmt.Fprintf(stdout, "wrote %s\nwrote %s\n\n", privPath, pubPath)
	printRSABakeValues(stdout, priv.N, big.NewInt(int64(priv.E)), priv.D)

	if (priv.N.BitLen()+7)/8 > wireLengthCapBytes {
		fmt.Fprintf(stderr, "\nwarning: a %d-bit modulus exceeds the login wire cap (~%d-bit); "+
			"the RSA login block length is a single byte and will overflow.\n",
			priv.N.BitLen(), wireLengthCapBytes*8)
	}
	return 0
}

// runRSAInfo prints the bake values for a PEM key. With no path it prints the
// built-in default key (pkg/io/protocol/rsakey.go).
func runRSAInfo(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("rsa info", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()

	if len(rest) == 0 {
		fmt.Fprintln(stdout, "Built-in default key (pkg/io/protocol/rsakey.go):")
		printRSABakeValues(stdout, protocol.DefaultRSAKey.Modulus,
			protocol.DefaultRSAKey.PublicExponent, protocol.DefaultRSAKey.PrivateExponent)
		return 0
	}

	path := rest[0]
	b, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "rsa info: read %s: %v\n", path, err)
		return 1
	}
	n, e, d, err := parseAnyRSAPEM(b)
	if err != nil {
		fmt.Fprintf(stderr, "rsa info: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "%s:\n", path)
	printRSABakeValues(stdout, n, e, d)
	return 0
}

// parseAnyRSAPEM decodes a PEM block as an RSA private key (PKCS#1 or PKCS#8)
// or an RSA public key (PKIX or PKCS#1). For a public key, d is nil.
func parseAnyRSAPEM(b []byte) (*big.Int, *big.Int, *big.Int, error) {
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, nil, nil, errors.New("no PEM block found")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k.N, big.NewInt(int64(k.E)), k.D, nil
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, nil, nil, errors.New("PEM is not an RSA key")
		}
		return rk.N, big.NewInt(int64(rk.E)), rk.D, nil
	}
	if k, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		pk, ok := k.(*rsa.PublicKey)
		if !ok {
			return nil, nil, nil, errors.New("PEM is not an RSA key")
		}
		return pk.N, big.NewInt(int64(pk.E)), nil, nil
	}
	if k, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return k.N, big.NewInt(int64(k.E)), nil, nil
	}
	return nil, nil, nil, errors.New("unrecognized PEM key format")
}

// printRSABakeValues prints N and E in decimal (Client.java LOGIN_RSAN /
// LOGIN_RSAE) and N/E/D in hex (pkg/io/protocol/rsakey.go). D is printed only
// when known (private key).
func printRSABakeValues(w io.Writer, n, e, d *big.Int) {
	fmt.Fprintf(w, "Modulus (N):\n")
	fmt.Fprintf(w, "  decimal (Client.java LOGIN_RSAN): %s\n", n.String())
	fmt.Fprintf(w, "  hex (rsakey.go):                  %s\n", n.Text(16))
	fmt.Fprintf(w, "Public exponent (E):\n")
	fmt.Fprintf(w, "  decimal (Client.java LOGIN_RSAE): %s\n", e.String())
	fmt.Fprintf(w, "  hex (rsakey.go):                  %s\n", e.Text(16))
	if d != nil {
		fmt.Fprintf(w, "Private exponent (D):\n")
		fmt.Fprintf(w, "  hex (rsakey.go): %s\n", d.Text(16))
	}
}
