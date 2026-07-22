package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"whatsapp-ai-poc/internal/middleware"
	"whatsapp-ai-poc/internal/model"
	"whatsapp-ai-poc/internal/store"
)

var sendWhatsAppReply = sendOpenClawWhatsAppMessage

// maxHistoryCap limits how much conversation history a single caller can
// request. Above this the cost of loading, transferring and asking the LLM to
// consume history grows without improving answer quality.
const maxHistoryCap = 50

// maxKnowledgeCap bounds the number of retrieved knowledge chunks returned per
// query. The embedding search itself is cheap but each result is fed verbatim
// into the system prompt, so an unbounded limit would balloon token cost and
// confuse the model.
const maxKnowledgeCap = 50

// retrievalTokenSalt is mixed into the retrieval token hash so that the token
// cannot be trivially recomputed from outside (it does not have to be a
// secret: the token is only an idempotency key, but a stable non-guessable
// value prevents clients from colliding on each other's tokens).
const retrievalTokenSalt = "whatsapp-ai:retrieval:2026-07"

// clampNonPositive returns fallback when value is zero/negative, and cap when
// value exceeds cap. Used for MaxHistory / MaxKnowledge query parameters.
func clampNonPositive(value, fallback, cap int) int {
	if value <= 0 {
		return fallback
	}
	if value > cap {
		return cap
	}
	return value
}

// effectiveSearchQuery returns the search query that should be used for
// knowledge-base retrieval: an explicit SearchQuery when provided, falling
// back to the raw customer Message. Agent C (rag-mcp) may pre-extract a
// cleaner query from the user message before calling the query endpoint;
// when it does not, the message itself is still a reasonable search input.
func effectiveSearchQuery(req *model.ConversationQueryRequest) string {
	if q := strings.TrimSpace(req.SearchQuery); q != "" {
		return q
	}
	return req.Message
}

// generateRetrievalToken returns a deterministic-but-unique idempotency token
// for one (conversationId, timestamp) pair. The timestamp is second-granular
// which is sufficient to distinguish successive queries while collapsing
// legitimate retries of the SAME query (within the same second) into one
// reply. The hash is salted so the token is not equal to a raw conversationId
// (which a client could guess).
func generateRetrievalToken(conversationID string, now time.Time) string {
	h := sha256.New()
	h.Write([]byte(conversationID))
	h.Write([]byte{0})
	// Second granularity: retries within the same second collapse, but a
	// genuine follow-up query a few seconds later gets a fresh token.
	ts := now.Unix()
	tsBytes := []byte(fmt.Sprintf("%d|%s", ts, retrievalTokenSalt))
	h.Write(tsBytes)
	sum := h.Sum(nil)
	return hex.EncodeToString(sum[:16])
}

func RegisterConversations(r *gin.RouterGroup, st *store.Store) {
	RegisterConversationRead(r, st)
	RegisterConversationReply(r, st)
	RegisterConversationAdministration(r, st)
}

// RegisterConversationRead registers conversation history endpoints available
// to all active tenant members.
func RegisterConversationRead(r *gin.RouterGroup, st *store.Store) {
	r.GET("", handleListConversations(st))
	r.GET("/:id/messages", handleMessages(st))
}

// RegisterConversationReply registers the mutations used to answer customers.
func RegisterConversationReply(r *gin.RouterGroup, st *store.Store) {
	r.POST("/query", handleConversationQuery(st))
	r.POST("/messages", handleSaveMessage(st))
	r.POST("/reply", handleSaveReply(st))
	r.POST("/:id/send", handleSendConversationReply(st))
}

// RegisterConversationAdministration registers destructive conversation work.
func RegisterConversationAdministration(r *gin.RouterGroup, st *store.Store) {
	r.DELETE("/:id", handleDeleteConversation(st))
}

