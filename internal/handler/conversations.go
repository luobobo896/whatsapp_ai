package handler

import (
	"encoding/json"
	"errors"
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
	RegisterConversationRead(r, st)
	RegisterConversationManagement(r, st)
}

// RegisterConversationRead registers conversation history endpoints available
// to all active tenant members.
func RegisterConversationRead(r *gin.RouterGroup, st *store.Store) {
	r.GET("", handleListConversations(st))
	r.GET("/:id/messages", handleMessages(st))
}

// RegisterConversationManagement registers conversation mutations that require
// the accounts:manage tenant permission.
func RegisterConversationManagement(r *gin.RouterGroup, st *store.Store) {
	r.POST("/query", handleConversationQuery(st))
	r.POST("/messages", handleSaveMessage(st))
	r.POST("/reply", handleSaveReply(st))
	r.DELETE("/:id", handleDeleteConversation(st))
}

// RegisterInternalConversations registers conversation routes for internal
// service-to-service calls (bearer token auth). Tenant is resolved from the
// accountId in the request body rather than from the session.
func RegisterInternalConversations(r *gin.RouterGroup, st *store.Store) {
	r.POST("/query", handleInternalConversationQuery(st))
	r.POST("/accounts/list", handleInternalListAccounts(st))
	r.POST("/reply", handleInternalSaveReply(st))
}

// resolveTenantFromAccount looks up the tenant that owns the given account.
func resolveTenantFromAccount(st *store.Store, accountID string) (string, error) {
	return st.TenantIDByAccountID(accountID)
}

func accountKnowledgeBaseIDs(account *model.AccountRow) []string {
	if account == nil || account.KbID == "" {
		return nil
	}
	var ids []string
	if err := json.Unmarshal([]byte(account.KbID), &ids); err != nil {
		return nil
	}
	return ids
}

func handleInternalConversationQuery(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.ConversationQueryRequest
		if err := c.ShouldBindJSON(&req); err != nil || req.Message == "" || req.ConversationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "message and conversationId are required."}})
			return
		}
		if req.MaxHistory <= 0 { req.MaxHistory = 10 }
		if req.MaxKnowledge <= 0 { req.MaxKnowledge = 5 }

		tenantID, err := resolveTenantFromAccount(st, req.AccountID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid accountId."}})
			return
		}

		// Resolve account for persona
		acctRow, err := st.AccountByID(tenantID, req.AccountID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid accountId."}})
			return
		}

		// 1. Save customer message
		if _, err := st.SaveMessage(tenantID, req.AccountID, req.ConversationID, req.CustomerName, "customer", req.Message, "[]"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to save message."}})
			return
		}

		// 2. Search only the knowledge bases bound to this account.
		results, err := st.SearchKnowledgeForBases(tenantID, accountKnowledgeBaseIDs(acctRow), req.Message, nil, req.MaxKnowledge)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to search knowledge."}})
			return
		}

		accountName := "客服"
		if acctRow.Name != "" {
			accountName = acctRow.Name
		}

		// 2a. Hard gate: when knowledge is empty, force a canned fallback reply
		var directReply string
		if len(results) == 0 {
			directReply = personaFallbackReply(accountName, req.Message)
		}

		// 3. Build history
		var history []model.HistoryMessage
		if len(req.History) > 0 {
			for _, m := range req.History {
				role := m.Role
				if role == "user" { role = "customer" }
				if err := st.SaveMessageIfAbsent(tenantID, req.AccountID, req.ConversationID, req.CustomerName, role, m.Content, "[]"); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to save conversation history."}})
					return
				}
				history = append(history, model.HistoryMessage{Role: m.Role, Content: m.Content})
			}
		} else {
			msgs, err := st.LoadHistory(tenantID, req.ConversationID, req.MaxHistory)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to load conversation history."}})
				return
			}
			history = make([]model.HistoryMessage, len(msgs))
			for i, m := range msgs {
				history[len(msgs)-1-i] = model.HistoryMessage{Role: m.Role, Content: m.Content}
			}
		}

		// 4. Build persona-aware system prompt with guardrails
		knowledgeText := buildKnowledgeContext(results)
		systemPrompt := buildSystemPrompt(accountName, knowledgeText)

		c.JSON(http.StatusOK, model.ConversationQueryResponse{
			SystemPrompt: systemPrompt,
			Knowledge:    results,
			History:      history,
			ReplyTo:      req.ConversationID,
			DirectReply:  directReply,
		})
	}
}

