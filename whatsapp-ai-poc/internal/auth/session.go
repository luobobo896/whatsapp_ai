package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsapp-ai-poc/internal/platform/apperror"
	"whatsapp-ai-poc/internal/platform/database"
)

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExpired  = errors.New("session expired")
)

type SessionTokens struct {
	SessionID    uuid.UUID
	SessionToken string
	CSRFToken    string
	ExpiresAt    time.Time
}

type Identity struct {
	SessionID      uuid.UUID
	UserID         uuid.UUID
	Email          string
	DisplayName    string
	ActiveTenantID *uuid.UUID
	ExpiresAt      time.Time
	csrfHash       string
}

func Issue(ctx context.Context, db database.DBTX, userID uuid.UUID, ttl time.Duration) (SessionTokens, error) {
	sessionToken, err := randomToken()
	if err != nil {
		return SessionTokens{}, err
	}
	csrfToken, err := randomToken()
	if err != nil {
		return SessionTokens{}, err
	}
	tokens := SessionTokens{
		SessionID:    uuid.New(),
		SessionToken: sessionToken,
		CSRFToken:    csrfToken,
		ExpiresAt:    time.Now().UTC().Add(ttl),
	}
	_, err = db.Exec(ctx, `
		INSERT INTO auth_sessions (id, user_id, token_hash, csrf_hash, expires_at)
		VALUES ($1, $2, $3, $4, $5)
	`, tokens.SessionID, userID, tokenHash(sessionToken), tokenHash(csrfToken), tokens.ExpiresAt)
	if err != nil {
		return SessionTokens{}, err
	}
	return tokens, nil
}

func Resolve(ctx context.Context, pool *pgxpool.Pool, rawToken string) (Identity, error) {
	if rawToken == "" {
		return Identity{}, ErrSessionNotFound
	}
	var identity Identity
	var activeTenant pgtype.UUID
	var revokedAt *time.Time
	var createdAt, passwordChangedAt time.Time
	var userStatus string
	err := pool.QueryRow(ctx, `
		SELECT s.id, s.user_id, u.email, u.display_name, s.active_tenant_id,
			s.expires_at, s.csrf_hash, s.revoked_at, s.created_at,
			u.password_changed_at, u.status
		FROM auth_sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1
	`, tokenHash(rawToken)).Scan(
		&identity.SessionID, &identity.UserID, &identity.Email, &identity.DisplayName,
		&activeTenant, &identity.ExpiresAt, &identity.csrfHash, &revokedAt,
		&createdAt, &passwordChangedAt, &userStatus,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Identity{}, ErrSessionNotFound
	}
	if err != nil {
		return Identity{}, err
	}
	if activeTenant.Valid {
		value := uuid.UUID(activeTenant.Bytes)
		identity.ActiveTenantID = &value
	}
	if revokedAt != nil || !identity.ExpiresAt.After(time.Now()) || userStatus != "active" || createdAt.Before(passwordChangedAt) {
		return Identity{}, ErrSessionExpired
	}
	return identity, nil
}

func Revoke(ctx context.Context, pool *pgxpool.Pool, rawToken string) error {
	_, err := pool.Exec(ctx, `
		UPDATE auth_sessions SET revoked_at = COALESCE(revoked_at, now())
		WHERE token_hash = $1
	`, tokenHash(rawToken))
	return err
}

func RevokeUserSessions(ctx context.Context, pool *pgxpool.Pool, userID uuid.UUID) error {
	_, err := pool.Exec(ctx, `
		UPDATE auth_sessions SET revoked_at = COALESCE(revoked_at, now())
		WHERE user_id = $1 AND revoked_at IS NULL
	`, userID)
	return err
}

func SelectTenant(ctx context.Context, pool *pgxpool.Pool, sessionID, tenantID uuid.UUID) error {
	_, err := database.WithTenantTx(ctx, pool, tenantID, func(tx pgx.Tx) (struct{}, error) {
		var tenantStatus, membershipStatus string
		err := tx.QueryRow(ctx, `
			SELECT t.status, m.status
			FROM auth_sessions s
			JOIN tenants t ON t.id = $2
			JOIN tenant_memberships m ON m.tenant_id = t.id AND m.user_id = s.user_id
			WHERE s.id = $1 AND s.revoked_at IS NULL AND s.expires_at > now()
		`, sessionID, tenantID).Scan(&tenantStatus, &membershipStatus)
		if errors.Is(err, pgx.ErrNoRows) {
			return struct{}{}, apperror.Forbidden()
		}
		if err != nil {
			return struct{}{}, err
		}
		if membershipStatus != "active" {
			return struct{}{}, apperror.Forbidden()
		}
		if tenantStatus == "suspended" {
			return struct{}{}, apperror.TenantSuspended()
		}
		_, err = tx.Exec(ctx, "UPDATE auth_sessions SET active_tenant_id = $2 WHERE id = $1", sessionID, tenantID)
		return struct{}{}, err
	})
	return err
}

func RotateCSRF(ctx context.Context, pool *pgxpool.Pool, sessionID uuid.UUID) (string, error) {
	csrfToken, err := randomToken()
	if err != nil {
		return "", err
	}
	tag, err := pool.Exec(ctx, `
		UPDATE auth_sessions SET csrf_hash = $2
		WHERE id = $1 AND revoked_at IS NULL AND expires_at > now()
	`, sessionID, tokenHash(csrfToken))
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() != 1 {
		return "", ErrSessionExpired
	}
	return csrfToken, nil
}

func verifyCSRF(identity Identity, rawToken string) bool {
	if rawToken == "" {
		return false
	}
	want, err := hex.DecodeString(identity.csrfHash)
	if err != nil || len(want) != sha256.Size {
		return false
	}
	got := sha256.Sum256([]byte(rawToken))
	return subtle.ConstantTimeCompare(got[:], want) == 1
}

func randomToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func tokenHash(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}