func handleSendConversationReply(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		var req model.SendConversationReplyRequest
		if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.AccountID) == "" || strings.TrimSpace(req.Content) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "accountId and content are required."}})
			return
		}
		conversationID := c.Param("id")
		if _, err := normalizeWhatsAppTarget(conversationID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: err.Error()}})
			return
		}
		account, err := st.AccountByID(session.ActiveTenantID, req.AccountID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Account not found."}})
			return
		}
		if account.Status != "connected" {
			c.JSON(http.StatusConflict, gin.H{"error": model.ErrorDetail{Code: "OPENCLAW_ERROR", Message: "WhatsApp account is not connected."}})
			return
		}
		var deliveryErr error
		message, err := st.DeliverAndSaveAssistantReply(session.ActiveTenantID, account.ID, conversationID, req.CustomerName, req.Content, "[]", func() error {
			deliveryErr = sendWhatsAppReply(account.AccountKey, conversationID, req.Content)
			return deliveryErr
		})
		if err != nil {
			if deliveryErr != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": model.ErrorDetail{Code: "OPENCLAW_ERROR", Message: deliveryErr.Error()}})
				return
			}
			if errors.Is(err, store.ErrDailyReplyLimitReached) {
				c.JSON(http.StatusTooManyRequests, gin.H{"error": model.ErrorDetail{Code: "DAILY_LIMIT_REACHED", Message: "Daily reply limit reached."}})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to send and save message."}})
			return
		}
		c.JSON(http.StatusOK, message)
	}
}

// RegisterInternalConversations registers conversation routes for internal
// service-to-service calls (bearer token auth). Tenant is resolved from the
// accountId in the request body rather than from the session.
func RegisterInternalConversations(r *gin.RouterGroup, st *store.Store) {
	r.POST("/query", handleInternalConversationQuery(st))
	r.POST("/reply", handleInternalSaveReply(st))
}

