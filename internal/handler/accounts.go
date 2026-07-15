package handler

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"whatsapp-ai-poc/internal/middleware"
	"whatsapp-ai-poc/internal/model"
	"whatsapp-ai-poc/internal/store"
)

var qrCache = map[string]*qrSession{}
var qrCacheMu sync.Mutex
var qrBridgePathOnce sync.Once
var qrBridgePath string
var qrBridgePathErr error
var openClawConfigMu sync.Mutex
var openClawGatewayMu sync.Mutex
var openClawRAGAssetsMu sync.Mutex

var (
	channelStatusCache   map[string]channelConnectionStatus
	channelStatusCacheAt time.Time
	channelStatusCacheMu sync.Mutex
)

const (
	qrBridgeTimeout        = 25 * time.Second
	qrCodeTTL              = 45 * time.Second
	qrConnectionTimeout    = time.Minute
	qrSessionCleanupWait   = time.Minute
	openClawRestartWait    = 30 * time.Second
	openClawCommandTimeout = 10 * time.Second
	openClawRAGInstallWait = 2 * time.Minute
	openClawPollInterval   = time.Second
	channelStatusCacheTTL  = 5 * time.Second
)

//go:embed whatsapp_qr_bridge.mjs
var whatsappQrBridgeScript []byte

type qrSession struct {
	QrData             string
	ExpiresAt          time.Time
	ConnectionDeadline time.Time
	CleanupAt          time.Time
	AccountID          string
	AccountKey         string
	Cmd                *exec.Cmd
	Status             string
	Err                error
	Events             <-chan qrBridgeEvent
	Stderr             *bytes.Buffer
}

type qrBridgeEvent struct {
	Type      string `json:"type"`
	QrDataURL string `json:"qrDataUrl,omitempty"`
	Connected bool   `json:"connected,omitempty"`
	Message   string `json:"message,omitempty"`
	Error     string `json:"error,omitempty"`
}

type channelConnectionStatus struct {
	Known     bool
	Linked    bool
	Running   bool
	Connected bool
}

type openClawChannelStatusPayload struct {
	Channels map[string]struct {
		Linked    bool `json:"linked"`
		Running   bool `json:"running"`
		Connected bool `json:"connected"`
	} `json:"channels"`
	ChannelAccounts map[string][]struct {
		AccountID string `json:"accountId"`
		Linked    bool   `json:"linked"`
		Running   bool   `json:"running"`
		Connected bool   `json:"connected"`
	} `json:"channelAccounts"`
}

func updateQrSessionStatus(session *qrSession, channel channelConnectionStatus, now time.Time) string {
	if session == nil {
		return ""
	}
	if session.Err != nil {
		session.Status = "expired"
		session.CleanupAt = now
		return session.Status
	}
	if session.Status == "connected" || channel.Known && channel.Running && channel.Connected {
		session.Status = "connected"
		if session.CleanupAt.Before(now) {
			session.CleanupAt = now.Add(qrSessionCleanupWait)
		}
		return session.Status
	}
	if session.Status == "connecting" {
		if session.ConnectionDeadline.IsZero() {
			session.ConnectionDeadline = now.Add(qrConnectionTimeout)
			session.CleanupAt = session.ConnectionDeadline
		}
		if !now.Before(session.ConnectionDeadline) {
			session.Status = "expired"
		}
		return session.Status
	}
	if channel.Known && channel.Linked {
		session.Status = "connecting"
		session.ConnectionDeadline = now.Add(qrConnectionTimeout)
		session.CleanupAt = session.ConnectionDeadline
		return session.Status
	}
	if session.Status == "starting" {
		return session.Status
	}
	if now.Before(session.ExpiresAt) {
		session.Status = "qr_pending"
		return session.Status
	}
	session.Status = "expired"
	return session.Status
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
				if !v.CleanupAt.IsZero() && !now.Before(v.CleanupAt) {
					stopQrSession(v)
					delete(qrCache, k)
				}
			}
			qrCacheMu.Unlock()
		}
	}()
}

func isOpenClawAvailable() bool {
	command, _ := openClawCommandSpec()
	_, err := exec.LookPath(command)
	return err == nil
}

func openClawDockerContainer() string {
	return strings.TrimSpace(os.Getenv("OPENCLAW_DOCKER_CONTAINER"))
}

func openClawCommandSpec(args ...string) (string, []string) {
	if container := openClawDockerContainer(); container != "" {
		return "docker", append([]string{"exec", container, "openclaw"}, args...)
	}
	return "openclaw", args
}

func openClawBridgeCommandSpec(modulePath, accountKey string) (string, []string) {
	if container := openClawDockerContainer(); container != "" {
		return "docker", []string{"exec", "-i", container, "node", "--input-type=module", "-", modulePath, accountKey}
	}
	return "node", []string{modulePath, accountKey}
}

func openClawCommand(ctx context.Context, args ...string) *exec.Cmd {
	command, commandArgs := openClawCommandSpec(args...)
	return exec.CommandContext(ctx, command, commandArgs...)
}

