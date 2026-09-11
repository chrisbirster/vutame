package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNewSMTPSenderValidatesConfiguration(t *testing.T) {
	if _, err := NewSMTPSender(SMTPConfig{}); err == nil {
		t.Fatal("empty SMTP config should fail")
	}
	if _, err := NewSMTPSender(SMTPConfig{Address: "smtp.example.com", From: "login@example.com"}); err == nil {
		t.Fatal("SMTP address without port should fail")
	}
	if _, err := NewSMTPSender(SMTPConfig{Address: "smtp.example.com:587", From: "bad\r\nBcc: attacker@example.com"}); err == nil {
		t.Fatal("header-injection from address should fail")
	}
	if _, err := NewSMTPSender(SMTPConfig{Address: "smtp.example.com:587", From: "login@example.com", Username: "user"}); err == nil {
		t.Fatal("partial SMTP credentials should fail")
	}
}

func TestSMTPSenderBuildsOneTimeCodeMessage(t *testing.T) {
	sender, err := NewSMTPSender(SMTPConfig{
		Address:  "smtp.example.com:587",
		Username: "user",
		Password: "secret",
		From:     "Vutame <login@example.com>",
	})
	if err != nil {
		t.Fatal(err)
	}
	message := sender.message("creator@example.com", "123456")
	for _, want := range []string{
		"From: \"Vutame\" <login@example.com>",
		"To: creator@example.com",
		"Subject: Your Vutame sign-in code",
		"Your Vutame sign-in code is: 123456",
		"can only be used once",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("message missing %q: %q", want, message)
		}
	}
	if got := strings.Count(message, "123456"); got != 1 {
		t.Fatalf("code appears %d times, want 1", got)
	}
}

func TestSMTPSenderRejectsInvalidRecipientAndCodeBeforeDial(t *testing.T) {
	sender, err := NewSMTPSender(SMTPConfig{Address: "smtp.example.com:587", From: "login@example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if err := sender.SendCode(context.Background(), "not-an-email", "123456"); !errors.Is(err, ErrInvalidEmail) {
		t.Fatalf("invalid recipient err = %v, want ErrInvalidEmail", err)
	}
	if err := sender.SendCode(context.Background(), "creator@example.com", "secret"); err == nil || !strings.Contains(err.Error(), "six digits") {
		t.Fatalf("invalid code err = %v", err)
	}
}
