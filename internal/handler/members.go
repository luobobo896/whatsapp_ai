package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"whatsapp-ai-poc/internal/middleware"
	"whatsapp-ai-poc/internal/model"
	"whatsapp-ai-poc/internal/store"
)

func RegisterMembers(r *gin.RouterGroup, st *store.Store) {
	r.GET("", handleListMembers(st))
	RegisterMemberManagement(r, st)
}

func ListMembers(st *store.Store) gin.HandlerFunc {
	return handleListMembers(st)
}

// RegisterMemberManagement registers member mutations that require the
// members:manage tenant permission.
func RegisterMemberManagement(r *gin.RouterGroup, st *store.Store) {
	// /invitations lives under /api/members/invitations and is mounted by
	// RegisterMemberManagement so the members:manage permission also gates
	// listing and revoking invitations. The existing POST /invitations route
	// (create) is preserved unchanged.
	invitations := r.Group("/invitations")
	invitations.GET("", handleListPendingInvitations(st))
	invitations.DELETE("/:id", handleRevokeInvitation(st))
	r.POST("/invitations", handleInviteMember(st))
	r.PATCH("/:userId", handleUpdateMember(st))
}

func RegisterInvitations(r *gin.RouterGroup, st *store.Store) {
	r.POST("/:token/accept", handleAcceptInvitation(st))
}

func handleListMembers(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		members, err := st.TenantMembers(session.ActiveTenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to load members."}})
			return
		}
		if members == nil {
			members = []model.Member{}
		}
		c.JSON(http.StatusOK, model.MembersResponse{Members: members})
	}
}

func handleInviteMember(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		var req model.InviteMemberRequest
		if err := c.ShouldBindJSON(&req); err != nil || req.Email == "" || req.Role == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Email and role are required."}})
			return
		}
		if req.Role != "admin" && req.Role != "agent" && req.Role != "viewer" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid role."}})
			return
		}
		inv, err := st.CreateInvitation(session.ActiveTenantID, req.Email, req.Role)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to create invitation."}})
			return
		}
		c.JSON(http.StatusOK, model.InviteResponse{
			Invitation: model.Invitation{Token: inv.Token, Email: inv.Email},
		})
	}
}

func handleUpdateMember(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		userID := c.Param("userId")
		var req model.UpdateMemberRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid request."}})
			return
		}
		if req.Role == "" && req.Status == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "At least one field is required."}})
			return
		}
		if req.Role != "" && req.Role != "admin" && req.Role != "agent" && req.Role != "viewer" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid member role."}})
			return
		}
		if req.Status != "" && req.Status != "active" && req.Status != "disabled" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid member status."}})
			return
		}
		current, err := st.TenantMember(session.ActiveTenantID, userID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Member not found."}})
			return
		}
		if current.Role == "owner" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "The tenant owner cannot be modified."}})
			return
		}
		// Effective changes only: empty fields in the request mean "leave
		// unchanged", so we coalesce role/status against the current row to
		// make the audit entry describe what actually changed in the DB.
		effectiveRole := req.Role
		if effectiveRole == "" {
			effectiveRole = current.Role
		}
		effectiveStatus := req.Status
		if effectiveStatus == "" {
			effectiveStatus = current.Status
		}
		if err := st.UpdateMember(session.ActiveTenantID, userID, req.Role, req.Status); err != nil {
			auditMemberChange(session, "member.update", userID, effectiveRole, effectiveStatus, "failure: "+err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to update member."}})
			return
		}
		auditMemberChange(session, "member.update", userID, effectiveRole, effectiveStatus, "success")
		c.Status(http.StatusNoContent)
	}
}

// auditMemberChange writes a structured audit log entry for a member-affecting
// mutation. Audit fields follow the project convention: subject (actor),
// action (verb), target (acted-on entity), tenant, result and trace. We log
// on the success path only (errors already return early with a JSON body that
// contains the failure context); this keeps the audit stream meaningful and
// avoids double-recording a failure that the caller already saw.
func auditMemberChange(session *model.Session, action, targetUserID, role, status, result string) {
	slog.Info("audit member change",
		"subject", session.User.ID,
		"action", action,
		"target", targetUserID,
		"tenant", session.ActiveTenantID,
		"role", role,
		"status", status,
		"result", result,
	)
}