func openClawGatewayRestartCommandSpec() (string, []string) {
	if container := openClawDockerContainer(); container != "" {
		return "docker", []string{"restart", container}
	}
	return "openclaw", []string{"gateway", "restart"}
}

func openClawConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".openclaw", "openclaw.json"), nil
}

// ensureOpenClawAccount adds the account to OpenClaw's config so the gateway
// monitors its auth directory. Safe to call multiple times (idempotent).
func ensureOpenClawAccount(accountKey string) error {
	cfgPath, err := openClawConfigPath()
	if err != nil {
		return err
	}
	return ensureOpenClawAccountAtPath(cfgPath, accountKey)
}

func ensureOpenClawAccountAtPath(cfgPath, accountKey string) error {
	openClawConfigMu.Lock()
	defer openClawConfigMu.Unlock()

	data, err := os.ReadFile(cfgPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	var cfg map[string]any
	if len(data) == 0 {
		cfg = map[string]any{}
	} else if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}

	if !ensureOpenClawAccountConfig(cfg, accountKey) {
		return nil
	}

	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		return err
	}
	return os.WriteFile(cfgPath, updated, 0o600)
}

func ensureOpenClawAccountConfig(cfg map[string]any, accountKey string) bool {
	changed := false
	channels, _ := cfg["channels"].(map[string]any)
	if channels == nil {
		channels = map[string]any{}
		cfg["channels"] = channels
		changed = true
	}
	wa, _ := channels["whatsapp"].(map[string]any)
	if wa == nil {
		wa = map[string]any{}
		channels["whatsapp"] = wa
		changed = true
	}
	accounts, _ := wa["accounts"].(map[string]any)
	if accounts == nil {
		accounts = map[string]any{}
		wa["accounts"] = accounts
		changed = true
	}
	account, exists := accounts[accountKey].(map[string]any)
	if !exists || account == nil {
		accounts[accountKey] = map[string]any{"enabled": true}
		changed = true
	} else if account["enabled"] != true {
		account["enabled"] = true
		changed = true
	}
	return changed
}

type openClawRAGOptions struct {
	APIURL    string
	APIToken  string
	MCPPath   string
	Workspace string
}

func openClawRAGMCPName(accountKey string) string {
	return "whatsapp-rag-" + accountKey
}

func openClawRAGAgentID(accountKey string) string {
	return "whatsapp-rag-" + accountKey
}

func ensureOpenClawRAGConfig(cfg map[string]any, accountID, accountKey string, options openClawRAGOptions) error {
	if accountID == "" || accountKey == "" {
		return errors.New("OpenClaw RAG 账号 ID 和账号键不能为空")
	}
	if options.APIURL == "" || options.APIToken == "" || options.MCPPath == "" || options.Workspace == "" {
		return errors.New("OpenClaw RAG 配置不完整")
	}

	mcp, _ := cfg["mcp"].(map[string]any)
	if mcp == nil {
		mcp = map[string]any{}
		cfg["mcp"] = mcp
	}
	servers, _ := mcp["servers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
		mcp["servers"] = servers
	}
	delete(servers, "whatsapp-rag")
	mcpName := openClawRAGMCPName(accountKey)
	servers[mcpName] = map[string]any{
		"command": "node",
		"args":    []string{options.MCPPath},
		"env": map[string]any{
			"WHATSAPP_AI_API_URL":    options.APIURL,
			"INTERNAL_API_TOKEN":     options.APIToken,
			"WHATSAPP_AI_ACCOUNT_ID": accountID,
		},
		"toolFilter": map[string]any{"include": []string{"search_knowledge", "save_reply"}},
	}

	agents, _ := cfg["agents"].(map[string]any)
	if agents == nil {
		agents = map[string]any{}
		cfg["agents"] = agents
	}
	agentList, _ := agents["list"].([]any)
	agentID := openClawRAGAgentID(accountKey)
	agent := map[string]any{
		"id":          agentID,
		"workspace":   filepath.Join(options.Workspace, agentID),
		"description": "WhatsApp knowledge-base customer service agent",
		"tools": map[string]any{
			"profile": "minimal",
			"allow":   []string{mcpName + "__*"},
		},
	}
	foundAgent := false
	for i, raw := range agentList {
		existing, ok := raw.(map[string]any)
		if ok && existing["id"] == agentID {
			agentList[i] = agent
			foundAgent = true
			break
		}
	}
	if !foundAgent {
		agentList = append(agentList, agent)
	}
	agents["list"] = agentList

	bindings, _ := cfg["bindings"].([]any)
	binding := map[string]any{
		"agentId": agentID,
		"comment": "Managed by WhatsApp AI knowledge-base routing",
		"match": map[string]any{
			"channel":   "whatsapp",
			"accountId": accountKey,
		},
		"session": map[string]any{"dmScope": "per-account-channel-peer"},
	}
	foundBinding := false
	for i, raw := range bindings {
		existing, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		match, _ := existing["match"].(map[string]any)
		if match["channel"] == "whatsapp" && match["accountId"] == accountKey {
			if existing["agentId"] != agentID || existing["comment"] != binding["comment"] {
				return fmt.Errorf("WhatsApp 账号 %s 已绑定到其他 OpenClaw Agent", accountKey)
			}
			bindings[i] = binding
			foundBinding = true
			break
		}
	}
	if !foundBinding {
		bindings = append(bindings, binding)
	}
	cfg["bindings"] = bindings
	return nil
}

