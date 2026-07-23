package handler

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"whatsapp-ai-poc/internal/middleware"
	"whatsapp-ai-poc/internal/model"
	"whatsapp-ai-poc/internal/store"
)

func RegisterKnowledgeSearch(r *gin.RouterGroup, st *store.Store) {
	r.POST("/search", handleSearchKnowledge(st))
}

func RegisterKnowledge(r *gin.RouterGroup, st *store.Store) {
	RegisterKnowledgeRead(r, st)
	RegisterKnowledgeManagement(r, st)
}

// RegisterKnowledgeRead registers endpoints available to all active tenant members.
func RegisterKnowledgeRead(r *gin.RouterGroup, st *store.Store) {
	r.GET("/bases", handleListBases(st))
	r.GET("/bases/:id", handleGetBase(st))
	r.GET("/bases/:id/articles", handleListArticles(st))
}

// RegisterKnowledgeManagement registers mutations that require the
// knowledge:manage tenant permission.
func RegisterKnowledgeManagement(r *gin.RouterGroup, st *store.Store) {
	r.POST("/bases", handleCreateBase(st))
	r.PATCH("/bases/:id", handleUpdateBase(st))
	r.DELETE("/bases/:id", handleDeleteBase(st))
	r.POST("/bases/:id/articles", handleCreateArticle(st))
	r.POST("/bases/:id/import", handleImportArticles(st))
	r.PATCH("/bases/:id/articles/:articleId", handleUpdateArticle(st))
	r.DELETE("/bases/:id/articles/:articleId", handleDeleteArticle(st))
}

const (
	maxKnowledgeImportFiles    = 20
	maxKnowledgeImportArticles = 500
	maxKnowledgeImportBytes    = 5 << 20

	// maxJSONRequestBytes caps JSON request bodies on knowledge endpoints so a
	// huge payload cannot exhaust server memory before ShouldBindJSON parses.
	maxJSONRequestBytes = 1 << 20 // 1MB
	// maxArticleContentRunes bounds article content length; matches the chunk
	// slicing budget in the store so validated input always fits downstream.
	maxArticleContentRunes = 50000
	// maxSearchLimit caps how many results a single search request can return.
	maxSearchLimit = 50
	// maxImportMemoryBytes buffers only the first chunk of each upload in RAM;
	// larger uploads spill to disk temp files instead of 100MB in-process.
	maxImportMemoryBytes = 1 << 20 // 1MB
)

func handleImportArticles(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		knowledgeBaseID := c.Param("id")
		if _, err := st.KnowledgeBaseByID(knowledgeBaseID, session.ActiveTenantID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Knowledge base not found."}})
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxKnowledgeImportBytes*maxKnowledgeImportFiles)
		if err := c.Request.ParseMultipartForm(maxImportMemoryBytes); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "无法读取导入文件。"}})
			return
		}
		files := c.Request.MultipartForm.File["files"]
		if len(files) == 0 || len(files) > maxKnowledgeImportFiles {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: fmt.Sprintf("请选择 1 到 %d 个文件。", maxKnowledgeImportFiles)}})
			return
		}

		articles := make([]model.CreateArticleRequest, 0)
		for _, file := range files {
			if file.Size > maxKnowledgeImportBytes {
				c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: fmt.Sprintf("%s 超过 5MB 限制。", file.Filename)}})
				return
			}
			reader, err := file.Open()
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: fmt.Sprintf("无法读取 %s。", file.Filename)}})
			defer reader.Close()
				return
			}
			data, readErr := io.ReadAll(io.LimitReader(reader, maxKnowledgeImportBytes+1))
			if readErr != nil || len(data) > maxKnowledgeImportBytes {
				c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: fmt.Sprintf("%s 超过 5MB 限制。", file.Filename)}})
				return
			}
			parsed, err := parseKnowledgeImportFile(file.Filename, data)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: err.Error()}})
				return
			}
			articles = append(articles, parsed...)
			if len(articles) > maxKnowledgeImportArticles {
				c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: fmt.Sprintf("单次最多导入 %d 条知识。", maxKnowledgeImportArticles)}})
				return
			}
		}
		if err := st.CreateArticles(knowledgeBaseID, articles); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to import knowledge articles."}})
			return
		}
		c.JSON(http.StatusOK, model.KnowledgeImportResponse{Created: len(articles)})
	}
}

