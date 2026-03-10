package notify

import (
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strings"
)

// SMTPNotifier sends notifications via SMTP email.
type SMTPNotifier struct {
	host     string
	port     string
	username string
	password string
	from     string
	to       []string
}

// NewSMTPNotifier creates a new SMTP notifier from config.
func NewSMTPNotifier(config map[string]string) (*SMTPNotifier, error) {
	host := config["host"]
	port := config["port"]
	from := config["from"]
	to := config["to"]

	if host == "" {
		return nil, fmt.Errorf("SMTP host is required")
	}
	if port == "" {
		port = "587"
	}
	if from == "" {
		return nil, fmt.Errorf("SMTP from address is required")
	}
	if to == "" {
		return nil, fmt.Errorf("SMTP to address is required")
	}

	recipients := strings.Split(to, ",")
	for i := range recipients {
		recipients[i] = strings.TrimSpace(recipients[i])
	}

	return &SMTPNotifier{
		host:     host,
		port:     port,
		username: config["username"],
		password: config["password"],
		from:     from,
		to:       recipients,
	}, nil
}

// Send sends a build notification email via SMTP.
func (n *SMTPNotifier) Send(_ context.Context, event BuildEvent) error {
	subject := fmt.Sprintf("[FeatherCI] %s Build #%d %s",
		event.ProjectName, event.BuildNumber, strings.ToUpper(event.Status[:1])+event.Status[1:])

	body, err := renderEmailHTML(event)
	if err != nil {
		return fmt.Errorf("rendering email: %w", err)
	}

	toHeader := strings.Join(n.to, ", ")
	msg := "From: " + n.from + "\r\n" +
		"To: " + toHeader + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n" +
		"\r\n" +
		body

	addr := net.JoinHostPort(n.host, n.port)

	var auth smtp.Auth
	if n.username != "" {
		auth = smtp.PlainAuth("", n.username, n.password, n.host)
	}

	if err := smtp.SendMail(addr, auth, n.from, n.to, []byte(msg)); err != nil {
		return fmt.Errorf("sending email: %w", err)
	}
	return nil
}
