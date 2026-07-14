package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"whatsapp-ai-poc/internal/middleware"
	"whatsapp-ai-poc/internal/model"
	"whatsapp-ai-poc/internal/store"
)

func RegisterKnowledge(r *gin.RouterGroup, st *store.Store) {
	// Base CRUD
	r.GET("/bases", handleListBases(st))
	r.POST("/bases", handleCreateBase(st))
	r.GET("/bases/:id", handleGetBase(st))
	r.PATCH("/bases/:id", handleUpdateBase(st))
	r.DELETE("/bases/:id", handleDeleteBase(st))

	// Article CRUD under a base
	r.GET("/bases/:id/articles", handleListArticles(st))
	r.POST("/bases/:id/articles", handleCreateArticle(st))
	r.PATCH("/bases/:id/articles/:articleId", handleUpdateArticle(st))
	r.DELETE("/bases/:id/articles/:articleId", handleDeleteArticle(st))
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

func handleGetBase(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		base, err := st.KnowledgeBaseByID(c.Param("id"), session.ActiveTenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Knowledge base not found."}})
			return
		}
		c.JSON(http.StatusOK, base)
	}
}

func handleUpdateBase(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		var req model.UpdateKnowledgeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid request."}})
			return
		}
		base, err := st.UpdateKnowledgeBase(c.Param("id"), session.ActiveTenantID, req.Name, req.Description, req.Status)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to update knowledge base."}})
			return
		}
		c.JSON(http.StatusOK, base)
	}
}

func handleDeleteBase(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		if err := st.DeleteKnowledgeBase(c.Param("id"), session.ActiveTenantID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to delete knowledge base."}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func handleListArticles(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		// Verify base belongs to tenant
		if _, err := st.KnowledgeBaseByID(c.Param("id"), session.ActiveTenantID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Knowledge base not found."}})
			return
		}
		articles, err := st.ArticlesByKnowledgeBase(c.Param("id"), session.ActiveTenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to load articles."}})
			return
		}
		c.JSON(http.StatusOK, model.KnowledgeArticlesResponse{Articles: articles})
	}
}

func handleCreateArticle(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		// Verify base belongs to tenant
		if _, err := st.KnowledgeBaseByID(c.Param("id"), session.ActiveTenantID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Knowledge base not found."}})
			return
		}
		var req model.CreateArticleRequest
		if err := c.ShouldBindJSON(&req); err != nil || req.Title == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Title is required."}})
			return
		}
		article, err := st.CreateArticle(c.Param("id"), req.Title, req.Content, req.Category, req.Attributes)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to create article."}})
			return
		}
		c.JSON(http.StatusOK, article)
	}
}

func handleUpdateArticle(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		// Verify the parent knowledge base belongs to tenant
		if _, err := st.KnowledgeBaseByID(c.Param("id"), session.ActiveTenantID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Knowledge base not found."}})
			return
		}
		var req model.UpdateArticleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid request."}})
			return
		}
		article, err := st.UpdateArticle(c.Param("articleId"), req.Title, req.Content, req.Category, req.Attributes, req.Status)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to update article."}})
			return
		}
		c.JSON(http.StatusOK, article)
	}
}

func handleDeleteArticle(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		// Verify the parent knowledge base belongs to tenant
		if _, err := st.KnowledgeBaseByID(c.Param("id"), session.ActiveTenantID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Knowledge base not found."}})
			return
		}
		if err := st.DeleteArticle(c.Param("articleId")); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to delete article."}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