func parseKnowledgeImportFile(filename string, data []byte) ([]model.CreateArticleRequest, error) {
	name := filepath.Base(filename)
	extension := strings.ToLower(filepath.Ext(name))
	content := strings.TrimSpace(strings.TrimPrefix(string(data), "\ufeff"))
	switch extension {
	case ".txt", ".md":
		title := strings.TrimSpace(strings.TrimSuffix(name, extension))
		if title == "" || content == "" {
			return nil, fmt.Errorf("%s 缺少标题或内容。", name)
		}
		return []model.CreateArticleRequest{{Title: title, Content: content, Attributes: "{}"}}, nil
	case ".csv":
		return parseKnowledgeCSV(name, content)
	case ".json":
		return parseKnowledgeJSON(name, content)
	default:
		return nil, fmt.Errorf("%s 格式不受支持，请上传 CSV、JSON、Markdown 或文本文件。", name)
	}
}

func parseKnowledgeCSV(filename, content string) ([]model.CreateArticleRequest, error) {
	rows, err := csv.NewReader(strings.NewReader(content)).ReadAll()
	if err != nil || len(rows) < 2 {
		return nil, fmt.Errorf("%s 不是有效的 CSV 知识文件。", filename)
	}
	columns := map[string]int{}
	for i, column := range rows[0] {
		columns[strings.ToLower(strings.TrimSpace(strings.TrimPrefix(column, "\ufeff")))] = i
	}
	titleColumn, titleOK := columns["title"]
	contentColumn, contentOK := columns["content"]
	if !titleOK || !contentOK {
		return nil, fmt.Errorf("%s 必须包含 title 和 content 列。", filename)
	}
	categoryColumn := -1
	if index, ok := columns["category"]; ok {
		categoryColumn = index
	}
	attributesColumn := -1
	if index, ok := columns["attributes"]; ok {
		attributesColumn = index
	}
	articles := make([]model.CreateArticleRequest, 0, len(rows)-1)
	for rowNumber, row := range rows[1:] {
		if allCellsBlank(row) {
			continue
		}
		title, body := cellAt(row, titleColumn), cellAt(row, contentColumn)
		if title == "" || body == "" {
			return nil, fmt.Errorf("%s 第 %d 行缺少 title 或 content。", filename, rowNumber+2)
		}
		attributes := cellAt(row, attributesColumn)
		if attributes == "" {
			attributes = "{}"
		}
		if !json.Valid([]byte(attributes)) {
			return nil, fmt.Errorf("%s 第 %d 行的 attributes 必须是 JSON。", filename, rowNumber+2)
		}
		articles = append(articles, model.CreateArticleRequest{
			Title: title, Content: body, Category: cellAt(row, categoryColumn), Attributes: attributes,
		})
	}
	if len(articles) == 0 {
		return nil, fmt.Errorf("%s 没有可导入的知识条目。", filename)
	}
	return articles, nil
}

func parseKnowledgeJSON(filename, content string) ([]model.CreateArticleRequest, error) {
	type importedArticle struct {
		Title      string          `json:"title"`
		Content    string          `json:"content"`
		Category   string          `json:"category"`
		Attributes json.RawMessage `json:"attributes"`
	}
	var imported []importedArticle
	if err := json.Unmarshal([]byte(content), &imported); err != nil || len(imported) == 0 {
		return nil, fmt.Errorf("%s 必须是非空的知识条目 JSON 数组。", filename)
	}
	articles := make([]model.CreateArticleRequest, 0, len(imported))
	for index, item := range imported {
		title := strings.TrimSpace(item.Title)
		body := strings.TrimSpace(item.Content)
		if title == "" || body == "" {
			return nil, fmt.Errorf("%s 第 %d 条缺少 title 或 content。", filename, index+1)
		}
		attributes, err := normalizeImportAttributes(item.Attributes)
		if err != nil {
			return nil, fmt.Errorf("%s 第 %d 条的 attributes 必须是 JSON。", filename, index+1)
		}
		articles = append(articles, model.CreateArticleRequest{
			Title: title, Content: body, Category: strings.TrimSpace(item.Category), Attributes: attributes,
		})
	}
	return articles, nil
}

