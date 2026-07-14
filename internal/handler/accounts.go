package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"whatsapp-ai-poc/internal/middleware"
	"whatsapp-ai-poc/internal/model"
	"whatsapp-ai-poc/internal/store"
)

var qrCache = map[string]*qrSession{}
var qrCacheMu sync.Mutex

type qrSession struct {
	QrData    string
	ExpiresAt time.Time
	AccountID string
}

func init() {
	go func() {
		for {
			time.Sleep(10 * time.Second)
			qrCacheMu.Lock()
			now := time.Now()
			for k, v := range qrCache {
				if now.After(v.ExpiresAt) {
					delete(qrCache, k)
				}
			}
			qrCacheMu.Unlock()
		}
	}()
}

func isOpenClawAvailable() bool {
	_, err := exec.LookPath("openclaw")
	return err == nil
}

// getQRFromCLI runs openclaw channels login via script PTY and extracts clean QR text.
func getQRFromCLI(accountKey string) (string, error) {
	if !isOpenClawAvailable() {
		return "", fmt.Errorf("openclaw 未安装")
	}

	tmpFile, err := os.CreateTemp("", "oc_qr_*.txt")
	if err != nil {
		return "", err
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "script", "-q", tmpPath,
		"openclaw", "channels", "login", "--channel", "whatsapp", "--account", accountKey)
	cmd.Start()

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-time.After(10 * time.Second):
		cmd.Process.Kill()
		<-done
	case <-done:
	}

	data, _ := os.ReadFile(tmpPath)
	if len(data) == 0 {
		return "", fmt.Errorf("未能获取二维码")
	}
	return string(data), nil
}

// cleanQROutput strips terminal control sequences, keeping QR block characters.
func cleanQROutput(raw string) string {
	var buf strings.Builder
	esc := byte(0x1b)
	i := 0
	for i < len(raw) {
		c := raw[i]
		if c == esc && i+1 < len(raw) && raw[i+1] == '[' {
			// Skip CSI sequence: ESC[ ... letter
			end := i + 2
			for end < len(raw) && end-i < 20 {
				b := raw[end]
				if (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') {
					end++
					break
				}
				end++
			}
			i = end
			continue
		}
		if c == esc && i+1 < len(raw) && raw[i+1] == ']' {
			// Skip OSC sequence: ESC] ... BEL or ST
			end := i + 2
			for end < len(raw) && end-i < 200 {
				if raw[end] == 0x07 || (raw[end] == esc && end+1 < len(raw) && raw[end+1] == '\\') {
					end += 2
					break
				}
				end++
			}
			i = end
			continue
		}
		if c == '\r' || c == 0x04 {
			i++
			continue
		}
		buf.WriteByte(c)
		i++
	}
	return buf.String()
}

func RegisterAccounts(r *gin.RouterGroup, st *store.Store) {
	r.GET("", handleListAccounts(st))
	r.POST("", handleCreateAccount(st))
	r.PATCH("/:id", handleUpdateAccount(st))
	r.POST("/:id/qr-login", handleQrLogin(st))
	r.GET("/:id/qr-status", handleQrStatus(st))
	r.POST("/:id/disconnect", handleDisconnect(st))
}

func handleListAccounts(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		accounts, err := st.AccountsByTenant(session.ActiveTenantID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to load accounts."}})
			return
		}
		if accounts == nil {
			accounts = []model.Account{}
		}
		c.JSON(http.StatusOK, model.AccountsResponse{Accounts: accounts})
	}
}

func handleCreateAccount(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		var req model.CreateAccountRequest
		if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Account name is required."}})
			return
		}
		if req.DailyLimit <= 0 {
			req.DailyLimit = 30
		}
		if req.ReplyLimit <= 0 {
			req.ReplyLimit = 30
		}
		account, err := st.CreateAccount(session.ActiveTenantID, req.Name, req.KbID, req.DailyLimit, req.ReplyLimit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to create account."}})
			return
		}
		c.JSON(http.StatusOK, account)
	}
}

func handleUpdateAccount(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		var req model.UpdateAccountRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid request."}})
			return
		}
		account, err := st.UpdateAccount(session.ActiveTenantID, c.Param("id"), req.Name, req.KbID, req.Status, req.DailyLimit, req.ReplyLimit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to update account."}})
			return
		}
		c.JSON(http.StatusOK, account)
	}
}

func handleQrLogin(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		accountID := c.Param("id")
		acct, err := st.AccountByID(session.ActiveTenantID, accountID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Account not found."}})
			return
		}

		qrCacheMu.Lock()
		delete(qrCache, accountID)
		qrCacheMu.Unlock()

		// Use CLI to get QR code
		raw, err := getQRFromCLI(acct.AccountKey)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "OPENCLAW_ERROR", Message: "获取二维码失败: " + err.Error()}})
			return
		}

		qrData := cleanQROutput(raw)

		expiresAt := time.Now().Add(30 * time.Second)
		qrCacheMu.Lock()
		qrCache[accountID] = &qrSession{QrData: qrData, ExpiresAt: expiresAt, AccountID: accountID}
		qrCacheMu.Unlock()

		c.JSON(http.StatusOK, model.QrLoginResponse{
			QrData:    qrData,
			ExpiresAt: expiresAt.Format("2006-01-02 15:04:05"),
			AccountID: accountID,
		})
	}
}

func handleQrStatus(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		accountID := c.Param("id")
		acct, err := st.AccountByID(session.ActiveTenantID, accountID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Account not found."}})
			return
		}

		qrCacheMu.Lock()
		_, hasQR := qrCache[accountID]
		qrCacheMu.Unlock()

		resp := model.AccountStatusResponse{Status: acct.Status}
		if hasQR {
			resp.Status = "qr_pending"
		}

		if isOpenClawAvailable() {
			out, err := exec.Command("openclaw", "channels", "status", "--channel", "whatsapp", "--json").Output()
			if err == nil {
				var sr struct {
					Channels map[string]struct {
						Linked    bool `json:"linked"`
						Connected bool `json:"connected"`
					} `json:"channels"`
				}
				if json.Unmarshal(out, &sr) == nil {
					if ch, ok := sr.Channels["whatsapp"]; ok && ch.Linked {
						resp.Status = "connected"
						resp.ConnectedAt = time.Now().Format("2006-01-02 15:04:05")
						st.UpdateAccount(session.ActiveTenantID, accountID, "", "", "connected", nil, nil)
						qrCacheMu.Lock()
						delete(qrCache, accountID)
						qrCacheMu.Unlock()
					}
				}
			}
		}

		c.JSON(http.StatusOK, resp)
	}
}

func handleDisconnect(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		accountID := c.Param("id")
		acct, err := st.AccountByID(session.ActiveTenantID, accountID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Account not found."}})
			return
		}

		exec.Command("openclaw", "channels", "logout", "--channel", "whatsapp", "--account", acct.AccountKey).Run()

		qrCacheMu.Lock()
		delete(qrCache, accountID)
		qrCacheMu.Unlock()

		st.UpdateAccount(session.ActiveTenantID, accountID, "", "", "pending", nil, nil)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
