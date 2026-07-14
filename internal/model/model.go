package model

// -- request / response types --

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type SelectTenantRequest struct {
	TenantID string `json:"tenantId"`
}

type CreateTenantRequest struct {
	Name string `json:"name"`
}

type CreateAccountRequest struct {
	Name       string   `json:"name"`
	DailyLimit *int     `json:"dailyLimit"`
	KbID       []string `json:"kbId"`
	ReplyLimit int      `json:"replyLimit"`
}

type CreateKnowledgeRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type InviteMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type AcceptInvitationRequest struct {
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Password    string `json:"password"`
}

type UpdateMemberRequest struct {
	Role   string `json:"role,omitempty"`
	Status string `json:"status,omitempty"`
}

type UpdateTenantStatusRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type UpdateAccountRequest struct {
	Name       string   `json:"name,omitempty"`
	KbID       []string `json:"kbId,omitempty"`
	Status     string   `json:"status,omitempty"`
	DailyLimit *int     `json:"dailyLimit,omitempty"`
	ReplyLimit *int     `json:"replyLimit,omitempty"`
}

// -- OpenClaw QR login --

type QrLoginResponse struct {
	QrData    string `json:"qrData"`
	ExpiresAt string `json:"expiresAt"`
	AccountID string `json:"accountId"`
}

type AccountStatusResponse struct {
	Status      string `json:"status"`
	ConnectedAt string `json:"connectedAt,omitempty"`
	QrData      string `json:"qrData,omitempty"`
	ExpiresAt   string `json:"expiresAt,omitempty"`
	Error       string `json:"error,omitempty"`
}

type ErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId,omitempty"`
}

// -- session --

type Session struct {
	CSRFToken      string `json:"csrfToken"`
	ActiveTenantID string `json:"activeTenantId"`
	User           User   `json:"user"`
}

func (s Session) IsPlatformAdmin() bool {
	return s.User.PlatformRole == "platform_admin"
}

// -- user --

type User struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	DisplayName  string `json:"displayName"`
	PlatformRole string `json:"platformRole,omitempty"`
}

// -- tenant (with membership info) --

type TenantWithMembership struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Status           string   `json:"status"`
	Role             string   `json:"role,omitempty"`
	MembershipStatus string   `json:"membershipStatus,omitempty"`
	Permissions      []string `json:"permissions,omitempty"`
}

func PermissionsForRole(role string) []string {
	switch role {
	case "owner", "admin":
		return []string{"accounts:manage", "knowledge:manage", "members:manage"}
	case "agent":
		return []string{"accounts:manage", "knowledge:manage"}
	default:
		return nil
	}
}

// -- tenant list response --

type TenantsResponse struct {
	PlatformRole string                 `json:"platformRole"`
	Tenants      []TenantWithMembership `json:"tenants"`
}

// -- member --

type Member struct {
	UserID      string `json:"userId"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	CreatedAt   string `json:"createdAt"`
}

type MembersResponse struct {
	Members []Member `json:"members"`
}

// -- invitation --

type Invitation struct {
	Token string `json:"token"`
	Email string `json:"email"`
}

type InviteResponse struct {
	Invitation Invitation `json:"invitation"`
}

// -- account --

type Account struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	AccountKey string   `json:"accountKey"`
	Status     string   `json:"status"`
	DailyLimit int      `json:"dailyLimit"`
	KbID       []string `json:"kbId"`
	ReplyLimit int      `json:"replyLimit"`
	CreatedAt  string   `json:"createdAt"`
}

type AccountsResponse struct {
	Accounts []Account `json:"accounts"`
}

// -- knowledge base --

type KnowledgeBase struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
	CreatedAt   string `json:"createdAt"`
}

type KnowledgeBasesResponse struct {
	Bases []KnowledgeBase `json:"bases"`
}

type UpdateKnowledgeRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Status      *string `json:"status,omitempty"`
}

// -- knowledge article --

type KnowledgeArticle struct {
	ID              string `json:"id"`
	KnowledgeBaseID string `json:"knowledgeBaseId"`
	Title           string `json:"title"`
	Content         string `json:"content"`
	Category        string `json:"category"`
	Attributes      string `json:"attributes"`
	Status          string `json:"status"`
	CreatedAt       string `json:"createdAt"`
	UpdatedAt       string `json:"updatedAt"`
}

type KnowledgeArticlesResponse struct {
	Articles []KnowledgeArticle `json:"articles"`
}

type CreateArticleRequest struct {
	Title      string `json:"title"`
	Content    string `json:"content"`
	Category   string `json:"category"`
	Attributes string `json:"attributes"`
}

type UpdateArticleRequest struct {
	Title      *string `json:"title,omitempty"`
	Content    *string `json:"content,omitempty"`
	Category   *string `json:"category,omitempty"`
	Attributes *string `json:"attributes,omitempty"`
	Status     *string `json:"status,omitempty"`
}

// -- knowledge search --

type SearchRequest struct {
	Query     string    `json:"query"`
	Embedding []float32 `json:"embedding,omitempty"` // optional vector for pgvector search
	Limit     int       `json:"limit"`
}

type SearchResultItem struct {
	ID                string  `json:"id"`
	Title             string  `json:"title"`
	Content           string  `json:"content"`
	Category          string  `json:"category"`
	Attributes        string  `json:"attributes"`
	KnowledgeBaseName string  `json:"knowledgeBaseName"`
	Score             float64 `json:"score"`
}

type SearchResponse struct {
	Results []SearchResultItem `json:"results"`
}

// -- conversation messages --

type ConversationMessage struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversationId"`
	AccountID      string `json:"accountId"`
	CustomerName   string `json:"customerName"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	KnowledgeIDs   string `json:"knowledgeIds"`
	CreatedAt      string `json:"createdAt"`
}

type ConversationMessagesResponse struct {
	Messages []ConversationMessage `json:"messages"`
}

type SaveMessageRequest struct {
	ConversationID string `json:"conversationId"`
	AccountID      string `json:"accountId"`
	CustomerName   string `json:"customerName"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	KnowledgeIDs   string `json:"knowledgeIds"`
}