func normalizeImportAttributes(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "{}", nil
	}
	if raw[0] == '"' {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil || !json.Valid([]byte(value)) {
			return "", fmt.Errorf("invalid attributes")
		}
		return value, nil
	}
	if !json.Valid(raw) {
		return "", fmt.Errorf("invalid attributes")
	}
	return string(raw), nil
}

func cellAt(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[index])
}

func allCellsBlank(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
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
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxJSONRequestBytes)
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
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxJSONRequestBytes)
		var req model.UpdateKnowledgeRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid request."}})
			return
		}
		if req.Name == nil && req.Description == nil && req.Status == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "At least one field is required."}})
			return
		}
		if req.Status != nil && *req.Status != "active" && *req.Status != "inactive" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid knowledge base status."}})
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
		knowledgeBaseID := c.Param("id")
		// Load the base BEFORE deleting so the audit entry records the human-
		// readable name as well as the ID. After deletion the row is gone and
		// only the ID would remain in the logs.
		base, err := st.KnowledgeBaseByID(knowledgeBaseID, session.ActiveTenantID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Knowledge base not found."}})
			return
		}
		if err := st.DeleteKnowledgeBase(knowledgeBaseID, session.ActiveTenantID); err != nil {
			slog.Info("audit knowledge base delete",
				"subject", session.User.ID,
				"action", "knowledge_base.delete",
				"target", knowledgeBaseID,
				"tenant", session.ActiveTenantID,
				"name", base.Name,
				"result", "failure: "+err.Error(),
			)
			if errors.Is(err, pgx.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Knowledge base not found."}})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to delete knowledge base."}})
			}
			return
		}
		slog.Info("audit knowledge base delete",
			"subject", session.User.ID,
			"action", "knowledge_base.delete",
			"target", knowledgeBaseID,
			"tenant", session.ActiveTenantID,
			"name", base.Name,
			"result", "success",
		)
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
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxJSONRequestBytes)
		var req model.CreateArticleRequest
		if err := c.ShouldBindJSON(&req); err != nil || req.Title == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Title is required."}})
			return
		}
		if utf8.RuneCountInString(req.Content) > maxArticleContentRunes {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: fmt.Sprintf("正文不能超过 %d 个字符。", maxArticleContentRunes)}})
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
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxJSONRequestBytes)
		var req model.UpdateArticleRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid request."}})
			return
		}
		if req.Title == nil && req.Content == nil && req.Category == nil && req.Attributes == nil && req.Status == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "At least one field is required."}})
			return
		}
		if req.Content != nil && utf8.RuneCountInString(*req.Content) > maxArticleContentRunes {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: fmt.Sprintf("正文不能超过 %d 个字符。", maxArticleContentRunes)}})
			return
		}
		if req.Status != nil && *req.Status != "active" && *req.Status != "inactive" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid article status."}})
			return
		}
		article, err := st.UpdateArticle(c.Param("articleId"), c.Param("id"), session.ActiveTenantID, req.Title, req.Content, req.Category, req.Attributes, req.Status)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Article not found."}})
			} else {
				c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to update article."}})
			}
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
		if err := st.DeleteArticle(c.Param("articleId"), c.Param("id"), session.ActiveTenantID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Article not found."}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func handleSearchKnowledge(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxJSONRequestBytes)
		var req model.SearchRequest
		if err := c.ShouldBindJSON(&req); err != nil || req.Query == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Query is required."}})
			return
		}
		if req.Limit <= 0 {
			req.Limit = 5
		}
		if req.Limit > maxSearchLimit {
			req.Limit = maxSearchLimit
		}
		results, err := st.SearchKnowledge(session.ActiveTenantID, req.Query, req.Embedding, req.Limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Search failed."}})
			return
		}
		c.JSON(http.StatusOK, model.SearchResponse{Results: results})
	}
}