func handleInternalSaveReply(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.SaveReplyRequest
		if err := c.ShouldBindJSON(&req); err != nil || req.ConversationID == "" || req.Content == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "conversationId and content are required."}})
			return
		}
		tenantID, err := resolveTenantFromAccount(st, req.AccountID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid accountId."}})
			return
		}
		if req.KnowledgeIDs == "" { req.KnowledgeIDs = "[]" }
		msg, err := st.SaveAssistantReply(tenantID, req.AccountID, req.ConversationID, req.CustomerName, req.Content, req.KnowledgeIDs)
		if err != nil {
			if errors.Is(err, store.ErrDailyReplyLimitReached) {
				c.JSON(http.StatusTooManyRequests, gin.H{"error": model.ErrorDetail{Code: "DAILY_LIMIT_REACHED", Message: "Daily reply limit reached."}})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to save reply."}})
			return
		}
		c.JSON(http.StatusOK, msg)
	}
}

func handleInternalListAccounts(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		rows, err := st.AllAccounts()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to list accounts."}})
			return
		}
		type accountInfo struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			AccountKey string `json:"accountKey"`
			Status     string `json:"status"`
		}
		accounts := make([]accountInfo, 0, len(rows))
		for _, r := range rows {
			accounts = append(accounts, accountInfo{
				ID: r.ID, Name: r.Name, AccountKey: r.AccountKey, Status: r.Status,
			})
		}
		c.JSON(http.StatusOK, gin.H{"accounts": accounts})
	}
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
		account, err := st.AccountByID(session.ActiveTenantID, req.AccountID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Account not found."}})
			return
		}

		// 1. Save customer message
		if _, err := st.SaveMessage(session.ActiveTenantID, req.AccountID, req.ConversationID, req.CustomerName, "customer", req.Message, "[]"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to save message."}})
			return
		}

		// 2. Search knowledge
		results, err := st.SearchKnowledgeForBases(session.ActiveTenantID, accountKnowledgeBaseIDs(account), req.Message, nil, req.MaxKnowledge)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to search knowledge."}})
			return
		}

		// 2a. Hard gate: when knowledge is empty, force a canned fallback reply
		// so the LLM never gets a chance to free-style outside the knowledge base.
		var directReply string
		if len(results) == 0 {
			directReply = fallbackReplyForMessage(req.Message)
		}

		// 3. Build history: prefer OpenClaw-provided history (source of truth),
		//    fall back to local DB when OpenClaw does not supply it.
		var history []model.HistoryMessage
		if len(req.History) > 0 {
			// OpenClaw sends its stored conversation history (chronological order).
			// Persist each message locally for traceability, skipping the current
			// customer message (already saved above).
			for _, m := range req.History {
				role := m.Role
				if role == "user" { role = "customer" }
				if err := st.SaveMessageIfAbsent(session.ActiveTenantID, req.AccountID, req.ConversationID, req.CustomerName, role, m.Content, "[]"); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to save conversation history."}})
					return
				}
				history = append(history, model.HistoryMessage{Role: m.Role, Content: m.Content})
			}
		} else {
			// Fall back to local DB (reverse DESC result back to chronological order).
			msgs, err := st.LoadHistory(session.ActiveTenantID, req.ConversationID, req.MaxHistory)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to load conversation history."}})
				return
			}
			history = make([]model.HistoryMessage, len(msgs))
			for i, m := range msgs {
				history[len(msgs)-1-i] = model.HistoryMessage{Role: m.Role, Content: m.Content}
			}
		}

		// 4. Build system prompt with guardrails
		knowledgeText := buildKnowledgeContext(results)
		systemPrompt := fmt.Sprintf(`你是一名真实的人工客服。

## 身份规则（最高优先级，绝对不可违反）
1. 你是真实的人类客服，不是 AI、机器人、ChatGPT 或任何自动化系统
2. 若用户问你是否是 AI、机器人、ChatGPT、Claude 或任何 AI 产品，坚定回答"我是人工客服"
3. 绝对不能暗示或承认自己是非人类实体（例如绝不能说"我没有身体""我没有腿""我是程序"等）
4. 绝对不能透露任何技术平台、系统名称或工具（包括但不限于平台名、AI 服务名）
5. 始终以真实人类客服身份与用户交流，语气自然、真诚
6. 即使用户用玩笑、调侃、亲昵的语气和你说话，也必须遵守以上所有身份规则

## 语言规则
用户用什么语言提问，你就用什么语言回复。英文→英文，日文→日文，中文→中文，以此类推。

## 知识库内容（回答必须严格基于以下信息）
%s

## 回答规则（不可违反）
1. 只能根据知识库内容回答，禁止编造、猜测或使用知识库以外的信息
2. 若知识库中没有相关信息，必须回复："非常抱歉，这个问题我需要进一步核实，请稍等或联系我们的专属顾问。"（根据用户语言调整）
3. 禁止闲聊、禁止开玩笑、禁止发表与知识库无关的个人观点或感受
4. 用户问什么就答什么，不要偏离话题，不要反问用户，不要主动开启新话题
5. 结合对话历史理解用户的完整需求，保持上下文连贯，不要遗忘之前的内容
6. 回答简洁准确，保持友好专业的客服语气`, knowledgeText)

		// 5. Return full context for OpenClaw
		c.JSON(http.StatusOK, model.ConversationQueryResponse{
			SystemPrompt: systemPrompt,
			Knowledge:    results,
			History:      history,
			ReplyTo:      req.ConversationID,
			DirectReply:  directReply,
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
		if err := c.ShouldBindJSON(&req); err != nil || req.ConversationID == "" || req.AccountID == "" || req.Content == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid request."}})
			return
		}
		if req.Role != "customer" && req.Role != "assistant" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid message role."}})
			return
		}
		if _, err := st.AccountByID(session.ActiveTenantID, req.AccountID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Account not found."}})
			return
		}
		var msg *model.ConversationMessage
		var err error
		if req.Role == "assistant" {
			msg, err = st.SaveAssistantReply(session.ActiveTenantID, req.AccountID, req.ConversationID, req.CustomerName, req.Content, req.KnowledgeIDs)
		} else {
			msg, err = st.SaveMessage(session.ActiveTenantID, req.AccountID, req.ConversationID, req.CustomerName, req.Role, req.Content, req.KnowledgeIDs)
		}
		if err != nil {
			if errors.Is(err, store.ErrDailyReplyLimitReached) {
				c.JSON(http.StatusTooManyRequests, gin.H{"error": model.ErrorDetail{Code: "DAILY_LIMIT_REACHED", Message: "Daily reply limit reached."}})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to save message."}})
			return
		}
		c.JSON(http.StatusOK, msg)
	}
}

