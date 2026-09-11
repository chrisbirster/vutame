package auth

import (
	"context"
	"log/slog"
)

type UnavailableSender struct{}

func (UnavailableSender) SendCode(context.Context, string, string) error {
	return ErrDeliveryUnavailable
}

// LogSender is for local development only. It must never be enabled in hosted
// production because it writes the one-time code to application logs.
type LogSender struct{}

func (LogSender) SendCode(_ context.Context, email, code string) error {
	slog.Info("vutame development auth code", "email", email, "code", code)
	return nil
}
