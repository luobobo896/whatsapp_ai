package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"whatsapp-ai-poc/internal/middleware"
	"whatsapp-ai-poc/internal/model"
	"whatsapp-ai-poc/internal/store"
)

func RegisterConversations(r *gin.RouterGroup, st *store.Store) {
	r.GET("", handleListConversations(st))
	r.POST("/query", handleConversationQuery(st))
	r.POST("/messages", handleSaveMessage(st))
	r.GET("/:id/messages", handleMessages(st))
}

func handleListConversations(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		accountID := c.Query("accountId")
		summaries, err := st.ListConversationSummaries(session.ActiveTenantID, accountID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to load conversations."}})
			return
		}
		c.JSON(http.StatusOK, model.ConversationListResponse{Conversations: summaries})
	}
}

func handleConversationQuery(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		var req model.ConversationQueryRequest
		if err := c.ShouldBindJSON(&req); err != nil || req.Message == "" || req.ConversationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "message and conversationId are required."}})
			return
		}
		if req.MaxHistory <= 0 { req.MaxHistory = 10 }
		if req.MaxKnowledge <= 0 { req.MaxKnowledge = 5 }

		// 1. Save customer message
		st.SaveMessage(session.ActiveTenantID, req.AccountID, req.ConversationID, req.CustomerName, "customer", req.Message, "[]")

		// 2. Search knowledge
		results, searchErr := st.SearchKnowledge(session.ActiveTenantID, req.Message, nil, req.MaxKnowledge)
		if searchErr != nil {
			// log error but continue
		}

		// 3. Load history
		msgs, _ := st.LoadHistory(session.ActiveTenantID, req.ConversationID, req.MaxHistory)
		// Reverse to chronological order
		history := make([]model.HistoryMessage, len(msgs))
		for i, m := range msgs {
			history[len(msgs)-1-i] = model.HistoryMessage{Role: m.Role, Content: m.Content}
		}

		// 4. Build system prompt with guardrails
		knowledgeText := buildKnowledgeContext(results)
		systemPrompt := fmt.Sprintf(`你是 WhatsApp AI 客服助手。

## 知识库内容（只能基于以下信息回答）
%s

## 严格规则
1. 只能根据上述知识库内容回答客户问题，禁止编造、猜测或使用知识库外的信息
2. 如果知识库中没有相关信息，请回复："抱歉，我暂时无法回答这个问题，请咨询人工客服。"
3. 回答要简洁准确，用中文回复
4. 保持友好专业的态度`, knowledgeText)

		// 5. Return full context for OpenClaw
		c.JSON(http.StatusOK, model.ConversationQueryResponse{
			SystemPrompt: systemPrompt,
			Knowledge:    results,
			History:      history,
			ReplyTo:      req.ConversationID,
		})
	}
}

func handleSaveMessage(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		var req model.SaveMessageRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid request."}})
			return
		}
		msg, err := st.SaveMessage(session.ActiveTenantID, req.AccountID, req.ConversationID, req.CustomerName, req.Role, req.Content, req.KnowledgeIDs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to save message."}})
			return
		}
		c.JSON(http.StatusOK, msg)
	}
}

func handleMessages(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		limit := 30
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		msgs, err := st.LoadHistory(session.ActiveTenantID, c.Param("id"), limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to load messages."}})
			return
		}
		c.JSON(http.StatusOK, model.ConversationMessagesResponse{Messages: msgs})
	}
}

func buildKnowledgeContext(results []model.SearchResultItem) string {
	if len(results) == 0 { return "（暂无相关知识库内容）" }
	var sb strings.Builder
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("\n### %d. %s [%s]\n", i+1, r.Title, r.KnowledgeBaseName))
		if r.Category != "" { sb.WriteString(fmt.Sprintf("分类: %s\n", r.Category)) }
		sb.WriteString(r.Content + "\n")
		if r.Attributes != "" && r.Attributes != "{}" {
			var attrs map[string]string
			if json.Unmarshal([]byte(r.Attributes), &attrs) == nil {
				for k, v := range attrs {
					sb.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
				}
			}
		}
	}
	return sb.String()
}