func removeOpenClawRAGConfig(cfg map[string]any, accountKey string) bool {
	changed := false
	mcpName := openClawRAGMCPName(accountKey)
	if mcp, ok := cfg["mcp"].(map[string]any); ok {
		if servers, ok := mcp["servers"].(map[string]any); ok {
			if _, exists := servers[mcpName]; exists {
				delete(servers, mcpName)
				changed = true
			}
		}
	}
	agentID := openClawRAGAgentID(accountKey)
	if agents, ok := cfg["agents"].(map[string]any); ok {
		if list, ok := agents["list"].([]any); ok {
			filtered := list[:0]
			for _, raw := range list {
				if agent, ok := raw.(map[string]any); ok && agent["id"] == agentID {
					changed = true
					continue
				}
				filtered = append(filtered, raw)
			}
			agents["list"] = filtered
		}
	}
	if bindings, ok := cfg["bindings"].([]any); ok {
		filtered := bindings[:0]
		for _, raw := range bindings {
			binding, ok := raw.(map[string]any)
			match, _ := binding["match"].(map[string]any)
			if ok && binding["agentId"] == agentID && match["channel"] == "whatsapp" && match["accountId"] == accountKey {
				changed = true
				continue
			}
			filtered = append(filtered, raw)
		}
		cfg["bindings"] = filtered
	}
	if channels, ok := cfg["channels"].(map[string]any); ok {
		if whatsapp, ok := channels["whatsapp"].(map[string]any); ok {
			if accounts, ok := whatsapp["accounts"].(map[string]any); ok {
				if account, ok := accounts[accountKey].(map[string]any); ok && account["enabled"] != false {
					account["enabled"] = false
					changed = true
				}
			}
		}
	}
	return changed
}

func openClawRAGSourceDir() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("WHATSAPP_AI_RAG_MCP_SOURCE_DIR")); configured != "" {
		return configured, nil
	}
	workingDir, err := os.Getwd()
	if err == nil {
		for _, candidate := range []string{
			filepath.Join(workingDir, "cmd", "rag-mcp-server"),
			filepath.Join(workingDir, "source", "cmd", "rag-mcp-server"),
		} {
			if _, statErr := os.Stat(filepath.Join(candidate, "index.mjs")); statErr == nil {
				return candidate, nil
			}
		}
	}
	if executable, executableErr := os.Executable(); executableErr == nil {
		candidate := filepath.Join(filepath.Dir(executable), "source", "cmd", "rag-mcp-server")
		if _, statErr := os.Stat(filepath.Join(candidate, "index.mjs")); statErr == nil {
			return candidate, nil
		}
	}
	return "", errors.New("未找到 RAG MCP 运行文件，请设置 WHATSAPP_AI_RAG_MCP_SOURCE_DIR")
}

func openClawRAGRuntimeDir(hostDir string) string {
	if configured := strings.TrimSpace(os.Getenv("OPENCLAW_RAG_MCP_RUNTIME_DIR")); configured != "" {
		return configured
	}
	if openClawDockerContainer() != "" {
		return "/home/node/.openclaw/whatsapp-rag-mcp"
	}
	return hostDir
}

func openClawRAGWorkspaceDirs(cfgPath string) (string, string) {
	hostDir := filepath.Join(filepath.Dir(cfgPath), "whatsapp-rag-workspaces")
	if openClawDockerContainer() != "" {
		return hostDir, "/home/node/.openclaw/whatsapp-rag-workspaces"
	}
	return hostDir, hostDir
}

func ensureOpenClawDockerOwnership(path string) error {
	if openClawDockerContainer() == "" {
		return nil
	}
	return os.Chown(path, 1000, 1000)
}

func copyRAGMCPFile(source, destination string) (bool, error) {
	data, err := os.ReadFile(source)
	if err != nil {
		return false, err
	}
	if existing, readErr := os.ReadFile(destination); readErr == nil && bytes.Equal(existing, data) {
		return false, nil
	}
	if err := os.WriteFile(destination, data, 0o644); err != nil {
		return false, err
	}
	if err := ensureOpenClawDockerOwnership(destination); err != nil {
		return false, err
	}
	return true, nil
}

