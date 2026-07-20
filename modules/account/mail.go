package account

import (
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
)

// Mailer sends portal email (verification, password reset). The SMTP
// implementation lands with the email flows (Task 16); logMailer is
// the no-SMTP fallback so a portal without smtp.host still functions —
// links are visible in the server log.
type Mailer interface {
	Send(to, subject, body string) error
}

type logMailer struct{ log *slog.Logger }

func newLogMailer(log *slog.Logger) Mailer { return &logMailer{log: log} }

func (m *logMailer) Send(to, subject, body string) error {
	m.log.Info("outbound mail (smtp.host not configured — logging instead)",
		slog.String("to", to), slog.String("subject", subject), slog.String("body", body))
	return nil
}

// buildMailMessage assembles a minimal RFC-5322 text/plain message.
func buildMailMessage(from, to, subject, body string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("\r\n\r\n")
	b.WriteString(body)
	return []byte(b.String())
}

// smtpMailer sends through a single relay. net/smtp negotiates
// STARTTLS automatically when the server advertises it; PlainAuth is
// used only when a username is configured.
type smtpMailer struct{ cfg SMTPConfig }

func newSMTPMailer(cfg SMTPConfig) Mailer { return &smtpMailer{cfg: cfg} }

func (m *smtpMailer) Send(to, subject, body string) error {
	addr := fmt.Sprintf("%s:%d", m.cfg.Host, m.cfg.Port)
	var auth smtp.Auth
	if m.cfg.Username != "" {
		auth = smtp.PlainAuth("", m.cfg.Username, m.cfg.Password, m.cfg.Host)
	}
	return smtp.SendMail(addr, auth, m.cfg.From, []string{to}, buildMailMessage(m.cfg.From, to, subject, body))
}
