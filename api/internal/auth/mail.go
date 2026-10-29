package auth

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

// linkTemplates are the sign-in e-mail per language: subject, then body
// with {link} and {minutes}.
var linkTemplates = map[string][2]string{
	"en": {"Your Draftmind sign-in link",
		"Open this link to sign in to Draftmind:\n\n{link}\n\nIt works once, for {minutes} minutes. If you did not ask for it, ignore this e-mail.\n"},
	"pt-BR": {"Seu link de acesso ao Draftmind",
		"Abra este link para entrar no Draftmind:\n\n{link}\n\nEle funciona uma vez, por {minutes} minutos. Se você não pediu, ignore este e-mail.\n"},
}

// linkMessage is the sign-in e-mail in lang, by exact tag, then by primary
// language, else English.
func linkMessage(lang, to, link string) Message {
	tpl, ok := linkTemplates[lang]
	if !ok {
		primary, _, _ := strings.Cut(strings.ToLower(lang), "-")
		tpl = linkTemplates["en"]
		for tag, t := range linkTemplates {
			if p, _, _ := strings.Cut(strings.ToLower(tag), "-"); p == primary {
				tpl = t
				break
			}
		}
	}
	body := strings.NewReplacer("{link}", link, "{minutes}", strconv.Itoa(int(LinkTTL.Minutes()))).Replace(tpl[1])
	return Message{To: to, Subject: tpl[0], Body: body, Link: link}
}

// logMailer writes the sign-in link to the log instead of sending it:
// development without SMTP. It logs the link alone.
type logMailer struct{ log *slog.Logger }

func (l logMailer) Send(_ context.Context, m Message) error {
	l.log.Info("auth: sign-in link (SMTP is not set up)", "link", m.Link)
	return nil
}

// WithLogMailer turns e-mail sign-in on with the links written to the log.
// Only in development: a link signs in, and production logs must never
// hold one.
func (s *Service) WithLogMailer() error {
	if !s.dev {
		return errors.New("sign-in links go to the log only when PUBLIC_URL is this machine")
	}
	s.mailer = logMailer{log: s.log}
	return nil
}

// SMTP sends e-mail through one server, with STARTTLS when it offers it.
type SMTP struct {
	Addr               string // host:port
	Username, Password string // empty: no auth
	From               string // e.g. Draftmind <signin@example.com>
}

// Send delivers m, giving up after 15 s or when ctx ends.
func (s SMTP) Send(ctx context.Context, m Message) error {
	from, err := mail.ParseAddress(s.From)
	if err != nil {
		return fmt.Errorf("MAIL_FROM: %w", err)
	}
	host, _, err := net.SplitHostPort(s.Addr)
	if err != nil {
		return fmt.Errorf("SMTP_HOST and SMTP_PORT: %w", err)
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", s.Addr)
	if err != nil {
		return err
	}
	deadline, _ := ctx.Deadline()
	_ = conn.SetDeadline(deadline)
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer func() { _ = c.Close() }()
	if ok, _ := c.Extension("STARTTLS"); ok {
		if err := c.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return err
		}
	}
	if s.Username != "" {
		// PlainAuth refuses to send the password unencrypted, except to localhost.
		if err := c.Auth(smtp.PlainAuth("", s.Username, s.Password, host)); err != nil {
			return err
		}
	}
	if err := c.Mail(from.Address); err != nil {
		return err
	}
	if err := c.Rcpt(m.To); err != nil {
		return err
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(s.build(m)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}

// build writes the message: plain UTF-8 text, quoted-printable. To was
// parsed as an address, so it holds no line break.
func (s SMTP) build(m Message) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\n", s.From, m.To,
		mime.QEncoding.Encode("utf-8", m.Subject), time.Now().Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n")
	qp := quotedprintable.NewWriter(&b)
	_, _ = qp.Write([]byte(strings.ReplaceAll(m.Body, "\n", "\r\n")))
	_ = qp.Close()
	return b.Bytes()
}
