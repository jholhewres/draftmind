package auth

import (
	"bufio"
	"context"
	"io"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"strings"
	"testing"
	"time"
)

// smtpSession is what a fake SMTP server received in one session.
type smtpSession struct {
	from, rcpt string
	data       string
}

// fakeSMTP accepts one session, without TLS or auth, and sends what it got on the channel.
func fakeSMTP(t *testing.T) (string, <-chan smtpSession) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	got := make(chan smtpSession, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
		r, w := bufio.NewReader(conn), conn
		reply := func(s string) { _, _ = io.WriteString(w, s+"\r\n") }
		var s smtpSession
		reply("220 fake ESMTP")
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			cmd := strings.ToUpper(strings.TrimSpace(line))
			switch {
			case strings.HasPrefix(cmd, "EHLO"):
				reply("250-fake")
				reply("250 8BITMIME")
			case strings.HasPrefix(cmd, "MAIL FROM:"):
				s.from, _, _ = strings.Cut(strings.TrimSpace(line)[len("MAIL FROM:"):], " ") // without BODY=8BITMIME
				reply("250 ok")
			case strings.HasPrefix(cmd, "RCPT TO:"):
				s.rcpt = strings.TrimSpace(line)[len("RCPT TO:"):]
				reply("250 ok")
			case cmd == "DATA":
				reply("354 go on")
				var b strings.Builder
				for {
					l, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if l == ".\r\n" {
						break
					}
					b.WriteString(l)
				}
				s.data = b.String()
				reply("250 queued")
			case cmd == "QUIT":
				reply("221 bye")
				got <- s
				return
			default:
				reply("250 ok")
			}
		}
	}()
	return ln.Addr().String(), got
}

func TestSMTPSendsTheSignInLink(t *testing.T) {
	addr, got := fakeSMTP(t)
	m := SMTP{Addr: addr, From: "Draftmind <signin@draftmind.test>"}
	link := publicURL + "/#/signin/link/abc_DEF-123"
	if err := m.Send(context.Background(), linkMessage("pt-BR", "player@example.com", link)); err != nil {
		t.Fatal(err)
	}
	var s smtpSession
	select {
	case s = <-got:
	case <-time.After(5 * time.Second):
		t.Fatal("the SMTP server got no session")
	}
	if s.from != "<signin@draftmind.test>" || s.rcpt != "<player@example.com>" {
		t.Fatalf("envelope from %q to %q", s.from, s.rcpt)
	}
	msg, err := mail.ReadMessage(strings.NewReader(s.data))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Header.Get("To") != "player@example.com" || msg.Header.Get("From") != "Draftmind <signin@draftmind.test>" ||
		msg.Header.Get("Subject") != "Seu link de acesso ao Draftmind" || msg.Header.Get("Date") == "" ||
		!strings.HasPrefix(msg.Header.Get("Content-Type"), "text/plain; charset=UTF-8") {
		t.Fatalf("headers %v", msg.Header)
	}
	body, err := io.ReadAll(quotedprintable.NewReader(msg.Body))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), link) || !strings.Contains(string(body), "você") {
		t.Fatalf("body %q", body)
	}
}

func TestLinkMessageFollowsTheLanguage(t *testing.T) {
	cases := map[string]string{
		"pt-BR": "Seu link de acesso ao Draftmind",
		"pt":    "Seu link de acesso ao Draftmind",
		"en-GB": "Your Draftmind sign-in link",
		"de":    "Your Draftmind sign-in link",
		"":      "Your Draftmind sign-in link",
	}
	for lang, want := range cases {
		m := linkMessage(lang, "player@example.com", "https://x/#/signin/link/t")
		if m.Subject != want || !strings.Contains(m.Body, "https://x/#/signin/link/t") || !strings.Contains(m.Body, "15") {
			t.Errorf("lang %q: %+v", lang, m)
		}
	}
}