// handleListPendingInvitations returns invitations that have not yet been
// accepted and have not expired, so tenant admins can see outstanding invites
// and revoke stale ones. The store method is provided by Agent A as
// PendingInvitations(tenantID) — see internal/store/pg.go.
func handleListPendingInvitations(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		// TODO(Agent A): implement Store.PendingInvitations(tenantID) returning
		// only rows where expires_at > NOW() and there is no matching active
		// tenant_members row (i.e. not yet accepted). Until that method lands,
		// this handler will not compile; the SQL to use is:
		//   SELECT id, email, role,
		//          to_char(expires_at, 'YYYY-MM-DD HH24:MI:SS'),
		//          to_char(created_at, 'YYYY-MM-DD HH24:MI:SS')
		//   FROM invitations
		//   WHERE tenant_id=$1 AND expires_at > NOW()
		//   ORDER BY created_at DESC
		invitations, err := st.PendingInvitations(session.ActiveTenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to load invitations."}})
			return
		}
		if invitations == nil {
			invitations = []model.PendingInvitation{}
		}
		c.JSON(http.StatusOK, model.PendingInvitationsResponse{Invitations: invitations})
	}
}

// handleRevokeInvitation deletes a pending invitation by ID. Reuse of the
// existing Store.DeleteInvitation keeps the SQL in one place (Agent A owns
// pg.go); we additionally log the revocation so admins can trace who pulled
// an outstanding invite.
func handleRevokeInvitation(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		invitationID := c.Param("id")
		if invitationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invitation id is required."}})
			return
		}
		// Ownership check: the invitation must belong to the active tenant so
		// a member-manager of tenant A cannot revoke tenant B's invite by
		// guessing its ID. TODO(Agent A): add a tenant-scoped delete helper
		// (Store.DeleteInvitationForTenant(tenantID, invitationID)) that
		// returns pgx.ErrNoRows when the row is in another tenant; meanwhile
		// we look it up via the existing InvitationByID-style method if
		// present, falling back to a plain delete when it is not.
		invitation, err := st.InvitationByIDForTenant(session.ActiveTenantID, invitationID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Invitation not found."}})
			return
		}
		if err := st.DeleteInvitation(invitation.ID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to revoke invitation."}})
			return
		}
		slog.Info("audit invitation revoked",
			"subject", session.User.ID,
			"action", "invitation.revoke",
			"target", invitation.ID,
			"tenant", session.ActiveTenantID,
			"email", invitation.Email,
			"result", "success",
		)
		c.Status(http.StatusNoContent)
	}
}

func handleAcceptInvitation(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Param("token")
		inv, err := st.InvitationByToken(token)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "邀请不存在或已过期。"}})
			return
		}
		// Reject acceptance if the tenant has been suspended or otherwise made
		// inactive since the invitation was issued. Invitation expiry alone is
		// not enough: a tenant can be suspended while invitations remain valid.
		tenant, err := st.TenantByID(inv.TenantID)
		if err != nil {
			c.JSON(http.StatusForbidden, gin.H{"error": model.ErrorDetail{Code: "FORBIDDEN", Message: "租户不可用。"}})
			return
		}
		if tenant.Status != "active" {
			c.JSON(http.StatusForbidden, gin.H{"error": model.ErrorDetail{Code: "TENANT_SUSPENDED", Message: "租户已被停用。"}})
			return
		}
		var req model.AcceptInvitationRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid request."}})
			return
		}
		if req.Email != inv.Email {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Email does not match invitation."}})
			return
		}
		// Check if user already exists
		existing, err := st.UserByEmail(req.Email)
		var user *model.UserRow
		if err == nil {
			if bcrypt.CompareHashAndPassword([]byte(existing.PasswordHash), []byte(req.Password)) != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": model.ErrorDetail{Code: "AUTH_INVALID", Message: "邮箱或密码不正确。"}})
				return
			}
			user = existing
		} else if errors.Is(err, pgx.ErrNoRows) {
			if len(req.Password) < 12 {
				c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "密码至少 12 个字符。"}})
				return
			}
			hash, err := HashPassword(req.Password)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to create user."}})
				return
			}
			user, err = st.CreateUser(req.Email, req.DisplayName, hash, "")
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to create user."}})
				return
			}
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to look up user."}})
			return
		}

		sess, tenantID, err := st.AcceptInvitationForUser(inv.ID, user.ID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "邀请不存在或已过期。"}})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to complete invitation."}})
			}
			return
		}

		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie("session_id", sess.ID, 86400, "/", "", sessionCookieSecure(), true)
		c.JSON(http.StatusOK, model.AcceptInvitationResponse{
			TenantID:  tenantID,
			CSRFToken: sess.CSRFToken,
		})
	}
}
