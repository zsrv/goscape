package account

import (
	"strings"
	"testing"
)

func TestBuildMailMessage(t *testing.T) {
	msg := string(buildMailMessage("noreply@x", "player@y", "Subject line", "Line1\r\nLine2\r\n"))
	for _, want := range []string{
		"From: noreply@x\r\n", "To: player@y\r\n", "Subject: Subject line\r\n",
		"MIME-Version: 1.0\r\n", "Content-Type: text/plain; charset=utf-8\r\n",
		"\r\n\r\nLine1\r\n",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
}
