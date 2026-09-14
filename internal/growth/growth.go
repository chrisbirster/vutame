package growth

import (
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"time"
)

var (
	ErrProfileRequired = errors.New("growth profile required")
	ErrInvalidBlock    = errors.New("invalid contact block")
	ErrContactDisabled = errors.New("contact capture disabled")
	ErrConsentRequired = errors.New("contact consent required")
	ErrInvalidEmail    = errors.New("invalid contact email")
	ErrInvalidSettings = errors.New("invalid data retention settings")
)

const (
	DefaultAnalyticsRetentionDays = 90
	DefaultContactRetentionDays   = 365
	DefaultContactHeading         = "Stay in touch"
	DefaultContactConsent         = "I agree to share my email with this creator for the purpose described above."
	DefaultContactButton          = "Sign up"
	MaxCampaignLength             = 80
)

var whitespace = regexp.MustCompile(`\s+`)

type ContactBlock struct {
	Enabled     bool   `json:"enabled"`
	Heading     string `json:"heading"`
	Description string `json:"description"`
	ConsentText string `json:"consent_text"`
	ButtonLabel string `json:"button_label"`
}

type ContactSubmission struct {
	ID        string `json:"id"`
	Email     string `json:"email"`
	Campaign  string `json:"campaign,omitempty"`
	CreatedAt string `json:"created_at"`
}

type DataSettings struct {
	AnalyticsRetentionDays int `json:"analytics_retention_days"`
	ContactRetentionDays   int `json:"contact_retention_days"`
}

func DefaultBlock() ContactBlock {
	return ContactBlock{
		Heading:     DefaultContactHeading,
		ConsentText: DefaultContactConsent,
		ButtonLabel: DefaultContactButton,
	}
}

func DefaultSettings() DataSettings {
	return DataSettings{
		AnalyticsRetentionDays: DefaultAnalyticsRetentionDays,
		ContactRetentionDays:   DefaultContactRetentionDays,
	}
}

func ValidateBlock(input ContactBlock) (ContactBlock, error) {
	input.Heading = cleanText(input.Heading)
	input.Description = cleanText(input.Description)
	input.ConsentText = cleanText(input.ConsentText)
	input.ButtonLabel = cleanText(input.ButtonLabel)
	if input.Heading == "" {
		input.Heading = DefaultContactHeading
	}
	if input.ConsentText == "" {
		input.ConsentText = DefaultContactConsent
	}
	if input.ButtonLabel == "" {
		input.ButtonLabel = DefaultContactButton
	}
	if len([]rune(input.Heading)) > 80 || len([]rune(input.Description)) > 240 || len([]rune(input.ConsentText)) > 320 || len([]rune(input.ButtonLabel)) > 40 {
		return ContactBlock{}, ErrInvalidBlock
	}
	return input, nil
}

func ValidateSettings(input DataSettings) (DataSettings, error) {
	if !allowedRetention(input.AnalyticsRetentionDays) || !allowedRetention(input.ContactRetentionDays) {
		return DataSettings{}, fmt.Errorf("%w: retention must be 30, 90, or 365 days", ErrInvalidSettings)
	}
	return input, nil
}

func NormalizeCampaign(value string) string {
	value = cleanText(value)
	if len([]rune(value)) > MaxCampaignLength {
		return ""
	}
	return value
}

func NormalizeEmail(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 254 {
		return "", ErrInvalidEmail
	}
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address == "" {
		return "", ErrInvalidEmail
	}
	clean := strings.ToLower(strings.TrimSpace(address.Address))
	if clean != strings.ToLower(value) {
		return "", ErrInvalidEmail
	}
	return clean, nil
}

func RetentionCutoff(days int, now time.Time) string {
	return now.UTC().AddDate(0, 0, -days).Format(time.RFC3339)
}

func allowedRetention(days int) bool {
	return days == 30 || days == 90 || days == 365
}

func cleanText(value string) string {
	return strings.TrimSpace(whitespace.ReplaceAllString(value, " "))
}
