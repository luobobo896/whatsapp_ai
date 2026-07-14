package members

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsapp-ai-poc/internal/audit"
	"whatsapp-ai-poc/internal/auth"
	"whatsapp-ai-poc/internal/platform/apperror"
	"whatsapp-ai-poc/internal/platform/database"
)

const invitationTTL = 7 * 24 * time.Hour

type Service struct {
	pool       *pgxpool.Pool
	sessionTTL time.Duration
}

type InviteInput struct {
	Email string
	Role  Role
}

type IssuedInvitation struct {
	ID        uuid.UUID `json:"id"`
	TenantID  uuid.UUID `json:"tenantId"`
	Email     string    `json:"email"`
	Role      Role      `json:"role"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type AcceptInput struct {
	Email       string
	DisplayName string
	Password    string
	RequestID   string
	IP          string
	UserAgent   string
}

type AcceptedInvitation struct {
	Tokens   auth.SessionTokens
	UserID   uuid.UUID
	TenantID uuid.UUID
}

type Member struct {
	UserID      uuid.UUID `json:"userId"`
	Email       string    `json:"email"`
	DisplayName string    `json:"displayName"`
	Role        Role      `json:"role"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
}

type UpdateInput struct {
	Role   Role
	Status string
}

func NewService(pool *pgxpool.Pool, sessionTTL time.Duration) *Service {
	return &Service{pool: pool, sessionTTL: sessionTTL}
}

func (s *Service) Invite(ctx context.Context, tenant TenantContext, input InviteInput) (IssuedInvitation, error) {
	if !HasPermission(tenant.Role, PermissionMembersManage) {
		return IssuedInvitation{}, apperror.Forbidden()
	}
	email := normalizeEmail(input.Email)
	if !validEmail(email) || !ValidRole(input.Role) {
		return IssuedInvitation{}, apperror.Validation("A valid email and role are required.", nil)
	}
	result, err := database.WithTenantTx(ctx, s.pool, tenant.TenantID, func(tx pgx.Tx) (IssuedInvitation, error) {
		invitation, err := s.IssueInTx(ctx, tx, tenant.TenantID, tenant.UserID, email, input.Role)
		if err != nil {
			return IssuedInvitation{}, err
		}
		tenantID := tenant.TenantID
		err = audit.Write(ctx, tx, audit.Event{
			TenantID: &tenantID,
			Actor:    audit.Actor{UserID: tenant.UserID, Role: string(tenant.Role), RequestID: tenant.RequestID, IP: tenant.IP, UserAgent: tenant.UserAgent},
			Action:   "member.invited", TargetType: "member_invitation", TargetID: invitation.ID.String(), Result: "success",
			ChangeSummary: map[string]any{"email": email, "role": input.Role},
		})
		return invitation, err
	})
	return result, translateConflict(err)
}

