package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"whatsapp-ai-poc/internal/middleware"
	"whatsapp-ai-poc/internal/model"
	"whatsapp-ai-poc/internal/store"
)

func RegisterConversations(r *gin.RouterGroup, st *store.Store) {
	r.GET("", handleListConversations(st))
}

func handleListConversations(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		convs, err := st.ConversationsByTenant(session.ActiveTenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to load conversations."}})
			return
		}
		if convs == nil {
			convs = []model.Conversation{}
		}
		c.JSON(http.StatusOK, model.ConversationsResponse{Conversations: convs})
	}
}