func ensureOpenClawRAGAssets(cfgPath string) (string, error) {
	openClawRAGAssetsMu.Lock()
	defer openClawRAGAssetsMu.Unlock()

	sourceDir, err := openClawRAGSourceDir()
	if err != nil {
		return "", err
	}
	hostDir := filepath.Join(filepath.Dir(cfgPath), "whatsapp-rag-mcp")
	if err := os.MkdirAll(hostDir, 0o700); err != nil {
		return "", err
	}
	if err := ensureOpenClawDockerOwnership(hostDir); err != nil {
		return "", err
	}
	changed := false
	for _, name := range []string{"index.mjs", "package.json", "package-lock.json"} {
		copied, err := copyRAGMCPFile(filepath.Join(sourceDir, name), filepath.Join(hostDir, name))
		if err != nil {
			return "", fmt.Errorf("准备 RAG MCP 文件 %s: %w", name, err)
		}
		changed = changed || copied
	}
	if _, err := os.Stat(filepath.Join(hostDir, "node_modules")); errors.Is(err, os.ErrNotExist) {
		changed = true
	}
	if changed {
		ctx, cancel := context.WithTimeout(context.Background(), openClawRAGInstallWait)
		defer cancel()
		var command *exec.Cmd
		runtimeDir := openClawRAGRuntimeDir(hostDir)
		if container := openClawDockerContainer(); container != "" {
			command = exec.CommandContext(ctx, "docker", "exec", container, "npm", "ci", "--omit=dev", "--prefix", runtimeDir)
		} else {
			command = exec.CommandContext(ctx, "npm", "ci", "--omit=dev", "--prefix", hostDir)
		}
		if output, err := command.CombinedOutput(); err != nil {
			return "", fmt.Errorf("安装 RAG MCP 依赖失败: %s", strings.TrimSpace(string(output)))
		}
	}
	return openClawRAGRuntimeDir(hostDir), nil
}

const (
	openClawRAGPolicyStart = "<!-- whatsapp-ai-rag-policy:start -->"
	openClawRAGPolicyEnd   = "<!-- whatsapp-ai-rag-policy:end -->"
)

func openClawRAGPolicy() string {
	return openClawRAGPolicyStart + "\n" +
		"# WhatsApp Knowledge-Base Reply Policy\n\n" +
		"For every customer message, call search_knowledge before replying. Use only returned knowledge-base content and system instructions. Never answer from general knowledge. If the response begins [DIRECT_REPLY], send its supplied text verbatim. After sending, call save_reply with the exact content.\n" +
		openClawRAGPolicyEnd + "\n"
}

func writeOpenClawRAGWorkspace(workspaceDir, accountKey string) error {
	if err := os.MkdirAll(workspaceDir, 0o700); err != nil {
		return err
	}
	if err := ensureOpenClawDockerOwnership(workspaceDir); err != nil {
		return err
	}
	agentDir := filepath.Join(workspaceDir, openClawRAGAgentID(accountKey))
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		return err
	}
	if err := ensureOpenClawDockerOwnership(agentDir); err != nil {
		return err
	}
	policyPath := filepath.Join(agentDir, "AGENTS.md")
	existing, err := os.ReadFile(policyPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	policy := openClawRAGPolicy()
	updated := string(existing)
	start := strings.Index(updated, openClawRAGPolicyStart)
	end := strings.Index(updated, openClawRAGPolicyEnd)
	if start >= 0 && end >= start {
		updated = updated[:start] + policy + updated[end+len(openClawRAGPolicyEnd):]
	} else if !strings.Contains(updated, openClawRAGPolicyStart) {
		if updated != "" && !strings.HasSuffix(updated, "\n") {
			updated += "\n"
		}
		updated += "\n" + policy
	}
	if string(existing) == updated {
		return nil
	}
	if err := os.WriteFile(policyPath, []byte(updated), 0o600); err != nil {
		return err
	}
	return ensureOpenClawDockerOwnership(policyPath)
}

func sameOpenClawConfig(data, updated []byte) bool {
	return bytes.Equal(bytes.TrimSpace(data), updated)
}

func ensureOpenClawRAGAccount(accountID, accountKey string) (bool, error) {
	apiToken := strings.TrimSpace(os.Getenv("INTERNAL_API_TOKEN"))
	if apiToken == "" {
		return false, errors.New("INTERNAL_API_TOKEN 未配置")
	}
	apiURL := strings.TrimSpace(os.Getenv("WHATSAPP_AI_RAG_API_URL"))
	if apiURL == "" {
		if openClawDockerContainer() != "" {
			return false, errors.New("Docker OpenClaw 需要配置 WHATSAPP_AI_RAG_API_URL")
		}
		apiURL = "http://127.0.0.1:8790"
	}
	cfgPath, err := openClawConfigPath()
	if err != nil {
		return false, err
	}
	mcpDir, err := ensureOpenClawRAGAssets(cfgPath)
	if err != nil {
		return false, err
	}
	hostWorkspace, runtimeWorkspace := openClawRAGWorkspaceDirs(cfgPath)
	if err := writeOpenClawRAGWorkspace(hostWorkspace, accountKey); err != nil {
		return false, fmt.Errorf("准备 OpenClaw RAG 工作区: %w", err)
	}

	openClawConfigMu.Lock()
	defer openClawConfigMu.Unlock()
	data, err := os.ReadFile(cfgPath)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	var cfg map[string]any
	if len(data) == 0 {
		cfg = map[string]any{}
	} else if err := json.Unmarshal(data, &cfg); err != nil {
		return false, err
	}
	if err := ensureOpenClawRAGConfig(cfg, accountID, accountKey, openClawRAGOptions{
		APIURL: apiURL, APIToken: apiToken, MCPPath: filepath.Join(mcpDir, "index.mjs"), Workspace: runtimeWorkspace,
	}); err != nil {
		return false, err
	}
	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return false, err
	}
	if sameOpenClawConfig(data, updated) {
		return false, nil
	}
	if err := os.WriteFile(cfgPath, updated, 0o600); err != nil {
		return false, err
	}
	return true, nil
}

