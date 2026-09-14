package auth

import (
	"context"
	"log/slog"
)

// MockSMSProvider is for local development only. It writes OTPs to structured logs.
type MockSMSProvider struct{ logger *slog.Logger }

func NewMockSMSProvider(logger *slog.Logger) *MockSMSProvider {
	return &MockSMSProvider{logger: logger}
}
func (p *MockSMSProvider) SendOTP(_ context.Context, phone, code string) error {
	p.logger.Warn("mock sms otp", "phone", phone, "code", code)
	return nil
}
