package tenants

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsapp-ai-poc/internal/audit"
	"whatsapp-ai-poc/internal/members"
	"whatsapp-ai-poc/internal/platform/apperror"
	"whatsapp-ai-poc/internal/platform/database"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type InvitationIssuer interface {
	IssueInTx(context.Context, pgx.Tx, uuid.UUID, uuid.UUID, string, members.Role) (members.IssuedInvitation, error)
}

type Service struct {
	pool   *pgxpool.Pool
	issuer InvitationIssuer
}

type Tenant struct {
	ID     uuid.UUID `json:"id"`
	Name   string    `json:"name"`
	Slug   string    `json:"slug"`
	Status string    `json:"status"`
}

type AccessibleTenant struct {
	Tenant
	Role             members.Role         `json:"role,omitempty"`
	MembershipStatus string               `json:"membershipStatus,omitempty"`
	Permissions      []members.Permission `json:"permissions,omitempty"`
}

type CreateInput struct {
	Name, Slug, OwnerEmail, OwnerDisplayName string
}

type Created struct {
	Tenant     Tenant
	Invitation members.IssuedInvitation
}

func NewService(pool *pgxpool.Pool, issuer InvitationIssuer) *Service {
	return &Service{pool: pool, issuer: issuer}
}

func (s *Service) ListAccessible(ctx context.Context, userID uuid.UUID) ([]AccessibleTenant, string, error) {
	var platformRole string
	err := s.pool.QueryRow(ctx, "SELECT role FROM platform_roles WHERE user_id = $1", userID).Scan(&platformRole)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, "", err
	}

	rows, err := s.pool.Query(ctx, "SELECT id, name, slug, status FROM tenants ORDER BY lower(name), id")
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	candidates := make([]AccessibleTenant, 0)
	for rows.Next() {
		var tenant AccessibleTenant
		if err := rows.Scan(&tenant.ID, &tenant.Name, &tenant.Slug, &tenant.Status); err != nil {
			return nil, "", err
		}
		candidates = append(candidates, tenant)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	result := make([]AccessibleTenant, 0, len(candidates))
	for _, candidate := range candidates {
		access, err := database.WithTenantTx(ctx, s.pool, candidate.ID, func(tx pgx.Tx) (AccessibleTenant, error) {
			var role members.Role
			var status string
			err := tx.QueryRow(ctx, `
				SELECT role, status FROM tenant_memberships
				WHERE tenant_id = $1 AND user_id = $2
			`, candidate.ID, userID).Scan(&role, &status)
			if err != nil {
				return AccessibleTenant{}, err
			}
			candidate.Role = role
			candidate.MembershipStatus = status
			candidate.Permissions = members.PermissionsFor(role)
			return candidate, nil
		})
		if errors.Is(err, pgx.ErrNoRows) {
			if platformRole == "platform_admin" {
				result = append(result, candidate)
			}
			continue
		}
		if err != nil {
			return nil, "", err
		}
		result = append(result, access)
	}
	return result, platformRole, nil
}

func (s *Service) Create(ctx context.Context, actor audit.Actor, input CreateInput) (Created, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Slug = strings.TrimSpace(input.Slug)
	input.OwnerEmail = strings.ToLower(strings.TrimSpace(input.OwnerEmail))
	input.OwnerDisplayName = strings.TrimSpace(input.OwnerDisplayName)
	if input.Name == "" || input.OwnerDisplayName == "" || !slugPattern.MatchString(input.Slug) || !strings.Contains(input.OwnerEmail, "@") {
		return Created{}, apperror.Validation("Valid tenant and owner details are required.", nil)
	}
	tenant := Tenant{ID: uuid.New(), Name: input.Name, Slug: input.Slug, Status: "active"}
	created, err := database.WithPlatformTx(ctx, s.pool, func(tx pgx.Tx) (Created, error) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO tenants (id, name, slug, status) VALUES ($1, $2, $3, 'active')
		`, tenant.ID, tenant.Name, tenant.Slug); err != nil {
			return Created{}, err
		}
		if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenant.ID.String()); err != nil {
			return Created{}, err
		}
		invitation, err := s.issuer.IssueInTx(ctx, tx, tenant.ID, actor.UserID, input.OwnerEmail, members.RoleOwner)
		if err != nil {
			return Created{}, err
		}
		tenantID := tenant.ID
		if err := audit.Write(ctx, tx, audit.Event{
			TenantID: &tenantID, Actor: actor,
			Action: "tenant.created", TargetType: "tenant", TargetID: tenant.ID.String(), Result: "success",
			ChangeSummary: map[string]any{"name": tenant.Name, "slug": tenant.Slug, "ownerEmail": input.OwnerEmail, "ownerDisplayName": input.OwnerDisplayName},
		}); err != nil {
			return Created{}, err
		}
		return Created{Tenant: tenant, Invitation: invitation}, nil
	})
	return created, tenantConflict(err)
}

func (s *Service) SetStatus(ctx context.Context, actor audit.Actor, tenantID uuid.UUID, status, reason string) error {
	status, reason = strings.TrimSpace(status), strings.TrimSpace(reason)
	if (status != "active" && status != "suspended") || (status == "suspended" && reason == "") {
		return apperror.Validation("A valid status and suspension reason are required.", nil)
	}
	_, err := database.WithPlatformTx(ctx, s.pool, func(tx pgx.Tx) (struct{}, error) {
		var suspendedReason any
		if status == "suspended" {
			suspendedReason = reason
		}
		tag, err := tx.Exec(ctx, `
			UPDATE tenants SET status = $2, suspended_reason = $3, updated_at = now()
			WHERE id = $1
		`, tenantID, status, suspendedReason)
		if err != nil {
			return struct{}{}, err
		}
		if tag.RowsAffected() != 1 {
			return struct{}{}, apperror.NotFound()
		}
		if _, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID.String()); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, audit.Write(ctx, tx, audit.Event{
			TenantID: &tenantID, Actor: actor,
			Action: "tenant.status_changed", TargetType: "tenant", TargetID: tenantID.String(), Result: "success",
			ChangeSummary: map[string]any{"status": status, "reason": reason},
		})
	})
	return err
}

func tenantConflict(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return apperror.Conflict("A tenant with this slug already exists.")
	}
	return err
}
