package audit

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Actor struct {
	UserID    uuid.UUID
	Role      string
	RequestID string
	IP        string
	UserAgent string
}

type Event struct {
	TenantID      *uuid.UUID
	Actor         Actor
	Action        string
	TargetType    string
	TargetID      string
	Result        string
	ChangeSummary any
}

func Write(ctx context.Context, tx pgx.Tx, event Event) error {
	summary, err := json.Marshal(Redact(event.ChangeSummary))
	if err != nil {
		return err
	}
	var actorID any
	if event.Actor.UserID != uuid.Nil {
		actorID = event.Actor.UserID
	}
	var ip any
	if event.Actor.IP != "" {
		ip = event.Actor.IP
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO audit_logs (
			id, tenant_id, actor_user_id, actor_role, action, target_type,
			target_id, request_id, result, change_summary, ip, user_agent
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11, $12)
	`, uuid.New(), event.TenantID, actorID, nullable(event.Actor.Role), event.Action,
		event.TargetType, event.TargetID, event.Actor.RequestID, event.Result,
		string(summary), ip, nullable(event.Actor.UserAgent))
	return err
}

func nullable(value string) any {
	if value == "" {
		return nil
	}
	return value
}
