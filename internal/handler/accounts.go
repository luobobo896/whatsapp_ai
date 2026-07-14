package handler

import (
	"bufio"
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
var qrBridgePathOnce sync.Once
var qrBridgePath string
var qrBridgePathErr error

const qrBridgeTimeout = 25 * time.Second

//go:embed whatsapp_qr_bridge.mjs
var whatsappQrBridgeScript []byte

type qrSession struct {
	QrData    string
	ExpiresAt time.Time
	AccountID string
	Cmd       *exec.Cmd
	Status    string
	Err       error
	Events    <-chan qrBridgeEvent
	Stderr    *bytes.Buffer
}

type qrBridgeEvent struct {
	Type      string `json:"type"`
	QrDataURL string `json:"qrDataUrl,omitempty"`
	Connected bool   `json:"connected,omitempty"`
	Message   string `json:"message,omitempty"`
	Error     string `json:"error,omitempty"`
}

func init() {
	go func() {
		for {
			time.Sleep(10 * time.Second)
			qrCacheMu.Lock()
			now := time.Now()
			for k, v := range qrCache {
				if v.Status == "starting" {
					continue
				}
				if now.After(v.ExpiresAt) {
					stopQrSession(v)
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

func parseQrBridgeEvent(line []byte) (qrBridgeEvent, error) {
	var event qrBridgeEvent
	if err := json.Unmarshal(bytes.TrimSpace(line), &event); err != nil {
		return qrBridgeEvent{}, err
	}
	return event, nil
}

func resolveWhatsAppLoginModule() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("OPENCLAW_WHATSAPP_LOGIN_MODULE")); configured != "" {
		if _, err := os.Stat(configured); err != nil {
			return "", fmt.Errorf("OpenClaw WhatsApp 登录模块不存在: %w", err)
		}
		return configured, nil
	}

	output, err := exec.Command("openclaw", "plugins", "list", "--json").Output()
	if err == nil {
		var payload struct {
			Plugins []struct {
				ID      string `json:"id"`
				RootDir string `json:"rootDir"`
				Source  string `json:"source"`
			} `json:"plugins"`
		}
		if json.Unmarshal(output, &payload) == nil {
			for _, plugin := range payload.Plugins {
				if plugin.ID != "whatsapp" {
					continue
				}
				root := plugin.RootDir
				if root == "" && plugin.Source != "" {
					root = filepath.Dir(filepath.Dir(plugin.Source))
				}
				if root != "" {
					module := filepath.Join(root, "dist", "login-qr-runtime.js")
					if _, statErr := os.Stat(module); statErr == nil {
						return module, nil
					}
				}
			}
		}
	}

	home, homeErr := os.UserHomeDir()
	if homeErr == nil {
		fallback := filepath.Join(home, ".openclaw", "extensions", "whatsapp", "dist", "login-qr-runtime.js")
		if _, statErr := os.Stat(fallback); statErr == nil {
			return fallback, nil
		}
	}
	if err != nil {
		return "", fmt.Errorf("无法定位 OpenClaw WhatsApp 登录模块: %w", err)
	}
	return "", fmt.Errorf("无法定位 OpenClaw WhatsApp 登录模块")
}

func qrBridgeScriptPath() (string, error) {
	qrBridgePathOnce.Do(func() {
		file, err := os.CreateTemp("", "whatsapp-ai-qr-bridge-*.mjs")
		if err != nil {
			qrBridgePathErr = err
			return
		}
		qrBridgePath = file.Name()
		if _, err = file.Write(whatsappQrBridgeScript); err != nil {
			qrBridgePathErr = err
			file.Close()
			return
		}
		if err = file.Close(); err != nil {
			qrBridgePathErr = err
		}
	})
	return qrBridgePath, qrBridgePathErr
}

func startQrSession(accountID, accountKey string) (*qrSession, error) {
	if !isOpenClawAvailable() {
		return nil, fmt.Errorf("openclaw 未安装")
	}
	modulePath, err := resolveWhatsAppLoginModule()
	if err != nil {
		return nil, err
	}
	bridgePath, err := qrBridgeScriptPath()
	if err != nil {
		return nil, fmt.Errorf("创建二维码桥接脚本失败: %w", err)
	}

	cmd := exec.Command("node", bridgePath, modulePath, accountKey)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	events := make(chan qrBridgeEvent, 8)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			event, err := parseQrBridgeEvent(scanner.Bytes())
			if err == nil {
				events <- event
			}
		}
		close(events)
	}()

	var first qrBridgeEvent
	var ok bool
	select {
	case first, ok = <-events:
		if !ok {
			cmd.Wait()
			if stderr.Len() > 0 {
				return nil, fmt.Errorf("OpenClaw 二维码进程未返回结果: %s", strings.TrimSpace(stderr.String()))
			}
			return nil, fmt.Errorf("OpenClaw 二维码进程未返回结果")
		}
	case <-time.After(qrBridgeTimeout):
		stopQrProcess(cmd)
		_ = cmd.Wait()
		return nil, fmt.Errorf("获取二维码超时")
	}
	if first.Type == "error" || !strings.HasPrefix(first.QrDataURL, "data:image/png;base64,") {
		stopQrProcess(cmd)
		_ = cmd.Wait()
		if first.Error != "" {
			return nil, fmt.Errorf("获取二维码失败: %s", first.Error)
		}
		return nil, fmt.Errorf("OpenClaw 未返回 PNG 二维码")
	}

	session := &qrSession{
		QrData:    first.QrDataURL,
		ExpiresAt: time.Now().Add(30 * time.Second),
		AccountID: accountID,
		Cmd:       cmd,
		Status:    "qr_pending",
		Events:    events,
		Stderr:    &stderr,
	}
	return session, nil
}

func monitorQrSession(session *qrSession, events <-chan qrBridgeEvent, stderr *bytes.Buffer) {
	for event := range events {
		qrCacheMu.Lock()
		current, ok := qrCache[session.AccountID]
		if ok && current == session {
			switch {
			case event.Type == "qr" && strings.HasPrefix(event.QrDataURL, "data:image/png;base64,"):
				session.QrData = event.QrDataURL
				session.ExpiresAt = time.Now().Add(30 * time.Second)
			case event.Type == "status" && event.Connected:
				session.Status = "connected"
			case event.Type == "error":
				session.Err = fmt.Errorf("%s", event.Error)
			}
		}
		qrCacheMu.Unlock()
	}
	if err := session.Cmd.Wait(); err != nil {
		qrCacheMu.Lock()
		if current, ok := qrCache[session.AccountID]; ok && current == session && session.Status != "connected" && session.Err == nil {
			message := strings.TrimSpace(stderr.String())
			if message == "" {
				message = err.Error()
			}
			session.Err = fmt.Errorf("%s", message)
		}
		qrCacheMu.Unlock()
	}
}

func stopQrProcess(cmd *exec.Cmd) {
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func stopQrSession(session *qrSession) {
	if session != nil && session.Status != "connected" {
		stopQrProcess(session.Cmd)
	}
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
		previous := qrCache[accountID]
		if previous != nil && previous.Status == "starting" {
			qrCacheMu.Unlock()
			c.JSON(http.StatusConflict, gin.H{"error": model.ErrorDetail{Code: "QR_IN_PROGRESS", Message: "QR login is already being started for this account."}})
			return
		}
		qrCache[accountID] = &qrSession{AccountID: accountID, Status: "starting"}
		qrCacheMu.Unlock()
		stopQrSession(previous)

		qr, err := startQrSession(accountID, acct.AccountKey)
		qrCacheMu.Lock()
		if err != nil {
			delete(qrCache, accountID)
			qrCacheMu.Unlock()
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "OPENCLAW_ERROR", Message: err.Error()}})
			return
		}
		qrCache[accountID] = qr
		qrCacheMu.Unlock()
		go monitorQrSession(qr, qr.Events, qr.Stderr)

		c.JSON(http.StatusOK, model.QrLoginResponse{
			QrData:    qr.QrData,
			ExpiresAt: qr.ExpiresAt.Format(time.RFC3339),
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

		resp := model.AccountStatusResponse{Status: acct.Status}
		qrCacheMu.Lock()
		qr := qrCache[accountID]
		if qr != nil {
			switch {
			case qr.Status == "starting":
				// still waiting for startQrSession to complete
			case qr.Status == "connected":
				resp.Status = "connected"
				resp.ConnectedAt = time.Now().Format("2006-01-02 15:04:05")
				delete(qrCache, accountID)
			case time.Now().Before(qr.ExpiresAt):
				resp.Status = "qr_pending"
				resp.QrData = qr.QrData
				resp.ExpiresAt = qr.ExpiresAt.Format(time.RFC3339)
			default:
				stopQrSession(qr)
				delete(qrCache, accountID)
			}
		}
		qrCacheMu.Unlock()

		if resp.Status == "connected" {
			st.UpdateAccount(session.ActiveTenantID, accountID, "", "", "connected", nil, nil)
			c.JSON(http.StatusOK, resp)
			return
		}

		if isOpenClawAvailable() {
			out, err := exec.Command("openclaw", "channels", "status", "--channel", "whatsapp", "--account", acct.AccountKey, "--json").Output()
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
						stopQrSession(qrCache[accountID])
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
		stopQrSession(qrCache[accountID])
		delete(qrCache, accountID)
		qrCacheMu.Unlock()

		st.UpdateAccount(session.ActiveTenantID, accountID, "", "", "pending", nil, nil)
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}
