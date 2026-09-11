package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/mail"
	"strings"
	"time"
)

var (
	ErrInvalidEmail        = errors.New("invalid email")
	ErrInvalidCode         = errors.New("invalid or expired code")
	ErrSessionNotFound     = errors.New("session not found")
	ErrDeliveryUnavailable = errors.New("email delivery unavailable")
)

type User struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
}

type Challenge struct {
	ID         string
	Email      string
	CodeHash   string
	Attempts   int
	ExpiresAt  time.Time
	ConsumedAt *time.Time
	CreatedAt  time.Time
}

type SessionRecord struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type Store interface {
	CreateChallenge(context.Context, Challenge) error
	DeleteChallenge(context.Context, string) error
	GetChallenge(context.Context, string) (Challenge, error)
	RecordFailedAttempt(context.Context, string) error
	ConsumeChallenge(context.Context, string, string, string, SessionRecord, time.Time) (User, error)
	GetSession(context.Context, string, time.Time) (User, error)
	DeleteSession(context.Context, string) error
}

type Sender interface {
	SendCode(context.Context, string, string) error
}

type Config struct {
	CodeTTL     time.Duration
	SessionTTL  time.Duration
	MaxAttempts int
	Now         func() time.Time
}

type Service struct {
	store       Store
	sender      Sender
	secret      []byte
	codeTTL     time.Duration
	sessionTTL  time.Duration
	maxAttempts int
	now         func() time.Time
}

type VerifyResult struct {
	User      User
	Token     string
	ExpiresAt time.Time
}

func NewService(store Store, sender Sender, secret []byte, config Config) (*Service, error) {
	if store == nil {
		return nil, errors.New("auth: store is required")
	}
	if sender == nil {
		return nil, errors.New("auth: sender is required")
	}
	if len(secret) < 32 {
		return nil, errors.New("auth: secret must be at least 32 bytes")
	}
	if config.CodeTTL <= 0 {
		config.CodeTTL = 10 * time.Minute
	}
	if config.SessionTTL <= 0 {
		config.SessionTTL = 30 * 24 * time.Hour
	}
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 5
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Service{
		store:       store,
		sender:      sender,
		secret:      append([]byte(nil), secret...),
		codeTTL:     config.CodeTTL,
		sessionTTL:  config.SessionTTL,
		maxAttempts: config.MaxAttempts,
		now:         config.Now,
	}, nil
}

func (s *Service) RequestCode(ctx context.Context, rawEmail string) (string, error) {
	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return "", err
	}
	code, err := generateCode()
	if err != nil {
		return "", fmt.Errorf("generate verification code: %w", err)
	}
	id, err := randomID("ach_", 18)
	if err != nil {
		return "", fmt.Errorf("generate challenge id: %w", err)
	}
	now := s.now().UTC()
	challenge := Challenge{
		ID:        id,
		Email:     email,
		CodeHash:  s.hashCode(id, code),
		ExpiresAt: now.Add(s.codeTTL),
		CreatedAt: now,
	}
	if err := s.store.CreateChallenge(ctx, challenge); err != nil {
		return "", err
	}
	if err := s.sender.SendCode(ctx, email, code); err != nil {
		_ = s.store.DeleteChallenge(ctx, id)
		if errors.Is(err, ErrDeliveryUnavailable) {
			return "", err
		}
		return "", fmt.Errorf("send verification code: %w", err)
	}
	return id, nil
}

func (s *Service) VerifyCode(ctx context.Context, challengeID, rawEmail, code string) (VerifyResult, error) {
	email, err := NormalizeEmail(rawEmail)
	if err != nil {
		return VerifyResult{}, ErrInvalidCode
	}
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return VerifyResult{}, ErrInvalidCode
	}
	challenge, err := s.store.GetChallenge(ctx, strings.TrimSpace(challengeID))
	if err != nil {
		return VerifyResult{}, ErrInvalidCode
	}
	now := s.now().UTC()
	if challenge.Email != email || challenge.ConsumedAt != nil || !challenge.ExpiresAt.After(now) || challenge.Attempts >= s.maxAttempts {
		return VerifyResult{}, ErrInvalidCode
	}
	candidate := s.hashCode(challenge.ID, code)
	if !hmac.Equal([]byte(candidate), []byte(challenge.CodeHash)) {
		_ = s.store.RecordFailedAttempt(ctx, challenge.ID)
		return VerifyResult{}, ErrInvalidCode
	}

	userID, err := randomID("usr_", 18)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("generate user id: %w", err)
	}
	sessionID, err := randomID("ses_", 18)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("generate session id: %w", err)
	}
	token, err := randomToken(32)
	if err != nil {
		return VerifyResult{}, fmt.Errorf("generate session token: %w", err)
	}
	expiresAt := now.Add(s.sessionTTL)
	session := SessionRecord{
		ID:        sessionID,
		TokenHash: s.hashToken(token),
		ExpiresAt: expiresAt,
		CreatedAt: now,
	}
	user, err := s.store.ConsumeChallenge(ctx, challenge.ID, email, userID, session, now)
	if err != nil {
		if errors.Is(err, ErrInvalidCode) {
			return VerifyResult{}, ErrInvalidCode
		}
		return VerifyResult{}, err
	}
	return VerifyResult{User: user, Token: token, ExpiresAt: expiresAt}, nil
}

func (s *Service) Session(ctx context.Context, token string) (User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return User{}, ErrSessionNotFound
	}
	return s.store.GetSession(ctx, s.hashToken(token), s.now().UTC())
}

func (s *Service) Logout(ctx context.Context, token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	return s.store.DeleteSession(ctx, s.hashToken(token))
}

func NormalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || len(email) > 320 {
		return "", ErrInvalidEmail
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || strings.ToLower(parsed.Address) != email || strings.ContainsAny(email, " <>\t\r\n") {
		return "", ErrInvalidEmail
	}
	return email, nil
}

func (s *Service) hashCode(challengeID, code string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte("code\x00" + challengeID + "\x00" + code))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Service) hashToken(token string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte("session\x00" + token))
	return hex.EncodeToString(mac.Sum(nil))
}

func randomID(prefix string, bytes int) (string, error) {
	value, err := randomToken(bytes)
	if err != nil {
		return "", err
	}
	return prefix + value, nil
}

func randomToken(bytes int) (string, error) {
	buffer := make([]byte, bytes)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func generateCode() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", value.Int64()), nil
}
