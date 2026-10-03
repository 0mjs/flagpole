// Package mail sends plain-text email over SMTP. Locally that's Mailpit, at
// http://localhost:8025.
package mail

import (
	"fmt"
	"net/smtp"
	"strings"
	"time"
)

type Sender struct {
	addr string
	from string
}

func New(addr, from string) *Sender { return &Sender{addr: addr, from: from} }

func (s *Sender) Send(to, subject, body string) error {
	msg := strings.Join([]string{
		"From: " + s.from,
		"To: " + to,
		"Subject: " + subject,
		"Date: " + time.Now().Format(time.RFC1123Z),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=utf-8",
		"",
		body,
	}, "\r\n")
	from := s.from
	if i := strings.Index(from, "<"); i >= 0 {
		from = strings.Trim(from[i:], "<>")
	}
	if err := smtp.SendMail(s.addr, nil, from, []string{to}, []byte(msg)); err != nil {
		return fmt.Errorf("mail: %w", err)
	}
	return nil
}