func disableOpenClawRAGAccount(accountKey string) (bool, error) {
	cfgPath, err := openClawConfigPath()
	if err != nil {
		return false, err
	}
	openClawConfigMu.Lock()
	defer openClawConfigMu.Unlock()
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return false, err
	}
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return false, err
	}
	if !removeOpenClawRAGConfig(cfg, accountKey) {
		return false, nil
	}
	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(cfgPath, updated, 0o600); err != nil {
		return false, err
	}
	return true, nil
}

// SyncOpenClawRAGAccounts configures every connected WhatsApp account after a
// service restart so existing accounts receive the same per-account RAG route
// as newly scanned accounts.
func SyncOpenClawRAGAccounts(st *store.Store) error {
	if !isOpenClawAvailable() {
		return nil
	}
	accounts, err := st.AllAccounts()
	if err != nil {
		return err
	}
	changed := false
	var syncErrors []error
	for _, account := range accounts {
		if account.Status == "disabled" {
			continue
		}
		accountChanged, err := ensureOpenClawRAGAccount(account.ID, account.AccountKey)
		if err != nil {
			syncErrors = append(syncErrors, fmt.Errorf("配置客服 %s 的知识库路由: %w", account.ID, err))
			continue
		}
		changed = changed || accountChanged
	}
	if changed {
		if err := restartOpenClawGatewaySafely(); err != nil {
			syncErrors = append(syncErrors, err)
		}
	}
	return errors.Join(syncErrors...)
}

func parseQrBridgeEvent(line []byte) (qrBridgeEvent, error) {
	var event qrBridgeEvent
	if err := json.Unmarshal(bytes.TrimSpace(line), &event); err != nil {
		return qrBridgeEvent{}, err
	}
	return event, nil
}

func openClawOutput(timeout time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return openClawCommand(ctx, args...).Output()
}

func readWhatsAppChannelStatus(accountKey string) channelConnectionStatus {
	out, err := openClawOutput(openClawCommandTimeout, "channels", "status", "--channel", "whatsapp", "--json")
	if err != nil {
		return channelConnectionStatus{}
	}
	return parseWhatsAppChannelStatus(out, accountKey)
}

func readAllWhatsAppChannelStatuses() map[string]channelConnectionStatus {
	channelStatusCacheMu.Lock()
	if time.Since(channelStatusCacheAt) < channelStatusCacheTTL && channelStatusCache != nil {
		cached := channelStatusCache
		channelStatusCacheMu.Unlock()
		return cached
	}
	channelStatusCacheMu.Unlock()

	out, err := openClawOutput(openClawCommandTimeout, "channels", "status", "--channel", "whatsapp", "--json")
	if err != nil {
		return nil
	}
	statuses := parseAllWhatsAppChannelStatuses(out)

	channelStatusCacheMu.Lock()
	channelStatusCache = statuses
	channelStatusCacheAt = time.Now()
	channelStatusCacheMu.Unlock()

	return statuses
}

func parseOpenClawChannelStatusPayload(data []byte) (openClawChannelStatusPayload, error) {
	var payload openClawChannelStatusPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return openClawChannelStatusPayload{}, err
	}
	return payload, nil
}

func parseAllWhatsAppChannelStatuses(data []byte) map[string]channelConnectionStatus {
	payload, err := parseOpenClawChannelStatusPayload(data)
	if err != nil {
		return nil
	}
	statuses := make(map[string]channelConnectionStatus, len(payload.ChannelAccounts["whatsapp"]))
	for _, account := range payload.ChannelAccounts["whatsapp"] {
		statuses[account.AccountID] = channelConnectionStatus{
			Known:     true,
			Linked:    account.Linked,
			Running:   account.Running,
			Connected: account.Connected,
		}
	}
	return statuses
}

func parseWhatsAppChannelStatus(data []byte, accountKey string) channelConnectionStatus {
	payload, err := parseOpenClawChannelStatusPayload(data)
	if err != nil {
		return channelConnectionStatus{}
	}
	for _, account := range payload.ChannelAccounts["whatsapp"] {
		if account.AccountID == accountKey {
			return channelConnectionStatus{
				Known:     true,
				Linked:    account.Linked,
				Running:   account.Running,
				Connected: account.Connected,
			}
		}
	}
	whatsapp, ok := payload.Channels["whatsapp"]
	if !ok {
		return channelConnectionStatus{}
	}
	return channelConnectionStatus{
		Known:     true,
		Linked:    whatsapp.Linked,
		Running:   whatsapp.Running,
		Connected: whatsapp.Connected,
	}
}

func restartOpenClawGateway() error {
	ctx, cancel := context.WithTimeout(context.Background(), openClawRestartWait)
	defer cancel()
	command, args := openClawGatewayRestartCommandSpec()
	output, err := exec.CommandContext(ctx, command, args...).CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("重启 OpenClaw gateway 失败: %s", message)
	}
	return nil
}