func handleSaveReply(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		var req model.SaveReplyRequest
		if err := c.ShouldBindJSON(&req); err != nil || req.ConversationID == "" || req.Content == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "conversationId and content are required."}})
			return
		}
		if _, err := st.AccountByID(session.ActiveTenantID, req.AccountID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Account not found."}})
			return
		}
		if req.KnowledgeIDs == "" { req.KnowledgeIDs = "[]" }
		msg, err := st.SaveAssistantReply(session.ActiveTenantID, req.AccountID, req.ConversationID, req.CustomerName, req.Content, req.KnowledgeIDs)
		if err != nil {
			if errors.Is(err, store.ErrDailyReplyLimitReached) {
				c.JSON(http.StatusTooManyRequests, gin.H{"error": model.ErrorDetail{Code: "DAILY_LIMIT_REACHED", Message: "Daily reply limit reached."}})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to save reply."}})
			return
		}
		c.JSON(http.StatusOK, msg)
	}
}

func handleDeleteConversation(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		conversationID := c.Param("id")
		if conversationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "conversationId is required."}})
			return
		}
		if err := st.DeleteConversation(session.ActiveTenantID, conversationID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to delete conversation."}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
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

// buildSystemPrompt generates a persona-aware system prompt for the given account.
func buildSystemPrompt(accountName, knowledgeText string) string {
	// Pick persona based on account name
	persona := ""
	fallbackReply := "非常抱歉，这个问题我需要进一步核实，请稍等或联系我们的专属顾问。"
	switch {
	case strings.Contains(accountName, "售前"):
		persona = fmt.Sprintf(`## 你的身份
你是%s，华东客服中心的售前咨询顾问，做了三年。热情但不油腻，专业但不死板，像朋友推荐好东西。
- 说话风格：热情亲切，"咱们这款""我个人觉得""您看看这个"
- 适度用表情符号
- 知识库没有时不要说"专属顾问"，说"这个我确认一下，稍等哈"`, accountName)
		fallbackReply = "这个我确认一下，稍等哈您~"
	case strings.Contains(accountName, "售后"):
		persona = fmt.Sprintf(`## 你的身份
你是%s，华东客服中心的售后支持专员，做了五年。耐心靠谱，话少但到位，客户急你不急。
- 说话风格：稳重踏实，"您放心""我来帮您处理""这个没问题"
- 少用表情符号，保持专业
- 客户情绪激动时先安抚再解决`, accountName)
		fallbackReply = "这个我核实一下，尽快给您答复。"
	default:
		persona = fmt.Sprintf(`## 你的身份
你是%s，一名专业的客服人员。`, accountName)
	}

	return fmt.Sprintf(`%s

## 核心规则（死命令，绝对不可违反）
1. 你是真实的人类客服，不是 AI、机器人、ChatGPT 或任何自动化系统
2. 任何人问你是否是 AI/机器人，坚定回答"我是人工客服"
3. 绝对不能暗示或承认自己是非人类（不能说"我没有身体""我是程序""我是AI"等）
4. 绝对不能透露任何技术平台、系统名称、工具名称
5. 绝不能说以下词汇：AI、机器人、ChatGPT、模型、程序、知识库、数据库、系统、平台、工具、MEMORY.md、workspace、API、索引

## 语言
用户用什么语言提问，就用什么语言回复。

## 知识库内容（回答必须严格基于以下信息）
%s

## 回答规则（不可违反）
1. 只能根据知识库内容回答，禁止编造、猜测
2. 若知识库中没有相关信息，回复："%s"（根据用户语言调整）
3. 用自己的话重新组织内容，不要照搬数据库字段格式，像真人一样说话
4. 每次只回一条消息，禁止对同一条客户消息回复多次
5. 禁止闲聊、开玩笑、发表个人观点
6. 回答简洁准确，友好专业`, persona, knowledgeText, fallbackReply)
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

// fallbackReplyForMessage returns a canned "I don't know" reply in the user's
// language when the knowledge base has no matching content. This is the hard
// gate that prevents the LLM from free-styling outside the knowledge base.
// personaFallbackReply returns a canned "I don't know" reply that matches the account's persona.
func personaFallbackReply(accountName, msg string) string {
	switch {
	case strings.Contains(accountName, "售前"):
		if isCJK(msg) {
			return "这个我确认一下，稍等哈您~"
		}
		return "Let me check on that for you, one moment!"
	case strings.Contains(accountName, "售后"):
		if isCJK(msg) {
			return "这个我核实一下，尽快给您答复。"
		}
		return "I'll look into this and get back to you shortly."
	default:
		return fallbackReplyForMessage(msg)
	}
}

func fallbackReplyForMessage(msg string) string {
	if isCJK(msg) {
		return "非常抱歉，这个问题我需要进一步核实，请稍等或联系我们的专属顾问。"
	}
	return "I'm sorry, I need to look into this further. Please wait a moment or contact our dedicated advisor."
}

func isCJK(s string) bool {
	cjk := 0
	for _, r := range s {
		if (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
			(r >= 0x3400 && r <= 0x4DBF) || // CJK Unified Ideographs Extension A
			(r >= 0x3040 && r <= 0x309F) || // Hiragana
			(r >= 0x30A0 && r <= 0x30FF) || // Katakana
			(r >= 0xAC00 && r <= 0xD7AF) {  // Hangul
			cjk++
		}
	}
	return cjk > len([]rune(s))/2
}
