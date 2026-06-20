package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zsrv/goscape/pkg/io/protocol"
	"github.com/zsrv/goscape/pkg/util/pemtoken"
)

func TestRunRSAInfo_DefaultKey(t *testing.T) {
	var out, errb bytes.Buffer
	if code := runRSA([]string{"info"}, &out, &errb); code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errb.String())
	}
	s := out.String()
	// Built-in default public key must match the Java client constants
	// (Client.java:1290-1291). Guards against accidental key changes.
	const wantN = "7162900525229798032761816791230527296329313291232324290237849263501208207972894053929065636522363163621000728841182238772712427862772219676577293600221789"
	const wantE = "58778699976184461502525193738213253649000149147835990136706041084440742975821"
	if !strings.Contains(s, wantN) {
		t.Errorf("default key N decimal not found in output:\n%s", s)
	}
	if !strings.Contains(s, wantE) {
		t.Errorf("default key E decimal not found in output:\n%s", s)
	}
}

func TestRunRSAGen_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	var genOut, genErr bytes.Buffer
	if code := runRSA([]string{"gen", "--bits", "1024", "--out-dir", dir}, &genOut, &genErr); code != 0 {
		t.Fatalf("gen exit %d, stderr=%s", code, genErr.String())
	}
	privPath := filepath.Join(dir, "private.pem")
	pubPath := filepath.Join(dir, "public.pem")

	pemBytes, err := os.ReadFile(privPath)
	if err != nil {
		t.Fatalf("read private.pem: %v", err)
	}
	key, err := protocol.ParseRSAKeyPEM(pemBytes)
	if err != nil {
		t.Fatalf("ParseRSAKeyPEM: %v", err)
	}
	wantN := key.Modulus.String()
	if !strings.Contains(genOut.String(), wantN) {
		t.Errorf("gen output missing modulus N decimal")
	}

	var infoOut, infoErr bytes.Buffer
	if code := runRSA([]string{"info", privPath}, &infoOut, &infoErr); code != 0 {
		t.Fatalf("info exit %d, stderr=%s", code, infoErr.String())
	}
	if !strings.Contains(infoOut.String(), wantN) {
		t.Errorf("info output missing modulus N decimal")
	}

	// public.pem must be usable as the ondemand pub_pem (PKIX RSA public key).
	pubBytes, err := os.ReadFile(pubPath)
	if err != nil {
		t.Fatalf("read public.pem: %v", err)
	}
	if _, err := pemtoken.Token(pubBytes, "host"); err != nil {
		t.Errorf("public.pem not usable as ondemand pub_pem: %v", err)
	}
}

func TestRunRSAGen_CreatesMissingOutDir(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "a", "b")
	var genOut, genErr bytes.Buffer
	if code := runRSA([]string{"gen", "--bits", "1024", "--out-dir", outDir}, &genOut, &genErr); code != 0 {
		t.Fatalf("gen exit %d, stderr=%s", code, genErr.String())
	}
	privPath := filepath.Join(outDir, "private.pem")
	pubPath := filepath.Join(outDir, "public.pem")
	if _, err := os.Stat(privPath); err != nil {
		t.Errorf("private.pem not created: %v", err)
	}
	if _, err := os.Stat(pubPath); err != nil {
		t.Errorf("public.pem not created: %v", err)
	}
}

func TestRunRSA_UnknownSubVerb(t *testing.T) {
	var out, errb bytes.Buffer
	if code := runRSA([]string{"bogus"}, &out, &errb); code != 2 {
		t.Errorf("expected exit 2 for unknown sub-verb, got %d", code)
	}
}