func restartOpenClawGatewaySafely() error {
	openClawGatewayMu.Lock()
	defer openClawGatewayMu.Unlock()
	return restartOpenClawGateway()
}

func restartAndWaitForOpenClawAccount(
	accountKey string,
	restart func() error,
	readStatus func(string) channelConnectionStatus,
	wait func(),
	attempts int,
) error {
	if err := restart(); err != nil {
		return err
	}
	for attempt := 0; attempt < attempts; attempt++ {
		status := readStatus(accountKey)
		if status.Known && status.Running && status.Connected {
			return nil
		}
		if attempt+1 < attempts {
			wait()
		}
	}
	return fmt.Errorf("OpenClaw gateway 重启后账号 %s 未连接", accountKey)
}

func activateOpenClawAccount(accountID, accountKey string) error {
	// OpenClaw owns one gateway process. Serializing activation prevents two
	// completed QR sessions from restarting that shared process concurrently.
	openClawGatewayMu.Lock()
	defer openClawGatewayMu.Unlock()
	if err := ensureOpenClawAccount(accountKey); err != nil {
		return err
	}
	if _, err := ensureOpenClawRAGAccount(accountID, accountKey); err != nil {
		return err
	}
	attempts := int(openClawRestartWait / openClawPollInterval)
	return restartAndWaitForOpenClawAccount(
		accountKey,
		restartOpenClawGateway,
		readWhatsAppChannelStatus,
		func() { time.Sleep(openClawPollInterval) },
		attempts,
	)
}

func applyLiveAccountStatuses(accounts []model.Account, statuses map[string]channelConnectionStatus) []model.Account {
	changed := make([]model.Account, 0)
	for i := range accounts {
		if accounts[i].Status == "disabled" {
			continue
		}
		live, ok := statuses[accounts[i].AccountKey]
		if !ok || !live.Known {
			continue
		}
		status := "pending"
		if live.Running && live.Connected {
			status = "connected"
		}
		if accounts[i].Status != status {
			accounts[i].Status = status
			changed = append(changed, accounts[i])
		}
	}
	return changed
}

func accountsForLiveStatusSync(accounts []model.Account) []model.Account {
	return append([]model.Account(nil), accounts...)
}

func resolveWhatsAppLoginModule() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("OPENCLAW_WHATSAPP_LOGIN_MODULE")); configured != "" {
		if openClawDockerContainer() != "" {
			return configured, nil
		}
		if _, err := os.Stat(configured); err != nil {
			return "", fmt.Errorf("OpenClaw WhatsApp 登录模块不存在: %w", err)
		}
		return configured, nil
	}

	output, err := openClawOutput(openClawCommandTimeout, "plugins", "list", "--json")
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
					if openClawDockerContainer() != "" {
						return module, nil
					}
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
	command, commandArgs := openClawBridgeCommandSpec(modulePath, accountKey)
	cmd := exec.Command(command, commandArgs...)
	if openClawDockerContainer() != "" {
		cmd.Stdin = bytes.NewReader(whatsappQrBridgeScript)
	} else {
		bridgePath, err := qrBridgeScriptPath()
		if err != nil {
			return nil, fmt.Errorf("创建二维码桥接脚本失败: %w", err)
		}
		cmd = exec.Command(command, append([]string{bridgePath}, commandArgs...)...)
	}
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

	now := time.Now()
	session := &qrSession{
		QrData:     first.QrDataURL,
		ExpiresAt:  now.Add(qrCodeTTL),
		CleanupAt:  now.Add(qrCodeTTL + qrSessionCleanupWait),
		AccountID:  accountID,
		AccountKey: accountKey,
		Cmd:        cmd,
		Status:     "qr_pending",
		Events:     events,
		Stderr:     &stderr,
	}
	return session, nil
}

