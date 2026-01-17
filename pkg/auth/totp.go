package auth

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"strings"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"github.com/thienel/tugo/pkg/apperror"
)

// TOTPConfig holds TOTP configuration.
type TOTPConfig struct {
	// Issuer is displayed in authenticator apps.
	Issuer string

	// Period is the TOTP period in seconds.
	Period uint

	// Digits is the number of digits in the code.
	Digits otp.Digits

	// Algorithm is the hash algorithm.
	Algorithm otp.Algorithm

	// SecretSize is the size of the generated secret in bytes.
	SecretSize uint
}

// DefaultTOTPConfig returns default TOTP configuration.
func DefaultTOTPConfig() TOTPConfig {
	return TOTPConfig{
		Issuer:     "TuGo",
		Period:     30,
		Digits:     otp.DigitsSix,
		Algorithm:  otp.AlgorithmSHA1,
		SecretSize: 20,
	}
}

// TOTPManager handles TOTP operations.
type TOTPManager struct {
	config    TOTPConfig
	userStore UserStore
}

// NewTOTPManager creates a new TOTP manager.
func NewTOTPManager(config TOTPConfig, userStore UserStore) *TOTPManager {
	if config.Issuer == "" {
		config.Issuer = DefaultTOTPConfig().Issuer
	}
	if config.Period == 0 {
		config.Period = DefaultTOTPConfig().Period
	}
	if config.Digits == 0 {
		config.Digits = DefaultTOTPConfig().Digits
	}
	if config.SecretSize == 0 {
		config.SecretSize = DefaultTOTPConfig().SecretSize
	}

	return &TOTPManager{
		config:    config,
		userStore: userStore,
	}
}

// TOTPSetupResponse contains information for setting up TOTP.
type TOTPSetupResponse struct {
	Secret        string   `json:"secret"`
	QRCode        string   `json:"qr_code,omitempty"`
	URL           string   `json:"url"`
	RecoveryCodes []string `json:"recovery_codes,omitempty"`
}

// GenerateSecret generates a new TOTP secret for a user.
func (m *TOTPManager) GenerateSecret(ctx context.Context, username string) (*TOTPSetupResponse, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      m.config.Issuer,
		AccountName: username,
		Period:      m.config.Period,
		Digits:      m.config.Digits,
		Algorithm:   m.config.Algorithm,
		SecretSize:  m.config.SecretSize,
	})

	if err != nil {
		return nil, apperror.ErrInternalServer.WithError(err)
	}

	return &TOTPSetupResponse{
		Secret: key.Secret(),
		URL:    key.URL(),
	}, nil
}

// ValidateCode validates a TOTP code against a secret.
func (m *TOTPManager) ValidateCode(secret, code string) bool {
	// Normalize secret (remove spaces, uppercase)
	secret = strings.ToUpper(strings.ReplaceAll(secret, " ", ""))

	return totp.Validate(code, secret)
}

// ValidateCodeForUser validates a TOTP code for a user.
func (m *TOTPManager) ValidateCodeForUser(ctx context.Context, userID, code string) error {
	secret, err := m.userStore.GetTOTPSecret(ctx, userID)
	if err != nil {
		return apperror.ErrInternalServer.WithError(err)
	}

	if secret == "" {
		return apperror.ErrBadRequest.WithMessage("TOTP not set up for this user")
	}

	if !m.ValidateCode(secret, code) {
		return apperror.ErrUnauthorized.WithMessage("Invalid TOTP code")
	}

	return nil
}

// SetupTOTP sets up TOTP for a user.
func (m *TOTPManager) SetupTOTP(ctx context.Context, userID, username string) (*TOTPSetupResponse, error) {
	// Generate new secret
	response, err := m.GenerateSecret(ctx, username)
	if err != nil {
		return nil, err
	}

	// Store the secret (not enabled yet)
	if err := m.userStore.SetTOTPSecret(ctx, userID, response.Secret); err != nil {
		return nil, apperror.ErrInternalServer.WithError(err)
	}

	return response, nil
}

// EnableTOTP enables TOTP for a user after verification.
func (m *TOTPManager) EnableTOTP(ctx context.Context, userID, code string) error {
	// Validate the code first
	if err := m.ValidateCodeForUser(ctx, userID, code); err != nil {
		return err
	}

	// Enable TOTP
	if err := m.userStore.EnableTOTP(ctx, userID, true); err != nil {
		return apperror.ErrInternalServer.WithError(err)
	}

	return nil
}

// DisableTOTP disables TOTP for a user.
func (m *TOTPManager) DisableTOTP(ctx context.Context, userID, code string) error {
	// Validate the code first
	if err := m.ValidateCodeForUser(ctx, userID, code); err != nil {
		return err
	}

	// Disable TOTP
	if err := m.userStore.EnableTOTP(ctx, userID, false); err != nil {
		return apperror.ErrInternalServer.WithError(err)
	}

	// Clear the secret
	if err := m.userStore.SetTOTPSecret(ctx, userID, ""); err != nil {
		return apperror.ErrInternalServer.WithError(err)
	}

	return nil
}

// GenerateRecoveryCodes generates backup recovery codes.
func GenerateRecoveryCodes(count int) ([]string, error) {
	codes := make([]string, count)
	for i := 0; i < count; i++ {
		// Generate 8 random bytes
		bytes := make([]byte, 8)
		if _, err := rand.Read(bytes); err != nil {
			return nil, err
		}
		// Encode as base32 and format
		code := base32.StdEncoding.EncodeToString(bytes)[:10]
		codes[i] = formatRecoveryCode(code)
	}
	return codes, nil
}

// formatRecoveryCode formats a recovery code as XXXXX-XXXXX.
func formatRecoveryCode(code string) string {
	if len(code) < 10 {
		return code
	}
	return code[:5] + "-" + code[5:10]
}