// resolveTenantFromAccount looks up the tenant that owns the given account.
func resolveTenantFromAccount(st *store.Store, accountID string) (string, error) {
	return st.ActiveTenantIDByAccountID(accountID)
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

func validConversationHistory(history []model.OpenClawMessage) bool {
	for _, message := range history {
		if (message.Role != "user" && message.Role != "assistant") || strings.TrimSpace(message.Content) == "" {
			return false
		}
	}
	return true
}

func handleInternalConversationQuery(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.ConversationQueryRequest
		if err := c.ShouldBindJSON(&req); err != nil || req.Message == "" || req.ConversationID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "message and conversationId are required."}})
			return
		}
		req.MaxHistory = clampNonPositive(req.MaxHistory, 10, maxHistoryCap)
		req.MaxKnowledge = clampNonPositive(req.MaxKnowledge, 5, maxKnowledgeCap)
		if !validConversationHistory(req.History) {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "History contains an invalid message."}})
			return
		}

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

		// 1. Save customer message (idempotent on retries: same role+content in
		//    this conversation collapses to one row via SaveMessageIfAbsent's
		//    partial unique index. Previously a 5xx on later steps doubled it.)
		if err := st.SaveMessageIfAbsent(tenantID, req.AccountID, req.ConversationID, req.CustomerName, "customer", req.Message, "[]"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to save message."}})
			return
		}

		// 2. Search only the knowledge bases bound to this account. Use the
		//    explicit SearchQuery when provided (Agent C may extract a cleaner
		//    query from the raw message); otherwise fall back to the raw
		//    customer message.
		searchQuery := effectiveSearchQuery(&req)
		results, err := st.SearchKnowledgeForBases(tenantID, accountKnowledgeBaseIDs(acctRow), searchQuery, st.EmbedTexts([]string{searchQuery}), req.MaxKnowledge)
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
				if role == "user" {
					role = "customer"
				}
				if err := st.SaveMessageIfAbsent(tenantID, req.AccountID, req.ConversationID, req.CustomerName, role, m.Content, "[]"); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to save conversation history."}})
					return
				}
				history = append(history, model.HistoryMessage{Role: m.Role, Content: m.Content})
			}
		} else {
			msgs, err := st.LoadHistory(tenantID, req.AccountID, req.ConversationID, req.MaxHistory)
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
		systemPrompt := buildSystemPrompt(accountName, knowledgeText, req.Message)

		c.JSON(http.StatusOK, model.ConversationQueryResponse{
			SystemPrompt:   systemPrompt,
			Knowledge:      results,
			History:        history,
			ReplyTo:        req.ConversationID,
			DirectReply:    directReply,
			RetrievalToken: generateRetrievalToken(req.ConversationID, time.Now()),
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
		if req.KnowledgeIDs == "" {
			req.KnowledgeIDs = "[]"
		}

		// Idempotency: when the caller supplies the RetrievalToken returned by
		// the originating /query call, look up an existing assistant reply
		// persisted with that token first. If found, return it without
		// inserting again so Agent C retries (timeout, network blip) do not
		// double-count the daily quota or duplicate the message in history.
		//
		// The lookup uses the conversation_messages.message_id column, which
		// already has a partial unique index (idx_conv_msg_dedup) so the same
		// token cannot be served by two rows. The store method is implemented
		// by Agent A in pg.go as SaveAssistantReplyWithToken.
		if token := strings.TrimSpace(req.RetrievalToken); token != "" {
			if existing, lookupErr := st.AssistantReplyByRetrievalToken(tenantID, req.ConversationID, token); lookupErr == nil && existing != nil {
				slog.Info("save_reply idempotent reuse",
					"tenant_id", tenantID,
					"conversation_id", req.ConversationID,
					"retrieval_token", token,
					"message_id", existing.ID,
				)
				c.JSON(http.StatusOK, existing)
				return
			} else if lookupErr != nil && !errors.Is(lookupErr, pgx.ErrNoRows) {
				slog.Warn("save_reply idempotency lookup failed",
					"tenant_id", tenantID,
					"conversation_id", req.ConversationID,
					"retrieval_token", token,
					"err", lookupErr,
				)
				// Fall through to insert path; dedup is best-effort on this
				// error path so we never block a legitimate save.
			}
			msg, saveErr := st.SaveAssistantReplyWithToken(tenantID, req.AccountID, req.ConversationID, req.CustomerName, req.Content, req.KnowledgeIDs, token)
			if saveErr != nil {
				if errors.Is(saveErr, store.ErrDailyReplyLimitReached) {
					c.JSON(http.StatusTooManyRequests, gin.H{"error": model.ErrorDetail{Code: "DAILY_LIMIT_REACHED", Message: "Daily reply limit reached."}})
					return
				}
				c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to save reply."}})
				return
			}
			c.JSON(http.StatusOK, msg)
			return
		}

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

func handleListConversations(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		accountID := c.Query("accountId")
		// Page size caps prevent unbounded result sets. Default 50, max 100;
		// offset is non-negative. Unknown / non-integer values fall back to
		// defaults instead of erroring so existing UI callers keep working.
		const (
			defaultLimit = 50
			maxLimit     = 100
		)
		limit := defaultLimit
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		if limit > maxLimit {
			limit = maxLimit
		}
		offset := 0
		if o := c.Query("offset"); o != "" {
			if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
				offset = parsed
			}
		}
		summaries, err := st.ListConversationSummaries(session.ActiveTenantID, accountID, limit, offset)
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
		req.MaxHistory = clampNonPositive(req.MaxHistory, 10, maxHistoryCap)
		req.MaxKnowledge = clampNonPositive(req.MaxKnowledge, 5, maxKnowledgeCap)
		if !validConversationHistory(req.History) {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "History contains an invalid message."}})
			return
		}
		account, err := st.AccountByID(session.ActiveTenantID, req.AccountID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Account not found."}})
			return
		}

		// 1. Save customer message (idempotent on retries; see SaveMessageIfAbsent).
		if err := st.SaveMessageIfAbsent(session.ActiveTenantID, req.AccountID, req.ConversationID, req.CustomerName, "customer", req.Message, "[]"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to save message."}})
			return
		}

		// 2. Search knowledge. Prefer an explicit SearchQuery (Agent C may
		//    pre-extract one); otherwise fall back to the raw customer message.
		searchQuery := effectiveSearchQuery(&req)
		results, err := st.SearchKnowledgeForBases(session.ActiveTenantID, accountKnowledgeBaseIDs(account), searchQuery, st.EmbedTexts([]string{searchQuery}), req.MaxKnowledge)
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
				if role == "user" {
					role = "customer"
				}
				if err := st.SaveMessageIfAbsent(session.ActiveTenantID, req.AccountID, req.ConversationID, req.CustomerName, role, m.Content, "[]"); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to save conversation history."}})
					return
				}
				history = append(history, model.HistoryMessage{Role: m.Role, Content: m.Content})
			}
		} else {
			// Fall back to local DB (reverse DESC result back to chronological order).
			msgs, err := st.LoadHistory(session.ActiveTenantID, req.AccountID, req.ConversationID, req.MaxHistory)
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
			systemPrompt := buildSystemPrompt(account.Name, knowledgeText, req.Message)

		// 5. Return full context for OpenClaw
		c.JSON(http.StatusOK, model.ConversationQueryResponse{
			SystemPrompt:   systemPrompt,
			Knowledge:      results,
			History:        history,
			ReplyTo:        req.ConversationID,
			DirectReply:    directReply,
			RetrievalToken: generateRetrievalToken(req.ConversationID, time.Now()),
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
		if req.KnowledgeIDs == "" {
			req.KnowledgeIDs = "[]"
		}
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
		accountID := c.Query("accountId")
		if conversationID == "" || accountID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "conversationId and accountId are required."}})
			return
		}
		if _, err := st.AccountByID(session.ActiveTenantID, accountID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Account not found."}})
			return
		}
		if err := st.DeleteConversation(session.ActiveTenantID, accountID, conversationID); err != nil {
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
		accountID := c.Query("accountId")
		if accountID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "accountId is required."}})
			return
		}
		account, err := st.AccountByID(session.ActiveTenantID, accountID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Account not found."}})
			return
		}
		requestedLimit := 0
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
				requestedLimit = parsed
			}
		}
		msgs, err := st.LoadHistory(session.ActiveTenantID, accountID, c.Param("id"), effectiveMessageLimit(requestedLimit, account.ReplyLimit))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to load messages."}})
			return
		}
		c.JSON(http.StatusOK, model.ConversationMessagesResponse{Messages: msgs})
	}
}