// -- save reply --

type SaveReplyRequest struct {
	ConversationID string `json:"conversationId"`
	AccountID      string `json:"accountId"`
	CustomerName   string `json:"customerName"`
	Content        string `json:"content"`
	KnowledgeIDs   string `json:"knowledgeIds"`
}

// -- RAG query --

type ConversationQueryRequest struct {
	ConversationID string           `json:"conversationId"`
	CustomerName   string           `json:"customerName"`
	Message        string           `json:"message"`
	AccountID      string           `json:"accountId"`
	MaxHistory     int              `json:"maxHistory"`
	MaxKnowledge   int              `json:"maxKnowledge"`
	// History provided by OpenClaw (its own stored messages, chronological order).
	// When present, used directly as context and persisted locally for traceability.
	// When absent, history is loaded from the local database.
	History        []OpenClawMessage `json:"history,omitempty"`
}

// OpenClawMessage is a single message entry sent by OpenClaw in the history field.
type OpenClawMessage struct {
	Role      string `json:"role"`    // "user" or "assistant"
	Content   string `json:"content"`
	Timestamp string `json:"timestamp,omitempty"` // ISO-8601, optional
}

type ConversationQueryResponse struct {
	SystemPrompt string             `json:"systemPrompt"`
	Knowledge    []SearchResultItem `json:"knowledge"`
	History      []HistoryMessage   `json:"history"`
	ReplyTo      string             `json:"replyTo"`
	// DirectReply is set when the backend provides a canned fallback response
	// (e.g. when knowledge search returns empty). When non-empty, the caller
	// should skip the LLM and send this text directly to the user.
	DirectReply string `json:"directReply,omitempty"`
}

type HistoryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// -- conversation list (aggregated from messages) --

type ConversationSummary struct {
	ConversationID string `json:"conversationId"`
	CustomerName   string `json:"customerName"`
	AccountID      string `json:"accountId"`
	LastMessage    string `json:"lastMessage"`
	LastMessageAt  string `json:"lastMessageAt"`
	MessageCount   int    `json:"messageCount"`
}

type ConversationListResponse struct {
	Conversations []ConversationSummary `json:"conversations"`
}

// -- legacy conversation (backward compat) --

type Conversation struct {
	ID            string `json:"id"`
	AccountID     string `json:"accountId"`
	Customer      string `json:"customer"`
	LastMessage   string `json:"lastMessage"`
	Status        string `json:"status"`
	LastMessageAt string `json:"lastMessageAt"`
}

type ConversationsResponse struct {
	Conversations []Conversation `json:"conversations"`
}

// -- create tenant response --

type CreateTenantResponse struct {
	Tenant      TenantWithMembership `json:"tenant"`
	Credentials struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	} `json:"credentials"`
}

// -- accept invitation response --

type AcceptInvitationResponse struct {
	TenantID  string `json:"tenantId"`
	CSRFToken string `json:"csrfToken"`
}

// -- internal DB row types (timestamps are strings because SQLite stores TEXT) --

type SessionRow struct {
	ID             string
	UserID         string
	CSRFToken      string
	ActiveTenantID string
	ExpiresAt      string
}

type UserRow struct {
	ID           string
	Email        string
	DisplayName  string
	PasswordHash string
	PlatformRole string
	CreatedAt    string
}

type TenantRow struct {
	ID        string
	Name      string
	Status    string
	CreatedAt string
}

type TenantMemberRow struct {
	TenantID string
	UserID   string
	Role     string
	Status   string
}

type InvitationRow struct {
	ID        string
	Token     string
	TenantID  string
	Email     string
	Role      string
	ExpiresAt string
	CreatedAt string
}

type AccountRow struct {
	ID         string
	TenantID   string
	Name       string
	AccountKey string
	Status     string
	DailyLimit int
	KbID       string
	ReplyLimit int
	CreatedAt  string
}

type KnowledgeRow struct {
	ID          string
	TenantID    string
	Name        string
	Description string
	Status      string
	CreatedAt   string
}

type KnowledgeArticleRow struct {
	ID              string
	KnowledgeBaseID string
	Title           string
	Content         string
	Category        string
	Attributes      string
	Status          string
	CreatedAt       string
	UpdatedAt       string
}

type ConversationRow struct {
	ID            string
	TenantID      string
	AccountID     string
	Customer      string
	LastMessage   string
	Status        string
	LastMessageAt string
}
