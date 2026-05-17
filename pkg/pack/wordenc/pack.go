// Package wordenc ports the TS client-wordenc Jagfile packer
// (tools/pack/chat/pack.ts) to Go.
package wordenc

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/zsrv/goscape/pkg/io/jagfile"
	"github.com/zsrv/goscape/pkg/io/packet"
	"github.com/zsrv/goscape/pkg/pack"
)

// Pack ports TS tools/pack/chat/pack.ts:packClientWordenc.
//
// Reads 4 ASCII text files from <srcDir>/wordenc/ (badenc.txt,
// fragmentsenc.txt, tldlist.txt, domainenc.txt), packs each into its
// own Packet, bundles into a Jagfile, saves to <outDir>/client/wordenc.
//
// No-ops when source dir missing (NAI-192-D-NO-SRC-NO-OP mirror) or
// when ShouldBuildFileAny determines no source file is newer than dest.
func Pack(srcDir, outDir string) error {
	wordencSrc := filepath.Join(srcDir, "wordenc")
	clientOut := filepath.Join(outDir, "client", "wordenc")

	// NAI-192-D-NO-SRC-NO-OP mirror: src dir absent → no-op cleanly.
	if _, err := os.Stat(wordencSrc); os.IsNotExist(err) {
		return nil
	}

	if !pack.ShouldBuildFileAny(wordencSrc, clientOut) {
		return nil
	}

	jag := jagfile.NewEmptyJagfile(false)

	if err := packBadenc(wordencSrc, jag); err != nil {
		return err
	}
	if err := packFragmentsenc(wordencSrc, jag); err != nil {
		return err
	}
	if err := packTldlist(wordencSrc, jag); err != nil {
		return err
	}
	if err := packDomainenc(wordencSrc, jag); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(clientOut), 0o755); err != nil {
		return err
	}
	return jag.Save(clientOut)
}

func readLines(path string) ([]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("wordenc: read %q: %w", path, err)
	}
	out := []string{}
	for line := range strings.SplitSeq(strings.ReplaceAll(string(raw), "\r", ""), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out, nil
}

func packBadenc(srcDir string, jag *jagfile.Jagfile) error {
	lines, err := readLines(filepath.Join(srcDir, "badenc.txt"))
	if err != nil {
		return err
	}
	out := packet.Alloc(2)
	out.P4(uint32(len(lines)))
	for _, line := range lines {
		fields := strings.Split(line, " ")
		word := fields[0]
		out.P1(uint8(len(word)))
		for j := range len(word) {
			out.P1(word[j])
		}
		combos := fields[1:]
		out.P1(uint8(len(combos)))
		for _, c := range combos {
			ab := strings.SplitN(c, ":", 2)
			a, _ := strconv.Atoi(ab[0])
			b, _ := strconv.Atoi(ab[1])
			out.P1(uint8(a))
			out.P1(uint8(b))
		}
	}
	jag.Write("badenc.txt", out)
	return nil
}

func packFragmentsenc(srcDir string, jag *jagfile.Jagfile) error {
	lines, err := readLines(filepath.Join(srcDir, "fragmentsenc.txt"))
	if err != nil {
		return err
	}
	out := packet.Alloc(2)
	out.P4(uint32(len(lines)))
	for _, line := range lines {
		n, _ := strconv.Atoi(line)
		out.P2(uint16(n))
	}
	jag.Write("fragmentsenc.txt", out)
	return nil
}

func packTldlist(srcDir string, jag *jagfile.Jagfile) error {
	lines, err := readLines(filepath.Join(srcDir, "tldlist.txt"))
	if err != nil {
		return err
	}
	out := packet.Alloc(2)
	out.P4(uint32(len(lines)))
	for _, line := range lines {
		parts := strings.SplitN(line, " ", 2)
		tld := parts[0]
		typ, _ := strconv.Atoi(parts[1])
		out.P1(uint8(typ))
		out.P1(uint8(len(tld)))
		for j := range len(tld) {
			out.P1(tld[j])
		}
	}
	jag.Write("tldlist.txt", out)
	return nil
}

func packDomainenc(srcDir string, jag *jagfile.Jagfile) error {
	lines, err := readLines(filepath.Join(srcDir, "domainenc.txt"))
	if err != nil {
		return err
	}
	out := packet.Alloc(2)
	out.P4(uint32(len(lines)))
	for _, line := range lines {
		out.P1(uint8(len(line)))
		for j := range len(line) {
			out.P1(line[j])
		}
	}
	jag.Write("domainenc.txt", out)
	return nil
}
