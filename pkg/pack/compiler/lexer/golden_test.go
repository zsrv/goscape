package lexer

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestLex_Golden_NeptuneScript runs the lexer over testdata/golden_script.src
// and compares the default-channel token sequence to
// testdata/golden_script.tokens. On mismatch, prints both sequences.
func TestLex_Golden_NeptuneScript(t *testing.T) {
	src, err := os.ReadFile("testdata/golden_script.src")
	if err != nil {
		t.Fatalf("read src: %v", err)
	}

	wantFile, err := os.Open("testdata/golden_script.tokens")
	if err != nil {
		t.Fatalf("read tokens: %v", err)
	}
	defer wantFile.Close()

	var want []string
	sc := bufio.NewScanner(wantFile)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "" {
			continue
		}
		want = append(want, line)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan tokens: %v", err)
	}

	ts := NewTokenStream(NewLexer(string(src), "golden_script.src"))
	var got []string
	for {
		tok := ts.LT(1)
		got = append(got, fmt.Sprintf("%s:%s", tok.Type, strconv.Quote(tok.Text)))
		if tok.Type == EOF {
			break
		}
		ts.Consume()
	}

	if len(got) != len(want) {
		t.Errorf("token count: got %d, want %d", len(got), len(want))
	}
	for i := 0; i < len(want) && i < len(got); i++ {
		if got[i] != want[i] {
			t.Errorf("token %d: got %s, want %s", i, got[i], want[i])
		}
	}
	if t.Failed() {
		t.Logf("full GOT sequence:\n%s", strings.Join(got, "\n"))
		t.Logf("full WANT sequence:\n%s", strings.Join(want, "\n"))
	}
}
