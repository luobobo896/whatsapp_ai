package operations

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsapp-ai-poc/internal/audit"
	"whatsapp-ai-poc/internal/members"
	"whatsapp-ai-poc/internal/platform/apperror"
	"whatsapp-ai-poc/internal/platform/database"
)

type Service struct{ pool *pgxpool.Pool }

type Account struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	AccountKey string    `json:"accountKey"`
	Status     string    `json:"status"`
	DailyLimit int       `json:"dailyLimit"`
	CreatedAt  time.Time `json:"createdAt"`
}

type KnowledgeBase struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Conversation struct {
	ID            uuid.UUID  `json:"id"`
	AccountID     *uuid.UUID `json:"accountId,omitempty"`
	Customer      string     `json:"customer"`
	LastMessage   string     `json:"lastMessage"`
	Status        string     `json:"status"`
	LastMessageAt time.Time  `json:"lastMessageAt"`
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

func (s *Service) ListAccounts(ctx context.Context, tenant members.TenantContext) ([]Account, error) {
	return database.WithTenantTx(ctx, s.pool, tenant.TenantID, func(tx pgx.Tx) ([]Account, error) {
		rows, err := tx.Query(ctx, `
			SELECT id, name, account_key, status, daily_limit, created_at
			FROM support_accounts WHERE tenant_id = $1 ORDER BY created_at DESC, id
		`, tenant.TenantID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		result := make([]Account, 0)
		for rows.Next() {
			var account Account
			if err := rows.Scan(&account.ID, &account.Name, &account.AccountKey, &account.Status, &account.DailyLimit, &account.CreatedAt); err != nil {
				return nil, err
			}
			result = append(result, account)
		}
		return result, rows.Err()
	})
}

func (s *Service) CreateAccount(ctx context.Context, tenant members.TenantContext, name string, dailyLimit int) (Account, error) {
	name = strings.TrimSpace(name)
	if name == "" || dailyLimit < 0 || dailyLimit > 10000 {
		return Account{}, apperror.Validation("A valid account name and daily limit are required.", nil)
	}
	id := uuid.New()
	account := Account{
		ID: id, Name: name, AccountKey: "wa-" + strings.ReplaceAll(id.String(), "-", ""),
		Status: "pending", DailyLimit: dailyLimit, CreatedAt: time.Now().UTC(),
	}
	return database.WithTenantTx(ctx, s.pool, tenant.TenantID, func(tx pgx.Tx) (Account, error) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO support_accounts (id, tenant_id, name, account_key, status, daily_limit, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, account.ID, tenant.TenantID, account.Name, account.AccountKey, account.Status, account.DailyLimit, account.CreatedAt); err != nil {
			return Account{}, err
		}
		if err := writeAudit(ctx, tx, tenant, "account.created", "support_account", account.ID.String(), map[string]any{
			"name": account.Name, "accountKey": account.AccountKey, "dailyLimit": account.DailyLimit,
		}); err != nil {
			return Account{}, err
		}
		return account, nil
	})
}

func (s *Service) ListKnowledgeBases(ctx context.Context, tenant members.TenantContext) ([]KnowledgeBase, error) {
	return database.WithTenantTx(ctx, s.pool, tenant.TenantID, func(tx pgx.Tx) ([]KnowledgeBase, error) {
		rows, err := tx.Query(ctx, `
			SELECT id, name, description, status, created_at
			FROM knowledge_bases WHERE tenant_id = $1 ORDER BY created_at DESC, id
		`, tenant.TenantID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		result := make([]KnowledgeBase, 0)
		for rows.Next() {
			var base KnowledgeBase
			if err := rows.Scan(&base.ID, &base.Name, &base.Description, &base.Status, &base.CreatedAt); err != nil {
				return nil, err
			}
			result = append(result, base)
		}
		return result, rows.Err()
	})
}

func (s *Service) CreateKnowledgeBase(ctx context.Context, tenant members.TenantContext, name, description string) (KnowledgeBase, error) {
	name, description = strings.TrimSpace(name), strings.TrimSpace(description)
	if name == "" {
		return KnowledgeBase{}, apperror.Validation("A knowledge base name is required.", nil)
	}
	base := KnowledgeBase{ID: uuid.New(), Name: name, Description: description, Status: "active", CreatedAt: time.Now().UTC()}
	return database.WithTenantTx(ctx, s.pool, tenant.TenantID, func(tx pgx.Tx) (KnowledgeBase, error) {
		if _, err := tx.Exec(ctx, `
			INSERT INTO knowledge_bases (id, tenant_id, name, description, status, created_at)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, base.ID, tenant.TenantID, base.Name, base.Description, base.Status, base.CreatedAt); err != nil {
			return KnowledgeBase{}, err
		}
		if err := writeAudit(ctx, tx, tenant, "knowledge_base.created", "knowledge_base", base.ID.String(), map[string]any{
			"name": base.Name,
		}); err != nil {
			return KnowledgeBase{}, err
		}
		return base, nil
	})
}

func (s *Service) ListConversations(ctx context.Context, tenant members.TenantContext) ([]Conversation, error) {
	return database.WithTenantTx(ctx, s.pool, tenant.TenantID, func(tx pgx.Tx) ([]Conversation, error) {
		rows, err := tx.Query(ctx, `
			SELECT id, account_id, customer, last_message, status, last_message_at
			FROM conversations WHERE tenant_id = $1 ORDER BY last_message_at DESC, id
		`, tenant.TenantID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		result := make([]Conversation, 0)
		for rows.Next() {
			var conversation Conversation
			if err := rows.Scan(&conversation.ID, &conversation.AccountID, &conversation.Customer, &conversation.LastMessage, &conversation.Status, &conversation.LastMessageAt); err != nil {
				return nil, err
			}
			result = append(result, conversation)
		}
		return result, rows.Err()
	})
}

func writeAudit(ctx context.Context, tx pgx.Tx, tenant members.TenantContext, action, targetType, targetID string, summary map[string]any) error {
	tenantID := tenant.TenantID
	return audit.Write(ctx, tx, audit.Event{
		TenantID: &tenantID,
		Actor: audit.Actor{
			UserID: tenant.UserID, Role: string(tenant.Role), RequestID: tenant.RequestID,
			IP: tenant.IP, UserAgent: tenant.UserAgent,
		},
		Action: action, TargetType: targetType, TargetID: targetID, Result: "success", ChangeSummary: summary,
	})
}
