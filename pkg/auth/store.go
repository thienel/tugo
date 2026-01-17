package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/thienel/tugo/pkg/apperror"
)

// DBUserStore implements UserStore using sqlx.
type DBUserStore struct {
	db        *sqlx.DB
	tableName string
}

// NewDBUserStore creates a new database-backed user store.
func NewDBUserStore(db *sqlx.DB, tableName string) *DBUserStore {
	if tableName == "" {
		tableName = "tugo_users"
	}
	return &DBUserStore{
		db:        db,
		tableName: tableName,
	}
}

// userRow represents a user row in the database.
type userRow struct {
	ID           string         `db:"id"`
	Username     string         `db:"username"`
	Email        sql.NullString `db:"email"`
	PasswordHash string         `db:"password_hash"`
	RoleID       sql.NullString `db:"role_id"`
	RoleName     sql.NullString `db:"role_name"`
	TOTPSecret   sql.NullString `db:"totp_secret"`
	TOTPEnabled  bool           `db:"totp_enabled"`
	Status       string         `db:"status"`
	CreatedAt    time.Time      `db:"created_at"`
	UpdatedAt    time.Time      `db:"updated_at"`
}

// toUser converts a userRow to a User.
func (r *userRow) toUser() *User {
	user := &User{
		ID:          r.ID,
		Username:    r.Username,
		Status:      r.Status,
		TOTPEnabled: r.TOTPEnabled,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
	if r.Email.Valid {
		user.Email = r.Email.String
	}
	if r.RoleID.Valid {
		user.RoleID = r.RoleID.String
	}
	if r.RoleName.Valid {
		user.Role = r.RoleName.String
	}
	return user
}

// GetByID retrieves a user by ID.
func (s *DBUserStore) GetByID(ctx context.Context, id string) (*User, error) {
	query := `
		SELECT u.id, u.username, u.email, u.password_hash, u.role_id,
			   r.name as role_name, u.totp_secret, u.totp_enabled,
			   u.status, u.created_at, u.updated_at
		FROM ` + s.tableName + ` u
		LEFT JOIN tugo_roles r ON u.role_id = r.id
		WHERE u.id = $1
	`

	var row userRow
	if err := s.db.GetContext(ctx, &row, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperror.ErrNotFound.WithMessage("User not found")
		}
		return nil, apperror.ErrInternalServer.WithError(err)
	}

	return row.toUser(), nil
}

// GetByUsername retrieves a user by username.
func (s *DBUserStore) GetByUsername(ctx context.Context, username string) (*User, error) {
	query := `
		SELECT u.id, u.username, u.email, u.password_hash, u.role_id,
			   r.name as role_name, u.totp_secret, u.totp_enabled,
			   u.status, u.created_at, u.updated_at
		FROM ` + s.tableName + ` u
		LEFT JOIN tugo_roles r ON u.role_id = r.id
		WHERE u.username = $1
	`

	var row userRow
	if err := s.db.GetContext(ctx, &row, query, username); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperror.ErrNotFound.WithMessage("User not found")
		}
		return nil, apperror.ErrInternalServer.WithError(err)
	}

	return row.toUser(), nil
}

// GetByEmail retrieves a user by email.
func (s *DBUserStore) GetByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT u.id, u.username, u.email, u.password_hash, u.role_id,
			   r.name as role_name, u.totp_secret, u.totp_enabled,
			   u.status, u.created_at, u.updated_at
		FROM ` + s.tableName + ` u
		LEFT JOIN tugo_roles r ON u.role_id = r.id
		WHERE u.email = $1
	`

	var row userRow
	if err := s.db.GetContext(ctx, &row, query, email); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperror.ErrNotFound.WithMessage("User not found")
		}
		return nil, apperror.ErrInternalServer.WithError(err)
	}

	return row.toUser(), nil
}

// GetPasswordHash retrieves the password hash for a user.
func (s *DBUserStore) GetPasswordHash(ctx context.Context, userID string) (string, error) {
	query := `SELECT password_hash FROM ` + s.tableName + ` WHERE id = $1`

	var hash string
	if err := s.db.GetContext(ctx, &hash, query, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", apperror.ErrNotFound.WithMessage("User not found")
		}
		return "", apperror.ErrInternalServer.WithError(err)
	}

	return hash, nil
}

// GetTOTPSecret retrieves the TOTP secret for a user.
func (s *DBUserStore) GetTOTPSecret(ctx context.Context, userID string) (string, error) {
	query := `SELECT totp_secret FROM ` + s.tableName + ` WHERE id = $1`

	var secret sql.NullString
	if err := s.db.GetContext(ctx, &secret, query, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", apperror.ErrNotFound.WithMessage("User not found")
		}
		return "", apperror.ErrInternalServer.WithError(err)
	}

	if !secret.Valid {
		return "", nil
	}
	return secret.String, nil
}

// Create creates a new user.
func (s *DBUserStore) Create(ctx context.Context, user *User, passwordHash string) error {
	if user.ID == "" {
		user.ID = uuid.New().String()
	}
	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	query := `
		INSERT INTO ` + s.tableName + ` (id, username, email, password_hash, role_id, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	var roleID interface{}
	if user.RoleID != "" {
		roleID = user.RoleID
	}

	var email interface{}
	if user.Email != "" {
		email = user.Email
	}

	status := user.Status
	if status == "" {
		status = "active"
	}

	_, err := s.db.ExecContext(ctx, query,
		user.ID, user.Username, email, passwordHash, roleID, status, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return apperror.ErrInternalServer.WithError(err)
	}

	return nil
}

