package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
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
		if err := st.UpdateMember(session.ActiveTenantID, userID, req.Role, req.Status); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to update member."}})
			return
		}
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
		var req model.AcceptInvitationRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid request."}})
			return
		}
		if req.Email != inv.Email {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Email does not match invitation."}})
			return
		}
		if len(req.Password) < 12 {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "密码至少 12 个字符。"}})
			return
		}

		// Check if user already exists
		existing, _ := st.UserByEmail(req.Email)
		var user *model.UserRow
		if existing != nil {
			user = existing
		} else {
			hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to create user."}})
				return
			}
			user, err = st.CreateUser(req.Email, req.DisplayName, string(hash), "")
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to create user."}})
				return
			}
		}

		// Add to tenant
		if err := st.AddTenantMember(inv.TenantID, user.ID, inv.Role); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to add member."}})
			return
		}

		// Create session
		sess, err := st.CreateSession(user.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to create session."}})
			return
		}
		st.UpdateSessionTenant(sess.ID, inv.TenantID)
		st.DeleteInvitation(inv.ID)

		c.SetCookie("session_id", sess.ID, 86400, "/", "", false, true)
		c.JSON(http.StatusOK, model.AcceptInvitationResponse{
			TenantID:  inv.TenantID,
			CSRFToken: sess.CSRFToken,
		})
	}
}