func monitorQrSession(session *qrSession, events <-chan qrBridgeEvent, stderr *bytes.Buffer) {
	bridgeConnected := false
	for event := range events {
		qrCacheMu.Lock()
		current, ok := qrCache[session.AccountID]
		if ok && current == session {
			switch {
			case event.Type == "qr" && strings.HasPrefix(event.QrDataURL, "data:image/png;base64,"):
				session.QrData = event.QrDataURL
				session.ExpiresAt = time.Now().Add(qrCodeTTL)
			case event.Type == "status" && event.Connected:
				bridgeConnected = true
				updateQrSessionStatus(session, channelConnectionStatus{Known: true, Linked: true}, time.Now())
			case event.Type == "error":
				session.Err = fmt.Errorf("%s", event.Error)
			}
		}
		qrCacheMu.Unlock()
	}
	waitErr := session.Cmd.Wait()
	if bridgeConnected && waitErr == nil {
		activationErr := activateOpenClawAccount(session.AccountID, session.AccountKey)
		qrCacheMu.Lock()
		if current, ok := qrCache[session.AccountID]; ok && current == session {
			if activationErr != nil {
				session.Err = activationErr
				session.Status = "expired"
			} else {
				updateQrSessionStatus(session, channelConnectionStatus{Known: true, Linked: true, Running: true, Connected: true}, time.Now())
			}
		}
		qrCacheMu.Unlock()
		return
	}
	if waitErr != nil {
		qrCacheMu.Lock()
		if current, ok := qrCache[session.AccountID]; ok && current == session && session.Status != "connected" && session.Err == nil {
			message := strings.TrimSpace(stderr.String())
			if message == "" {
				message = waitErr.Error()
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

func disconnectOpenClawAccount(accountKey string) error {
	ctx, cancel := context.WithTimeout(context.Background(), openClawCommandTimeout)
	defer cancel()
	output, err := openClawCommand(ctx, "channels", "logout", "--channel", "whatsapp", "--account", accountKey).CombinedOutput()
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(output))
	if message == "" {
		message = err.Error()
	}
	return fmt.Errorf("断开 OpenClaw WhatsApp 账号失败: %s", message)
}

// normalizeWhatsAppTarget accepts the two customer identifiers OpenClaw
// exposes for direct chats and rejects arbitrary conversation labels.
func normalizeWhatsAppTarget(conversationID string) (string, error) {
	target := strings.TrimSpace(conversationID)
	target = strings.TrimSuffix(target, "@s.whatsapp.net")
	target = strings.TrimSuffix(target, "@c.us")
	target = strings.TrimPrefix(target, "+")
	if len(target) < 7 || len(target) > 15 || target[0] == '0' {
		return "", fmt.Errorf("会话不是可发送的 WhatsApp 手机号")
	}
	for _, r := range target {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("会话不是可发送的 WhatsApp 手机号")
		}
	}
	return "+" + target, nil
}

func sendOpenClawWhatsAppMessage(accountKey, conversationID, content string) error {
	target, err := normalizeWhatsAppTarget(conversationID)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := openClawCommand(ctx,
		"message", "send",
		"--channel", "whatsapp",
		"--account", accountKey,
		"--target", target,
		"--message", content,
		"--json",
	).CombinedOutput()
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(output))
	if message == "" {
		message = err.Error()
	}
	return fmt.Errorf("OpenClaw 发送失败: %s", message)
}

func RegisterAccounts(r *gin.RouterGroup, st *store.Store) {
	r.GET("", handleListAccounts(st))
	RegisterAccountManagement(r, st)
}

func ListAccounts(st *store.Store) gin.HandlerFunc {
	return handleListAccounts(st)
}

// RegisterAccountManagement registers account mutations that require the
// accounts:manage tenant permission.
func RegisterAccountManagement(r *gin.RouterGroup, st *store.Store) {
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
		if isOpenClawAvailable() {
			// Sync live status in background — shelling out to openclaw is slow.
			tenantID := session.ActiveTenantID
			accountsForSync := accountsForLiveStatusSync(accounts)
			go func(accounts []model.Account) {
				statuses := readAllWhatsAppChannelStatuses()
				for _, account := range applyLiveAccountStatuses(accounts, statuses) {
					if _, err := st.UpdateAccount(tenantID, account.ID, "", account.Status, nil, nil, nil); err != nil {
						slog.Default().Warn("persist live account status", "tenant_id", tenantID, "account_id", account.ID, "error", err)
					}
				}
			}(accountsForSync)
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
		dailyLimit := 30
		if req.DailyLimit != nil {
			dailyLimit = *req.DailyLimit
		}
		if dailyLimit < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Daily limit cannot be negative."}})
			return
		}
		if req.ReplyLimit <= 0 {
			req.ReplyLimit = 30
		}
		knowledgeBasesValid, err := knowledgeBasesBelongToTenant(st, session.ActiveTenantID, req.KbID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to verify knowledge bases."}})
			return
		}
		if !knowledgeBasesValid {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "One or more knowledge bases are unavailable."}})
			return
		}
		kbIDJSON := marshalKbIDs(req.KbID)
		account, err := st.CreateAccount(session.ActiveTenantID, req.Name, kbIDJSON, dailyLimit, req.ReplyLimit)
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
		if req.Name == "" && req.Status == "" && req.KbID == nil && req.DailyLimit == nil && req.ReplyLimit == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "At least one field is required."}})
			return
		}
		if req.Status != "" && req.Status != "pending" && req.Status != "connected" && req.Status != "disabled" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid account status."}})
			return
		}
		if req.DailyLimit != nil && *req.DailyLimit < 0 || req.ReplyLimit != nil && *req.ReplyLimit <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Daily limit cannot be negative and reply limit must be positive."}})
			return
		}
		var kbID *string
		if req.KbID != nil {
			knowledgeBasesValid, err := knowledgeBasesBelongToTenant(st, session.ActiveTenantID, req.KbID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to verify knowledge bases."}})
				return
			}
			if !knowledgeBasesValid {
				c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "One or more knowledge bases are unavailable."}})
				return
			}
			s := marshalKbIDs(req.KbID)
			kbID = &s
		}
		account, err := st.UpdateAccount(session.ActiveTenantID, c.Param("id"), req.Name, req.Status, kbID, req.DailyLimit, req.ReplyLimit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to update account."}})
			return
		}
		if account.Status == "disabled" {
			changed, err := disableOpenClawRAGAccount(account.AccountKey)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "OPENCLAW_ERROR", Message: fmt.Sprintf("停用知识库客服失败: %v", err)}})
				return
			}
			if changed {
				if err := restartOpenClawGatewaySafely(); err != nil {
					c.JSON(http.StatusBadGateway, gin.H{"error": model.ErrorDetail{Code: "OPENCLAW_ERROR", Message: err.Error()}})
					return
				}
			}
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
		if !isOpenClawAvailable() {
			c.JSON(http.StatusBadGateway, gin.H{"error": model.ErrorDetail{Code: "OPENCLAW_ERROR", Message: "openclaw 未安装或不可用"}})
			return
		}
		live := readWhatsAppChannelStatus(acct.AccountKey)
		if live.Known && live.Running && live.Connected {
			changed, err := ensureOpenClawRAGAccount(acct.ID, acct.AccountKey)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "OPENCLAW_ERROR", Message: fmt.Sprintf("配置知识库客服失败: %v", err)}})
				return
			}
			if changed {
				if err := restartOpenClawGatewaySafely(); err != nil {
					c.JSON(http.StatusBadGateway, gin.H{"error": model.ErrorDetail{Code: "OPENCLAW_ERROR", Message: err.Error()}})
					return
				}
			}
			if _, err := st.UpdateAccount(session.ActiveTenantID, accountID, "", "connected", nil, nil, nil); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to update account status."}})
				return
			}
			c.JSON(http.StatusOK, model.QrLoginResponse{AccountID: accountID, Status: "connected"})
			return
		}

		if err := ensureOpenClawAccount(acct.AccountKey); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "OPENCLAW_ERROR", Message: fmt.Sprintf("注册 OpenClaw 账号失败: %v", err)}})
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
		channel := channelConnectionStatus{}
		if isOpenClawAvailable() {
			channel = readWhatsAppChannelStatus(acct.AccountKey)
		}

		now := time.Now()
		qrCacheMu.Lock()
		qr := qrCache[accountID]
		if qr != nil {
			switch updateQrSessionStatus(qr, channel, now) {
			case "connected":
				resp.Status = "connected"
				resp.ConnectedAt = now.Format("2006-01-02 15:04:05")
				delete(qrCache, accountID)
			case "connecting":
				resp.Status = "connecting"
				resp.ExpiresAt = qr.ConnectionDeadline.Format(time.RFC3339)
			case "qr_pending":
				resp.Status = "qr_pending"
				resp.QrData = qr.QrData
				resp.ExpiresAt = qr.ExpiresAt.Format(time.RFC3339)
			case "expired":
				resp.Status = "expired"
				if qr.Err != nil {
					resp.Error = qr.Err.Error()
				}
				stopQrSession(qr)
				delete(qrCache, accountID)
			}
		} else if channel.Known && channel.Connected {
			resp.Status = "connected"
			resp.ConnectedAt = now.Format("2006-01-02 15:04:05")
		}
		qrCacheMu.Unlock()

		if resp.Status == "connected" {
			if acct.AccountKey != "" {
				if err := ensureOpenClawAccount(acct.AccountKey); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "OPENCLAW_ERROR", Message: fmt.Sprintf("注册 OpenClaw 账号失败: %v", err)}})
					return
				}
			}
			changed, err := ensureOpenClawRAGAccount(acct.ID, acct.AccountKey)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "OPENCLAW_ERROR", Message: fmt.Sprintf("配置知识库客服失败: %v", err)}})
				return
			}
			if changed {
				if err := restartOpenClawGatewaySafely(); err != nil {
					c.JSON(http.StatusBadGateway, gin.H{"error": model.ErrorDetail{Code: "OPENCLAW_ERROR", Message: err.Error()}})
					return
				}
			}
			if _, err := st.UpdateAccount(session.ActiveTenantID, accountID, "", "connected", nil, nil, nil); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to update account status."}})
				return
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

		if err := disconnectOpenClawAccount(acct.AccountKey); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": model.ErrorDetail{Code: "OPENCLAW_ERROR", Message: err.Error()}})
			return
		}
		changed, err := disableOpenClawRAGAccount(acct.AccountKey)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "OPENCLAW_ERROR", Message: fmt.Sprintf("停用知识库客服失败: %v", err)}})
			return
		}
		if changed {
			if err := restartOpenClawGatewaySafely(); err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"error": model.ErrorDetail{Code: "OPENCLAW_ERROR", Message: err.Error()}})
				return
			}
		}

		qrCacheMu.Lock()
		stopQrSession(qrCache[accountID])
		delete(qrCache, accountID)
		qrCacheMu.Unlock()

		if _, err := st.UpdateAccount(session.ActiveTenantID, accountID, "", "pending", nil, nil, nil); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to update account status."}})
			return
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

func marshalKbIDs(ids []string) string {
	if len(ids) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(ids)
	return string(b)
}

func knowledgeBasesBelongToTenant(st *store.Store, tenantID string, ids []string) (bool, error) {
	for _, id := range ids {
		if id == "" {
			return false, nil
		}
		if _, err := st.KnowledgeBaseByID(id, tenantID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return false, nil
			}
			return false, err
		}
	}
	return true, nil
}
