package auth

import (
	"strings"
	"testing"
)

func TestHashPassword(t *testing.T) {
	password := "mysecretpassword"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if hash == "" {
		t.Error("hash should not be empty")
	}

	if hash == password {
		t.Error("hash should not equal the original password")
	}

	// bcrypt hashes start with $2a$ or $2b$
	if !strings.HasPrefix(hash, "$2") {
		t.Errorf("hash should start with $2, got: %s", hash[:4])
	}
}

func TestHashPasswordWithCost(t *testing.T) {
	password := "testpassword"

	// Test with different costs
	costs := []int{4, 10, 12}

	for _, cost := range costs {
		hash, err := HashPasswordWithCost(password, cost)
		if err != nil {
			t.Errorf("unexpected error with cost %d: %v", cost, err)
			continue
		}

		if hash == "" {
			t.Errorf("hash should not be empty for cost %d", cost)
		}

		// Verify the hash works
		if !CheckPassword(password, hash) {
			t.Errorf("CheckPassword should return true for cost %d", cost)
		}
	}
}

func TestCheckPassword(t *testing.T) {
	password := "correctpassword"
	wrongPassword := "wrongpassword"

	hash, _ := HashPassword(password)

	if !CheckPassword(password, hash) {
		t.Error("CheckPassword should return true for correct password")
	}

	if CheckPassword(wrongPassword, hash) {
		t.Error("CheckPassword should return false for wrong password")
	}
}

func TestCheckPassword_EmptyPassword(t *testing.T) {
	hash, _ := HashPassword("somepassword")

	if CheckPassword("", hash) {
		t.Error("CheckPassword should return false for empty password")
	}
}

func TestCheckPassword_InvalidHash(t *testing.T) {
	if CheckPassword("password", "notavalidhash") {
		t.Error("CheckPassword should return false for invalid hash")
	}
}

func TestHashPassword_DifferentHashes(t *testing.T) {
	password := "samepassword"

	hash1, _ := HashPassword(password)
	hash2, _ := HashPassword(password)

	// Due to salting, the same password should produce different hashes
	if hash1 == hash2 {
		t.Error("hashing the same password twice should produce different hashes due to salting")
	}

	// But both should validate correctly
	if !CheckPassword(password, hash1) {
		t.Error("hash1 should validate")
	}
	if !CheckPassword(password, hash2) {
		t.Error("hash2 should validate")
	}
}

func TestDefaultBcryptCost(t *testing.T) {
	if DefaultBcryptCost != 12 {
		t.Errorf("expected DefaultBcryptCost to be 12, got %d", DefaultBcryptCost)
	}
}