func (s *Service) IssueInTx(
	ctx context.Context,
	tx pgx.Tx,
	tenantID, createdBy uuid.UUID,
	email string,
	role Role,
) (IssuedInvitation, error) {
	token, err := invitationToken()
	if err != nil {
		return IssuedInvitation{}, err
	}
	invitation := IssuedInvitation{
		ID: uuid.New(), TenantID: tenantID, Email: normalizeEmail(email), Role: role,
		Token: token, ExpiresAt: time.Now().UTC().Add(invitationTTL),
	}
	if _, err := tx.Exec(ctx, `
		UPDATE member_invitations SET revoked_at = now()
		WHERE tenant_id = $1 AND lower(email) = $2 AND accepted_at IS NULL
			AND revoked_at IS NULL AND expires_at <= now()
	`, tenantID, invitation.Email); err != nil {
		return IssuedInvitation{}, err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO member_invitations
			(id, tenant_id, email, role, token_hash, expires_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, invitation.ID, tenantID, invitation.Email, invitation.Role,
		invitationHash(invitation.Token), invitation.ExpiresAt, createdBy)
	if err != nil {
		return IssuedInvitation{}, err
	}
	return invitation, nil
}

func (s *Service) Accept(ctx context.Context, token string, input AcceptInput) (AcceptedInvitation, error) {
	email := normalizeEmail(input.Email)
	displayName := strings.TrimSpace(input.DisplayName)
	if token == "" || !validEmail(email) || displayName == "" || len(input.Password) < 12 {
		return AcceptedInvitation{}, apperror.Validation("Valid invitation details and a password of at least 12 characters are required.", nil)
	}
	encodedPassword, err := auth.HashPassword(input.Password)
	if err != nil {
		return AcceptedInvitation{}, err
	}
	tokenHash := invitationHash(token)
	tenantID, err := database.ResolveInvitationTenant(ctx, s.pool, tokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return AcceptedInvitation{}, apperror.NotFound()
	}
	if err != nil {
		return AcceptedInvitation{}, err
	}

	accepted, err := database.WithTenantTx(ctx, s.pool, tenantID, func(tx pgx.Tx) (AcceptedInvitation, error) {
		var invitationID uuid.UUID
		var invitedEmail string
		var role Role
		var expiresAt time.Time
		var acceptedAt, revokedAt *time.Time
		err := tx.QueryRow(ctx, `
			SELECT id, email, role, expires_at, accepted_at, revoked_at
			FROM member_invitations
			WHERE token_hash = $1
			FOR UPDATE
		`, tokenHash).Scan(&invitationID, &invitedEmail, &role, &expiresAt, &acceptedAt, &revokedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return AcceptedInvitation{}, apperror.NotFound()
		}
		if err != nil {
			return AcceptedInvitation{}, err
		}
		if normalizeEmail(invitedEmail) != email {
			return AcceptedInvitation{}, apperror.Forbidden()
		}
		if acceptedAt != nil || revokedAt != nil || !expiresAt.After(time.Now()) {
			return AcceptedInvitation{}, apperror.Conflict("This invitation is no longer valid.")
		}

		userID, err := upsertInvitedUser(ctx, tx, email, displayName, encodedPassword)
		if err != nil {
			return AcceptedInvitation{}, err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO tenant_memberships (tenant_id, user_id, role, status)
			VALUES ($1, $2, $3, 'active')
			ON CONFLICT (tenant_id, user_id) DO UPDATE
			SET role = EXCLUDED.role, status = 'active', updated_at = now()
		`, tenantID, userID, role); err != nil {
			return AcceptedInvitation{}, err
		}
		if _, err := tx.Exec(ctx, "UPDATE member_invitations SET accepted_at = now() WHERE id = $1", invitationID); err != nil {
			return AcceptedInvitation{}, err
		}
		tokens, err := auth.Issue(ctx, tx, userID, s.sessionTTL)
		if err != nil {
			return AcceptedInvitation{}, err
		}
		tenantValue := tenantID
		if err := audit.Write(ctx, tx, audit.Event{
			TenantID: &tenantValue,
			Actor:    audit.Actor{UserID: userID, Role: string(role), RequestID: input.RequestID, IP: input.IP, UserAgent: input.UserAgent},
			Action:   "member.invitation_accepted", TargetType: "tenant_membership", TargetID: userID.String(), Result: "success",
			ChangeSummary: map[string]any{"email": email, "role": role},
		}); err != nil {
			return AcceptedInvitation{}, err
		}
		return AcceptedInvitation{Tokens: tokens, UserID: userID, TenantID: tenantID}, nil
	})
	return accepted, translateConflict(err)
}

func (s *Service) List(ctx context.Context, tenant TenantContext) ([]Member, error) {
	return database.WithTenantTx(ctx, s.pool, tenant.TenantID, func(tx pgx.Tx) ([]Member, error) {
		rows, err := tx.Query(ctx, `
			SELECT u.id, u.email, u.display_name, m.role, m.status, m.created_at
			FROM tenant_memberships m
			JOIN users u ON u.id = m.user_id
			ORDER BY lower(u.email), u.id
		`)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		members := make([]Member, 0)
		for rows.Next() {
			var member Member
			if err := rows.Scan(&member.UserID, &member.Email, &member.DisplayName, &member.Role, &member.Status, &member.CreatedAt); err != nil {
				return nil, err
			}
			members = append(members, member)
		}
		return members, rows.Err()
	})
}

func (s *Service) Update(ctx context.Context, tenant TenantContext, userID uuid.UUID, input UpdateInput) error {
	if !HasPermission(tenant.Role, PermissionMembersManage) {
		return apperror.Forbidden()
	}
	_, err := database.WithTenantTx(ctx, s.pool, tenant.TenantID, func(tx pgx.Tx) (struct{}, error) {
		var currentRole Role
		var currentStatus string
		err := tx.QueryRow(ctx, `
			SELECT role, status FROM tenant_memberships
			WHERE tenant_id = $1 AND user_id = $2 FOR UPDATE
		`, tenant.TenantID, userID).Scan(&currentRole, &currentStatus)
		if errors.Is(err, pgx.ErrNoRows) {
			return struct{}{}, apperror.NotFound()
		}
		if err != nil {
			return struct{}{}, err
		}
		role, status := input.Role, input.Status
		if role == "" {
			role = currentRole
		}
		if status == "" {
			status = currentStatus
		}
		if !ValidRole(role) || (status != "active" && status != "disabled") {
			return struct{}{}, apperror.Validation("A valid role and status are required.", nil)
		}
		if currentRole == RoleOwner && currentStatus == "active" && (role != RoleOwner || status != "active") {
			rows, err := tx.Query(ctx, `
				SELECT user_id FROM tenant_memberships
				WHERE tenant_id = $1 AND role = 'owner' AND status = 'active'
				FOR UPDATE
			`, tenant.TenantID)
			if err != nil {
				return struct{}{}, err
			}
			ownerCount := 0
			for rows.Next() {
				ownerCount++
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return struct{}{}, err
			}
			if ownerCount <= 1 {
				return struct{}{}, apperror.Conflict("The final active owner cannot be disabled or demoted.")
			}
		}
		if _, err := tx.Exec(ctx, `
			UPDATE tenant_memberships SET role = $3, status = $4, updated_at = now()
			WHERE tenant_id = $1 AND user_id = $2
		`, tenant.TenantID, userID, role, status); err != nil {
			return struct{}{}, err
		}
		tenantID := tenant.TenantID
		return struct{}{}, audit.Write(ctx, tx, audit.Event{
			TenantID: &tenantID,
			Actor:    audit.Actor{UserID: tenant.UserID, Role: string(tenant.Role), RequestID: tenant.RequestID, IP: tenant.IP, UserAgent: tenant.UserAgent},
			Action:   "member.updated", TargetType: "tenant_membership", TargetID: userID.String(), Result: "success",
			ChangeSummary: map[string]any{"role": role, "status": status},
		})
	})
	return translateConflict(err)
}

func upsertInvitedUser(ctx context.Context, tx pgx.Tx, email, displayName, encodedPassword string) (uuid.UUID, error) {
	var userID uuid.UUID
	err := tx.QueryRow(ctx, "SELECT id FROM users WHERE lower(email) = $1 FOR UPDATE", email).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		userID = uuid.New()
		_, err = tx.Exec(ctx, `
			INSERT INTO users (id, email, display_name, password_hash, status)
			VALUES ($1, $2, $3, $4, 'active')
		`, userID, email, displayName, encodedPassword)
		return userID, err
	}
	if err != nil {
		return uuid.Nil, err
	}
	_, err = tx.Exec(ctx, `
		UPDATE users SET display_name = $2, password_hash = $3, status = 'active',
			password_changed_at = now(), updated_at = now()
		WHERE id = $1
	`, userID, displayName, encodedPassword)
	return userID, err
}

func invitationToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func invitationHash(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func normalizeEmail(email string) string { return strings.ToLower(strings.TrimSpace(email)) }

func validEmail(email string) bool {
	parts := strings.Split(email, "@")
	return len(parts) == 2 && parts[0] != "" && strings.Contains(parts[1], ".") && !strings.ContainsAny(email, " \t\r\n")
}

func translateConflict(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return apperror.Conflict("")
	}
	return err
}
