package auth

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

const defaultSMTPTimeout = 15 * time.Second

type SMTPConfig struct {
	Address  string
	Username string
	Password string
	From     string
	Timeout  time.Duration
}

type SMTPSender struct {
	address  string
	host     string
	username string
	password string
	from     mail.Address
	timeout  time.Duration
}

func NewSMTPSender(config SMTPConfig) (*SMTPSender, error) {
	config.Address = strings.TrimSpace(config.Address)
	config.Username = strings.TrimSpace(config.Username)
	config.From = strings.TrimSpace(config.From)
	if config.Address == "" {
		return nil, errors.New("auth smtp: address is required")
	}
	host, _, err := net.SplitHostPort(config.Address)
	if err != nil || strings.TrimSpace(host) == "" {
		return nil, errors.New("auth smtp: address must be host:port")
	}
	if config.From == "" || strings.ContainsAny(config.From, "\r\n") {
		return nil, errors.New("auth smtp: from address is required")
	}
	from, err := mail.ParseAddress(config.From)
	if err != nil || strings.TrimSpace(from.Address) == "" || strings.ContainsAny(from.Address, "\r\n") {
		return nil, errors.New("auth smtp: invalid from address")
	}
	if (config.Username == "") != (config.Password == "") {
		return nil, errors.New("auth smtp: username and password must be configured together")
	}
	if config.Timeout <= 0 {
		config.Timeout = defaultSMTPTimeout
	}
	return &SMTPSender{
		address:  config.Address,
		host:     host,
		username: config.Username,
		password: config.Password,
		from:     *from,
		timeout:  config.Timeout,
	}, nil
}

func (s *SMTPSender) SendCode(ctx context.Context, rawEmail, code string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return err
	}
	if len(code) != 6 {
		return errors.New("auth smtp: verification code must contain six digits")
	}
	for _, char := range code {
		if char < '0' || char > '9' {
			return errors.New("auth smtp: verification code must contain six digits")
		}
	}

	dialer := net.Dialer{Timeout: s.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", s.address)
	if err != nil {
		return fmt.Errorf("auth smtp: connect: %w", err)
	}
	defer conn.Close()

	deadline := time.Now().Add(s.timeout)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return fmt.Errorf("auth smtp: set deadline: %w", err)
	}

	client, err := smtp.NewClient(conn, s.host)
	if err != nil {
		return fmt.Errorf("auth smtp: create client: %w", err)
	}
	defer client.Close()

	if ok, _ := client.Extension("STARTTLS"); !ok {
		return errors.New("auth smtp: server does not advertise STARTTLS")
	}
	if err := client.StartTLS(&tls.Config{
		ServerName: s.host,
		MinVersion: tls.VersionTLS12,
	}); err != nil {
		return fmt.Errorf("auth smtp: start TLS: %w", err)
	}
	if s.username != "" {
		if err := client.Auth(smtp.PlainAuth("", s.username, s.password, s.host)); err != nil {
			return fmt.Errorf("auth smtp: authenticate: %w", err)
		}
	}
	if err := client.Mail(s.from.Address); err != nil {
		return fmt.Errorf("auth smtp: set sender: %w", err)
	}
	if err := client.Rcpt(email); err != nil {
		return fmt.Errorf("auth smtp: set recipient: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("auth smtp: open message: %w", err)
	}
	if _, err := io.WriteString(writer, s.message(email, code)); err != nil {
		_ = writer.Close()
		return fmt.Errorf("auth smtp: write message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("auth smtp: finish message: %w", err)
	}
	if err := client.Quit(); err != nil {
		return fmt.Errorf("auth smtp: quit: %w", err)
	}
	return nil
}

func (s *SMTPSender) message(email, code string) string {
	return "From: " + s.from.String() + "\r\n" +
		"To: " + email + "\r\n" +
		"Subject: Your Vutame sign-in code\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"Content-Transfer-Encoding: 7bit\r\n" +
		"Auto-Submitted: auto-generated\r\n" +
		"\r\n" +
		"Your Vutame sign-in code is: " + code + "\r\n\r\n" +
		"This code expires shortly and can only be used once.\r\n" +
		"If you did not request this code, you can ignore this email.\r\n"
}