func effectiveMessageLimit(requested, accountLimit int) int {
	if accountLimit <= 0 {
		accountLimit = 30
	}
	if requested <= 0 {
		requested = 30
	}
	if requested > accountLimit {
		return accountLimit
	}
	return requested
}

// buildSystemPrompt generates a persona-aware system prompt for the given account,
// with mandatory language instruction based on the user's message language.
func buildSystemPrompt(accountName, knowledgeText, userMsg string) string {
	// Detect language from user message
	lang := detectLanguage(userMsg)
	langInstruction := buildLanguageInstruction(lang)

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

%s

## 知识库内容（回答必须严格基于以下信息）
%s

## 回答规则（不可违反）
1. 只能根据知识库内容回答，禁止编造、猜测
2. 若知识库中没有相关信息，回复："%s"（根据用户语言调整）
3. 用自己的话重新组织内容，不要照搬数据库字段格式，像真人一样说话
4. 每次只回一条消息，禁止对同一条客户消息回复多次
5. 禁止闲聊、开玩笑、发表个人观点
6. 回答简洁准确，友好专业`, persona, langInstruction, knowledgeText, fallbackReply)
}

func buildKnowledgeContext(results []model.SearchResultItem) string {
	if len(results) == 0 {
		return "（暂无相关知识库内容）"
	}
	var sb strings.Builder
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("\n### %d. %s [%s]\n", i+1, r.Title, r.KnowledgeBaseName))
		if r.Category != "" {
			sb.WriteString(fmt.Sprintf("分类: %s\n", r.Category))
		}
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
	lang := detectLanguage(msg)
	switch {
	case strings.Contains(accountName, "售前"):
		switch lang {
		case "zh":
			return "这个我确认一下，稍等哈您~"
		case "ja":
			return "それについて確認いたしますね、少々お待ちください！"
		case "ko":
			return "확인해 드릴게요, 잠시만요!"
		case "ar":
			return "سأتحقق من ذلك لك، لحظة واحدة!"
		case "ru":
			return "Проверю это для вас, один момент!"
		case "de":
			return "Ich check das mal für Sie, einen Moment!"
		case "fr":
			return "Laissez-moi vérifier cela pour vous, un instant!"
		case "es":
			return "Déjame verificar eso por ti, un momento!"
		case "pt":
			return "Deixa eu verificar isso para você, um momento!"
		default:
			return "Let me check on that for you, one moment!"
		}
	case strings.Contains(accountName, "售后"):
		switch lang {
		case "zh":
			return "这个我核实一下，尽快给您答复。"
		case "ja":
			return "この件について確認し、早急にご返信いたします。"
		case "ko":
			return "이를 확인하고 최대한 빨리 답변 드리겠습니다."
		case "ar":
			return "سأقوم بالتحقق من هذا والرد عليك في أقرب وقت."
		case "ru":
			return "Я проверю это и как можно скорее свяжусь с вами."
		case "de":
			return "Ich werde das prüfen und mich schnellstmöglich bei Ihnen melden."
		case "fr":
			return "Je vais examiner cela et vous répondre sous peu."
		case "es":
			return "Voy a investigar esto y te responderé lo antes posible."
		case "pt":
			return "Vou verificar isso e retornar o mais rápido possível."
		default:
			return "I'll look into this and get back to you shortly."
		}
	default:
		return fallbackReplyForMessage(msg)
	}
}

func fallbackReplyForMessage(msg string) string {
	lang := detectLanguage(msg)
	switch lang {
	case "zh":
		return "非常抱歉，这个问题我需要进一步核实，请稍等或联系我们的专属顾问。"
	case "ja":
		return "申し訳ございませんが、この件についてはさらに確認が必要です。しばらくお待ちいただくか、専任のアドバイザーにお問い合わせください。"
	case "ko":
		return "죄송하지만 이 문제는 추가 확인이 필요합니다. 잠시 기다려 주시거나 전용 어드바이저에게 문의해 주세요."
	case "ar":
		return "أعتذر، أحتاج إلى التحقق من هذا الأمر أكثر. يرجى الانتظار قليلاً أو الاتصال بمستشارينا المخصصين."
	case "ru":
		return "Приношу извинения, мне нужно дополнительно уточнить этот вопрос. Пожалуйста, подождите немного или обратитесь к нашему специальному консультанту."
	case "de":
		return "Entschuldigung, ich muss dies noch einmal überprüfen. Bitte warten Sie einen Moment oder wenden Sie sich an unseren spezialisierten Berater."
	case "fr":
		return "Je vous prie de m'excuser, je dois vérifier cela. Veuillez patienter un instant ou contacter notre conseiller dédié."
	case "es":
		return "Disculpe, necesito verificar esto más a fondo. Por favor, espere un momento o contacte a nuestro asesor dedicado."
	case "pt":
		return "Desculpe, preciso verificar isso mais detalhadamente. Por favor, aguarde um momento ou entre em contato com nosso consultor dedicado."
	default:
		return "I'm sorry, I need to look into this further. Please wait a moment or contact our dedicated advisor."
	}
}

// detectLanguage detects the language of the input message and returns
// a language code. Supported languages: en, zh, ja, ko, ar, ru, de, fr, es, pt.
// Defaults to "en" (English) when detection fails or input is empty.
func detectLanguage(s string) string {
	if s == "" {
		return "en"
	}

	// Count characters in each Unicode range
	cjk, arabic, cyrillic, hiragana, katakana, hangul := 0, 0, 0, 0, 0, 0
	runes := []rune(s)
	total := len(runes)

	// First pass: count Unicode ranges
	for _, r := range runes {
		switch {
		case (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3400 && r <= 0x4DBF):
			// CJK Unified Ideographs (includes Chinese)
			cjk++
		case r >= 0x3040 && r <= 0x309F:
			// Hiragana (Japanese)
			hiragana++
		case r >= 0x30A0 && r <= 0x30FF:
			// Katakana (Japanese)
			katakana++
		case r >= 0xAC00 && r <= 0xD7AF:
			// Hangul (Korean)
			hangul++
		case (r >= 0x0600 && r <= 0x06FF) || (r >= 0x0750 && r <= 0x077F):
			// Arabic
			arabic++
		case r >= 0x0400 && r <= 0x04FF:
			// Cyrillic (Russian)
			cyrillic++
		}
	}

	// Threshold: more than 10% of text in a specific script
	threshold := total / 10

	// Check Japanese-specific scripts first (they have higher priority)
	if hiragana > threshold || katakana > threshold {
		return "ja"
	}

	// Check Korean
	if hangul > threshold {
		return "ko"
	}

	// Check Chinese (CJK ideographs without Japanese/Korean scripts)
	if cjk > threshold && (hiragana+katakana+hangul) <= threshold {
		return "zh"
	}

	// Check Arabic
	if arabic > threshold {
		return "ar"
	}

	// Check Cyrillic (Russian, Ukrainian, etc.)
	if cyrillic > threshold {
		return "ru"
	}

	// For Latin-script languages, use keyword matching
	lower := strings.ToLower(s)

	// Check German
	germanKeywords := []string{"der", "die", "das", "ist", "nicht", "und", "auch", "noch"}
	germanCount := 0
	for _, kw := range germanKeywords {
		if strings.Contains(lower, kw) {
			germanCount++
		}
	}
	if germanCount >= 2 {
		return "de"
	}

	// Check French
	frenchKeywords := []string{"le", "la", "les", "est", "et", "vous", "être", "avoir"}
	frenchCount := 0
	for _, kw := range frenchKeywords {
		if strings.Contains(lower, kw) {
			frenchCount++
		}
	}
	if frenchCount >= 2 {
		return "fr"
	}

	// Check Spanish
	spanishKeywords := []string{"el", "la", "los", "las", "por", "para", "que", "con"}
	spanishCount := 0
	for _, kw := range spanishKeywords {
		if strings.Contains(lower, kw) {
			spanishCount++
		}
	}
	if spanishCount >= 2 {
		return "es"
	}

	// Check Portuguese
	portugueseKeywords := []string{"o", "a", "os", "as", "por", "para", "que", "com"}
	portugueseCount := 0
	for _, kw := range portugueseKeywords {
		if strings.Contains(lower, kw) {
			portugueseCount++
		}
	}
	if portugueseCount >= 2 {
		return "pt"
	}

	// Default to English
	return "en"
}

func isCJK(s string) bool {
	cjk := 0
	for _, r := range s {
		if (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
			(r >= 0x3400 && r <= 0x4DBF) || // CJK Unified Ideographs Extension A
			(r >= 0x3040 && r <= 0x309F) || // Hiragana
			(r >= 0x30A0 && r <= 0x30FF) || // Katakana
			(r >= 0xAC00 && r <= 0xD7AF) { // Hangul
			cjk++
		}
	}
	return cjk > len([]rune(s))/2
}

// buildLanguageInstruction returns a mandatory language instruction for the
// system prompt, forcing the LLM to respond in the detected language.
func buildLanguageInstruction(lang string) string {
	switch lang {
	case "zh":
		return "## 语言（死命令，绝对不可违反）\n用户正在使用【中文】与你交流，你必须用中文回复所有消息，禁止使用其他语言。"
	case "ja":
		return "## 言語（死命令、絶対に違反できない）\nユーザーは【日本語】であなたと通信しています。あなたは必ず日本語で返信しなければなりません。他の言語を使用することは禁止されています。"
	case "ko":
		return "## 언어（사망 명령, 절대 위반할 수 없음）\n사용자는 【한국어】로 당신과 소통하고 있습니다. 당신은 반드시 한국어로 모든 메시지를 응답해야 하며, 다른 언어를 사용하는 것이 금지되어 있습니다."
	case "ar":
		return "## اللغة（أمر قاتل، لا يمكن انتهاكه）\nالمستخدم يتواصل معك بال【لغة العربية】. يجب عليك الرد باللغة العربية فقط، ويُمنع استخدام أي لغة أخرى."
	case "ru":
		return "## Язык（смертельный приказ, абсолютно не нарушаемый）\nПользователь общается с вами на【русском языке】. Вы должны отвечать на русском языке, использование других языков запрещено."
	case "de":
		return "## Sprache（Todesbefehl, absolut nicht verletzbar）\nDer Benutzer kommuniziert mit dir auf【Deutsch】. Du musst auf Deutsch antworten, die Verwendung anderer Sprachen ist verboten."
	case "fr":
		return "## Langue（ordre mortel, absolument inviolable）\nL'utilisateur communique avec vous en【français】. Vous devez répondre en français, l'utilisation d'autres langues est interdite."
	case "es":
		return "## Idioma（orden mortal, absolutamente inviolable）\nEl usuario se comunica con usted en【español】. Debe responder en español, está prohibido usar otros idiomas."
	case "pt":
		return "## Idioma（ordem mortal, absolutamente inviolável）\nO usuário está se comunicando com você em【português】. Você deve responder em português, o uso de outros idiomas é proibido."
	default:
		return "## Language (Mandatory, absolutely unbreakable)\nYou are communicating with the user in 【English】. You must respond in English only, the use of other languages is prohibited."
	}
}

// embedHealthURL is the URL /health/ready polls to confirm the local
// embedding service is up. It mirrors the EMBED_URL convention used by
// internal/embedding (default http://127.0.0.1:8090) but appends /health.
// The check is best-effort: a non-2xx response or a connection failure makes
// the readiness probe fail, which is exactly what we want the outer load
// balancer to see.
func embedHealthURL() string {
	u := strings.TrimSpace(os.Getenv("EMBED_URL"))
	if u == "" {
		u = "http://127.0.0.1:8090"
	}
	return strings.TrimRight(u, "/") + "/health"
}

// pingEmbedService performs a 2-second HTTP GET against the embed service.
// Any 2xx status is treated as healthy. The embed service is optional for
// keyword search, but when configured it must respond so RAG retrieval is
// not silently degraded.
func pingEmbedService() error {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(embedHealthURL())
	if err != nil {
		return fmt.Errorf("embed unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("embed health returned %d", resp.StatusCode)
	}
	return nil
}

// pingDatabase confirms the Postgres connection is responsive via an existing
// lightweight read. We deliberately use TenantByID with a sentinel ID rather
// than a raw "SELECT 1" because the handler package cannot access Store.pool
// directly (pg.go is owned by Agent A). ErrNoRows is treated as success: it
// proves the round-trip completed.
func pingDatabase(st *store.Store) error {
	_, err := st.TenantByID("healthcheck-nonexistent")
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return fmt.Errorf("db unreachable: %w", err)
}

// MakeReadyHandler returns the composite readiness handler. Each component
// (database, embed service, OpenClaw) is checked independently; the first
// failing component short-circuits and returns 503 with a JSON body naming
// it, so an operator can see at a glance which dependency is degraded. The
// handler returns 200 only when every component is healthy, which is the
// condition under which an outer load balancer should keep the pod in
// rotation.
//
// OpenClaw availability uses isOpenClawAvailable (defined in accounts.go in
// the same package) which simply checks the binary/docker container is
// reachable via PATH. This intentionally does NOT verify the gateway is
// actually serving traffic — a deeper check would race with gateway restarts
// and cause flapping. Lack of the binary means the deployment does not use
// OpenClaw at all, so we treat that as "not applicable / healthy".
func MakeReadyHandler(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		checks := map[string]string{}

		if err := pingDatabase(st); err != nil {
			checks["database"] = err.Error()
		} else {
			checks["database"] = "ok"
		}

		if err := pingEmbedService(); err != nil {
			checks["embed"] = err.Error()
		} else {
			checks["embed"] = "ok"
		}

		// isOpenClawAvailable is in accounts.go (same package). When the
		// OpenClaw binary is not installed the deployment simply does not
		// integrate with WhatsApp, so absence is reported as "n/a" rather
		// than failing readiness.
		if isOpenClawAvailable() {
			checks["openclaw"] = "ok"
		} else {
			checks["openclaw"] = "n/a"
		}

		healthy := checks["database"] == "ok" && checks["embed"] == "ok"
		status := http.StatusOK
		if !healthy {
			status = http.StatusServiceUnavailable
			slog.Warn("readiness check failed", "checks", checks)
		}
		statusStr := "ok"
		if !healthy {
			statusStr = "unhealthy"
		}
		c.JSON(status, gin.H{
			"status": statusStr,
			"checks": checks,
		})
	}
}
