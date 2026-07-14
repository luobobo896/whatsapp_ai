package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"whatsapp-ai-poc/internal/middleware"
	"whatsapp-ai-poc/internal/model"
	"whatsapp-ai-poc/internal/store"
)

func RegisterKnowledge(r *gin.RouterGroup, st *store.Store) {
	r.GET("/bases", handleListBases(st))
	r.POST("/bases", handleCreateBase(st))
}

func handleListBases(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		bases, err := st.KnowledgeBasesByTenant(session.ActiveTenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to load knowledge bases."}})
			return
		}
		if bases == nil {
			bases = []model.KnowledgeBase{}
		}
		c.JSON(http.StatusOK, model.KnowledgeBasesResponse{Bases: bases})
	}
}

func handleCreateBase(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		var req model.CreateKnowledgeRequest
		if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Knowledge name is required."}})
			return
		}
		base, err := st.CreateKnowledgeBase(session.ActiveTenantID, req.Name, req.Description)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to create knowledge base."}})
			return
		}
		c.JSON(http.StatusOK, base)
	}
}
