package auth

import (
	"golang.org/x/crypto/bcrypt"
)

const (
	// DefaultBcryptCost is the default bcrypt cost factor.
	DefaultBcryptCost = 12
)

// HashPassword hashes a password using bcrypt.
func HashPassword(password string) (string, error) {
	return HashPasswordWithCost(password, DefaultBcryptCost)
}

// HashPasswordWithCost hashes a password with a specific cost factor.
func HashPasswordWithCost(password string, cost int) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// CheckPassword compares a password with a hash.
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