// UpdatePassword updates a user's password.
func (s *DBUserStore) UpdatePassword(ctx context.Context, userID string, passwordHash string) error {
	query := `UPDATE ` + s.tableName + ` SET password_hash = $1, updated_at = $2 WHERE id = $3`

	result, err := s.db.ExecContext(ctx, query, passwordHash, time.Now(), userID)
	if err != nil {
		return apperror.ErrInternalServer.WithError(err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.ErrNotFound.WithMessage("User not found")
	}

	return nil
}

// SetTOTPSecret sets the TOTP secret for a user.
func (s *DBUserStore) SetTOTPSecret(ctx context.Context, userID string, secret string) error {
	query := `UPDATE ` + s.tableName + ` SET totp_secret = $1, updated_at = $2 WHERE id = $3`

	var secretValue interface{}
	if secret != "" {
		secretValue = secret
	}

	result, err := s.db.ExecContext(ctx, query, secretValue, time.Now(), userID)
	if err != nil {
		return apperror.ErrInternalServer.WithError(err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.ErrNotFound.WithMessage("User not found")
	}

	return nil
}

// EnableTOTP enables or disables TOTP for a user.
func (s *DBUserStore) EnableTOTP(ctx context.Context, userID string, enabled bool) error {
	query := `UPDATE ` + s.tableName + ` SET totp_enabled = $1, updated_at = $2 WHERE id = $3`

	result, err := s.db.ExecContext(ctx, query, enabled, time.Now(), userID)
	if err != nil {
		return apperror.ErrInternalServer.WithError(err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return apperror.ErrNotFound.WithMessage("User not found")
	}

	return nil
}

// DBSessionStore implements SessionStore using sqlx.
type DBSessionStore struct {
	db        *sqlx.DB
	tableName string
}

// NewDBSessionStore creates a new database-backed session store.
func NewDBSessionStore(db *sqlx.DB, tableName string) *DBSessionStore {
	if tableName == "" {
		tableName = "tugo_sessions"
	}
	return &DBSessionStore{
		db:        db,
		tableName: tableName,
	}
}

// Create creates a new session.
func (s *DBSessionStore) Create(ctx context.Context, session *Session) error {
	query := `
		INSERT INTO ` + s.tableName + ` (id, user_id, token, expires_at, created_at, user_agent, ip_address)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := s.db.ExecContext(ctx, query,
		session.ID, session.UserID, session.Token, session.ExpiresAt,
		session.CreatedAt, session.UserAgent, session.IPAddress)
	if err != nil {
		return apperror.ErrInternalServer.WithError(err)
	}

	return nil
}

// GetByToken retrieves a session by token.
func (s *DBSessionStore) GetByToken(ctx context.Context, token string) (*Session, error) {
	query := `SELECT id, user_id, token, expires_at, created_at, user_agent, ip_address FROM ` + s.tableName + ` WHERE token = $1`

	var session Session
	if err := s.db.GetContext(ctx, &session, query, token); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperror.ErrNotFound.WithMessage("Session not found")
		}
		return nil, apperror.ErrInternalServer.WithError(err)
	}

	return &session, nil
}

// Delete deletes a session.
func (s *DBSessionStore) Delete(ctx context.Context, token string) error {
	query := `DELETE FROM ` + s.tableName + ` WHERE token = $1`

	_, err := s.db.ExecContext(ctx, query, token)
	if err != nil {
		return apperror.ErrInternalServer.WithError(err)
	}

	return nil
}

// DeleteByUserID deletes all sessions for a user.
func (s *DBSessionStore) DeleteByUserID(ctx context.Context, userID string) error {
	query := `DELETE FROM ` + s.tableName + ` WHERE user_id = $1`

	_, err := s.db.ExecContext(ctx, query, userID)
	if err != nil {
		return apperror.ErrInternalServer.WithError(err)
	}

	return nil
}

// CleanExpired removes expired sessions.
func (s *DBSessionStore) CleanExpired(ctx context.Context) error {
	query := `DELETE FROM ` + s.tableName + ` WHERE expires_at < $1`

	_, err := s.db.ExecContext(ctx, query, time.Now())
	if err != nil {
		return apperror.ErrInternalServer.WithError(err)
	}

	return nil
}
