package account

import "log/slog"

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
