package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"whatsapp-ai-poc/internal/model"
)

func TestRegisterAccountManagementIncludesDeleteRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterAccountManagement(router.Group("/api/accounts"), nil)

	for _, route := range router.Routes() {
		if route.Method == http.MethodDelete && route.Path == "/api/accounts/:id" {
			return
		}
	}
	t.Fatal("account deletion route is not registered")
}

func TestQrSessionWaitsOneMinuteForConnectionAfterScan(t *testing.T) {
	scannedAt := time.Date(2026, 7, 14, 12, 0, 29, 0, time.UTC)
	session := &qrSession{
		Status:    "qr_pending",
		ExpiresAt: scannedAt.Add(time.Second),
	}

	status := updateQrSessionStatus(session, channelConnectionStatus{
		Known:  true,
		Linked: true,
	}, scannedAt)
	if status != "connecting" {
		t.Fatalf("status after scan = %q, want connecting", status)
	}
	if got, want := session.ConnectionDeadline, scannedAt.Add(time.Minute); !got.Equal(want) {
		t.Fatalf("connection deadline = %v, want %v", got, want)
	}

	status = updateQrSessionStatus(session, channelConnectionStatus{}, scannedAt.Add(59*time.Second))
	if status != "connecting" {
		t.Fatalf("status before connection deadline = %q, want connecting", status)
	}

	status = updateQrSessionStatus(session, channelConnectionStatus{}, scannedAt.Add(time.Minute))
	if status != "expired" {
		t.Fatalf("status at connection deadline = %q, want expired", status)
	}
}

func TestQrSessionReportsConnectedAfterScan(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	session := &qrSession{
		Status:             "connecting",
		ExpiresAt:          now.Add(-time.Second),
		ConnectionDeadline: now.Add(time.Minute),
	}

	status := updateQrSessionStatus(session, channelConnectionStatus{
		Known:     true,
		Linked:    true,
		Running:   true,
		Connected: true,
	}, now)
	if status != "connected" {
		t.Fatalf("status after connection = %q, want connected", status)
	}
}

func TestQrSessionReportsBridgeFailureImmediately(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	session := &qrSession{
		Status:    "qr_pending",
		ExpiresAt: now.Add(qrCodeTTL),
		Err:       fmt.Errorf("OpenClaw login failed"),
	}

	if status := updateQrSessionStatus(session, channelConnectionStatus{}, now); status != "expired" {
		t.Fatalf("status after bridge failure = %q, want expired", status)
	}
	if !session.CleanupAt.Equal(now) {
		t.Fatalf("cleanup at = %v, want %v", session.CleanupAt, now)
	}
}

func TestQrStatusSnapshotUsesActiveBridgeSession(t *testing.T) {
	now := time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC)
	accountID := "account-qr-status"
	qrCacheMu.Lock()
	qrCache[accountID] = &qrSession{
		AccountID: accountID,
		Status:    "qr_pending",
		QrData:    "data:image/png;base64,ZmFrZQ==",
		ExpiresAt: now.Add(qrCodeTTL),
		CleanupAt: now.Add(qrCodeTTL + qrSessionCleanupWait),
	}
	qrCacheMu.Unlock()
	t.Cleanup(func() {
		qrCacheMu.Lock()
		delete(qrCache, accountID)
		qrCacheMu.Unlock()
	})

	resp, ok := qrSessionStatusSnapshot(accountID, "pending", now)
	if !ok {
		t.Fatal("active QR bridge session was not found")
	}
	if resp.Status != "qr_pending" || resp.QrData == "" {
		t.Fatalf("QR status response = %#v", resp)
	}
}

func TestQrBridgeAcceptsAlreadyConnectedAccount(t *testing.T) {
	status, err := initialQrBridgeStatus(qrBridgeEvent{Type: "status", Connected: true})
	if err != nil {
		t.Fatal(err)
	}
	if status != "connected" {
		t.Fatalf("initial bridge status = %q, want connected", status)
	}
}

func TestStartedQrSessionCannotReplaceNewerLifecycle(t *testing.T) {
	accountID := "account-session-owner"
	starting := &qrSession{AccountID: accountID, Status: "starting"}
	newer := &qrSession{AccountID: accountID, Status: "qr_pending"}
	started := &qrSession{AccountID: accountID, Status: "qr_pending"}
	qrCacheMu.Lock()
	qrCache[accountID] = newer
	qrCacheMu.Unlock()
	t.Cleanup(func() {
		qrCacheMu.Lock()
		delete(qrCache, accountID)
		qrCacheMu.Unlock()
	})

	if installStartedQrSession(accountID, starting, started) {
		t.Fatal("stale QR startup replaced a newer session")
	}
	qrCacheMu.Lock()
	current := qrCache[accountID]
	qrCacheMu.Unlock()
	if current != newer {
		t.Fatalf("current QR session = %p, want newer session %p", current, newer)
	}
}

func TestDeletingAccountBlocksQrActivation(t *testing.T) {
	accountID := "account-being-deleted"
	beginAccountDeletion(accountID)
	t.Cleanup(func() { cancelAccountDeletion(accountID) })
	called := false
	err := runQrActivation(accountID, func() error {
		called = true
		return nil
	})
	if !errors.Is(err, errAccountDeletionInProgress) {
		t.Fatalf("activation error = %v, want deletion in progress", err)
	}
	if called {
		t.Fatal("QR activation ran while account deletion owned the lifecycle")
	}
}

func TestAccountDeletionRequestsCoalesceAndCanRetry(t *testing.T) {
	accountID := "account-delete-request"
	cancelAccountDeletion(accountID)
	t.Cleanup(func() { cancelAccountDeletion(accountID) })
	if !requestAccountDeletion(accountID) {
		t.Fatal("first deletion request was not accepted")
	}
	if requestAccountDeletion(accountID) {
		t.Fatal("duplicate deletion request started another worker")
	}
	cancelAccountDeletion(accountID)
	if !requestAccountDeletion(accountID) {
		t.Fatal("deletion request could not retry after worker failure")
	}
}

func TestOpenClawLogoutAlreadyCompleteIsIdempotent(t *testing.T) {
	for _, message := range []string{
		"Account not found",
		"WhatsApp account is not linked",
		"Already logged out",
		"No credentials available",
	} {
		if !openClawLogoutAlreadyComplete(message) {
			t.Fatalf("logout message %q was not treated as complete", message)
		}
	}
	if openClawLogoutAlreadyComplete("gateway connection timed out") {
		t.Fatal("real gateway failure was treated as a completed logout")
	}
}

func TestWaitForQrBridgeProcessExitIsBounded(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 5")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	waitForQrBridgeProcessExit(cmd, 20*time.Millisecond)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("bridge process wait took %v", elapsed)
	}
	if cmd.ProcessState == nil {
		t.Fatal("bridge process was not reaped")
	}
}

func TestResolveWhatsAppLoginModuleUsesInstalledExtensionWithoutOpenClawCLI(t *testing.T) {
	whatsAppModuleMu.Lock()
	whatsAppModulePath = ""
	whatsAppModuleMu.Unlock()
	t.Cleanup(func() {
		whatsAppModuleMu.Lock()
		whatsAppModulePath = ""
		whatsAppModuleMu.Unlock()
	})
	home := t.TempDir()
	modulePath := filepath.Join(home, ".openclaw", "extensions", "whatsapp", "dist", "login-qr-runtime.js")
	if err := os.MkdirAll(filepath.Dir(modulePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modulePath, []byte("export {};"), 0o600); err != nil {
		t.Fatal(err)
	}
	binDir := t.TempDir()
	calledPath := filepath.Join(binDir, "called")
	script := "#!/bin/sh\ntouch " + calledPath + "\nexit 1\n"
	if err := os.WriteFile(filepath.Join(binDir, "openclaw"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("OPENCLAW_DOCKER_CONTAINER", "")
	t.Setenv("OPENCLAW_WHATSAPP_LOGIN_MODULE", "")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	got, err := resolveWhatsAppLoginModule()
	if err != nil {
		t.Fatal(err)
	}
	if got != modulePath {
		t.Fatalf("module path = %q, want %q", got, modulePath)
	}
	if _, err := os.Stat(calledPath); !os.IsNotExist(err) {
		t.Fatal("OpenClaw CLI was called despite installed WhatsApp module")
	}
}

func TestResolveWhatsAppLoginModuleRetriesAfterDiscoveryFailure(t *testing.T) {
	whatsAppModuleMu.Lock()
	whatsAppModulePath = ""
	whatsAppModuleMu.Unlock()
	t.Cleanup(func() {
		whatsAppModuleMu.Lock()
		whatsAppModulePath = ""
		whatsAppModuleMu.Unlock()
	})

	modulePath := filepath.Join(t.TempDir(), "login-qr-runtime.js")
	t.Setenv("OPENCLAW_DOCKER_CONTAINER", "")
	t.Setenv("OPENCLAW_WHATSAPP_LOGIN_MODULE", modulePath)
	if _, err := resolveWhatsAppLoginModule(); err == nil {
		t.Fatal("missing module discovery unexpectedly succeeded")
	}
	if err := os.WriteFile(modulePath, []byte("export {};"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := resolveWhatsAppLoginModule()
	if err != nil {
		t.Fatal(err)
	}
	if got != modulePath {
		t.Fatalf("module path after retry = %q, want %q", got, modulePath)
	}
}

func TestResolveWhatsAppLoginModuleDoesNotCallOpenClawCLI(t *testing.T) {
	whatsAppModuleMu.Lock()
	whatsAppModulePath = ""
	whatsAppModuleMu.Unlock()
	t.Cleanup(func() {
		whatsAppModuleMu.Lock()
		whatsAppModulePath = ""
		whatsAppModuleMu.Unlock()
	})

	home := t.TempDir()
	binDir := t.TempDir()
	calledPath := filepath.Join(binDir, "called")
	script := "#!/bin/sh\ntouch " + calledPath + "\nexit 1\n"
	if err := os.WriteFile(filepath.Join(binDir, "openclaw"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("OPENCLAW_DOCKER_CONTAINER", "")
	t.Setenv("OPENCLAW_WHATSAPP_LOGIN_MODULE", "")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if _, err := resolveWhatsAppLoginModule(); err == nil {
		t.Fatal("module resolution unexpectedly succeeded")
	}
	if _, err := os.Stat(calledPath); !os.IsNotExist(err) {
		t.Fatal("request-path module resolution called OpenClaw CLI")
	}
}

func TestEnsureOpenClawAccountConfigEnablesTokenAccessWithoutChangingGlobalTools(t *testing.T) {
	cfg := map[string]any{
		"gateway": map[string]any{},
		"channels": map[string]any{
			"whatsapp": map[string]any{
				"dmPolicy": "pairing",
			},
		},
		"tools": map[string]any{
			"profile": "full",
			"exec":    map[string]any{"mode": "full"},
		},
		"plugins": map[string]any{"allow": []string{"existing-plugin"}},
	}

	if !ensureOpenClawAccountConfig(cfg, "wa_support") {
		t.Fatal("expected initial registration to change config")
	}
	if ensureOpenClawAccountConfig(cfg, "wa_support") {
		t.Fatal("duplicate registration changed config")
	}
	encoded, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded map[string]any
	if err := json.Unmarshal(encoded, &reloaded); err != nil {
		t.Fatal(err)
	}
	if ensureOpenClawAccountConfig(reloaded, "wa_support") {
		t.Fatal("registration changed config after JSON reload")
	}

	channels := cfg["channels"].(map[string]any)
	whatsApp := channels["whatsapp"].(map[string]any)
	if whatsApp["dmPolicy"] != "open" {
		t.Fatalf("dm policy = %v, want open", whatsApp["dmPolicy"])
	}
	allowFrom := whatsApp["allowFrom"].([]string)
	if len(allowFrom) != 1 || allowFrom[0] != "*" {
		t.Fatalf("allow from = %#v, want wildcard", allowFrom)
	}
	if cfg["gateway"].(map[string]any)["auth"].(map[string]any)["mode"] != "token" {
		t.Fatalf("gateway auth = %#v, want token mode", cfg["gateway"])
	}
	tools := cfg["tools"].(map[string]any)
	if tools["profile"] != "full" || tools["exec"].(map[string]any)["mode"] != "full" {
		t.Fatalf("global tools changed: %#v", tools)
	}
	pluginAllow := cfg["plugins"].(map[string]any)["allow"].([]string)
	if !slices.Equal(pluginAllow, []string{"existing-plugin"}) {
		t.Fatalf("plugin allowlist changed: %#v", pluginAllow)
	}
	accounts := whatsApp["accounts"].(map[string]any)
	account, ok := accounts["wa_support"].(map[string]any)
	if !ok || account["enabled"] != true {
		t.Fatalf("registered account = %#v", accounts["wa_support"])
	}
}

func TestEnsureOpenClawAccountConfigCreatesMissingChannels(t *testing.T) {
	cfg := map[string]any{}
	ensureOpenClawAccountConfig(cfg, "wa_support")

	channels := cfg["channels"].(map[string]any)
	whatsApp := channels["whatsapp"].(map[string]any)
	accounts := whatsApp["accounts"].(map[string]any)
	if _, ok := accounts["wa_support"]; !ok {
		t.Fatal("missing registered account")
	}
}

func TestEnsureOpenClawRAGConfigCreatesIsolatedMCPAndRoutePerAccount(t *testing.T) {
	cfg := map[string]any{
		"mcp": map[string]any{
			"servers": map[string]any{
				"whatsapp-rag": map[string]any{"command": "node"},
			},
		},
	}
	options := openClawRAGOptions{
		APIURL:    "https://whatsapp.example.com",
		APIToken:  "test-token",
		MCPPath:   "/home/node/.openclaw/whatsapp-rag-mcp/index.mjs",
		Workspace: "/home/node/.openclaw/workspaces",
	}

	if err := ensureOpenClawRAGConfig(cfg, "account-sales", "wa_sales", options); err != nil {
		t.Fatal(err)
	}
	if err := ensureOpenClawRAGConfig(cfg, "account-support", "wa_support", options); err != nil {
		t.Fatal(err)
	}
	if err := ensureOpenClawRAGConfig(cfg, "account-sales", "wa_sales", options); err != nil {
		t.Fatal(err)
	}

	servers := cfg["mcp"].(map[string]any)["servers"].(map[string]any)
	if _, exists := servers["whatsapp-rag"]; exists {
		t.Fatal("legacy global RAG MCP remains configured")
	}
	sales := servers[openClawRAGMCPName("wa_sales")].(map[string]any)
	support := servers[openClawRAGMCPName("wa_support")].(map[string]any)
	toolFilter := sales["toolFilter"].(map[string]any)["include"].([]string)
	if !slices.Equal(toolFilter, []string{"search_knowledge", "save_reply"}) {
		t.Fatalf("MCP tool filter = %#v", toolFilter)
	}
	if sales["env"].(map[string]any)["WHATSAPP_AI_ACCOUNT_ID"] != "account-sales" {
		t.Fatalf("sales MCP account ID = %#v", sales["env"])
	}
	if support["env"].(map[string]any)["WHATSAPP_AI_ACCOUNT_ID"] != "account-support" {
		t.Fatalf("support MCP account ID = %#v", support["env"])
	}

	bindings := cfg["bindings"].([]any)
	if len(bindings) != 2 {
		t.Fatalf("bindings = %#v, want two account routes", bindings)
	}
	for _, raw := range bindings {
		binding := raw.(map[string]any)
		match := binding["match"].(map[string]any)
		if match["channel"] != "whatsapp" || match["accountId"] == "" {
			t.Fatalf("binding = %#v, want WhatsApp account route", binding)
		}
		if binding["agentId"] != openClawRAGAgentID(match["accountId"].(string)) {
			t.Fatalf("binding = %#v, want matching per-account agent", binding)
		}
	}

	agents := cfg["agents"].(map[string]any)["list"].([]any)
	if len(agents) != 2 {
		t.Fatalf("agents = %#v, want one agent per account", agents)
	}
	for _, raw := range agents {
		agent := raw.(map[string]any)
		tools := agent["tools"].(map[string]any)
		if tools["profile"] != "messaging" {
			t.Fatalf("agent tool profile = %#v, want messaging", tools)
		}
		// The live agent id is "whatsapp-<key>"; the tools.allow entry uses the
		// MCP server name "whatsapp-rag-<key>". They differ by design.
		accountKey := strings.TrimPrefix(agent["id"].(string), "whatsapp-")
		mcpName := openClawRAGMCPName(accountKey)
		wantAllow := []string{
			mcpName + "__search_knowledge",
			mcpName + "__save_reply",
		}
		if allow := tools["allow"].([]string); !slices.Equal(allow, wantAllow) {
			t.Fatalf("agent tool allowlist = %#v, want %#v", allow, wantAllow)
		}
		if agent["sandbox"].(map[string]any)["mode"] != "off" {
			t.Fatalf("agent sandbox = %#v, want off", agent["sandbox"])
		}
	}
}

func TestRemoveOpenClawRAGConfigOnlyRemovesRequestedAccount(t *testing.T) {
	cfg := map[string]any{}
	options := openClawRAGOptions{APIURL: "http://localhost", APIToken: "token", MCPPath: "/mcp/index.mjs", Workspace: "/workspaces"}
	if err := ensureOpenClawRAGConfig(cfg, "account-sales", "wa_sales", options); err != nil {
		t.Fatal(err)
	}
	if err := ensureOpenClawRAGConfig(cfg, "account-support", "wa_support", options); err != nil {
		t.Fatal(err)
	}
	channels := map[string]any{
		"whatsapp": map[string]any{
			"accounts": map[string]any{
				"wa_sales":   map[string]any{"enabled": true},
				"wa_support": map[string]any{"enabled": true},
			},
		},
	}
	cfg["channels"] = channels

	if !removeOpenClawRAGConfig(cfg, "wa_sales") {
		t.Fatal("expected account removal")
	}
	servers := cfg["mcp"].(map[string]any)["servers"].(map[string]any)
	if _, exists := servers[openClawRAGMCPName("wa_sales")]; exists {
		t.Fatal("sales MCP remains")
	}
	if _, exists := servers[openClawRAGMCPName("wa_support")]; !exists {
		t.Fatal("support MCP was removed")
	}
	if channels["whatsapp"].(map[string]any)["accounts"].(map[string]any)["wa_sales"].(map[string]any)["enabled"] != false {
		t.Fatal("sales channel remains enabled")
	}
}

func TestDeleteOpenClawRAGConfigRemovesChannelAccount(t *testing.T) {
	cfg := map[string]any{}
	options := openClawRAGOptions{APIURL: "http://localhost", APIToken: "token", MCPPath: "/mcp/index.mjs", Workspace: "/workspaces"}
	if err := ensureOpenClawRAGConfig(cfg, "account-sales", "wa_sales", options); err != nil {
		t.Fatal(err)
	}
	ensureOpenClawAccountConfig(cfg, "wa_sales")

	if !deleteOpenClawRAGConfig(cfg, "wa_sales") {
		t.Fatal("expected account config deletion")
	}
	accounts := cfg["channels"].(map[string]any)["whatsapp"].(map[string]any)["accounts"].(map[string]any)
	if _, exists := accounts["wa_sales"]; exists {
		t.Fatal("deleted WhatsApp account remains configured")
	}
	servers := cfg["mcp"].(map[string]any)["servers"].(map[string]any)
	if _, exists := servers[openClawRAGMCPName("wa_sales")]; exists {
		t.Fatal("deleted account MCP remains configured")
	}
}

func TestDisableOpenClawRAGAccountAtMissingPathIsAlreadyDisabled(t *testing.T) {
	changed, err := disableOpenClawRAGAccountAtPath(filepath.Join(t.TempDir(), "missing.json"), "wa_support")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("missing config was reported as changed")
	}
}

func TestSameOpenClawConfigIgnoresTrailingNewline(t *testing.T) {
	config := []byte("{\n  \"mcp\": {}\n}")
	if !sameOpenClawConfig(append(config, '\n'), config) {
		t.Fatal("trailing newline must not trigger an OpenClaw gateway restart")
	}
}

func TestPrepareOpenClawRAGTokenStoresSecretOutsideConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "openclaw.json")
	path := filepath.Join(filepath.Dir(configPath), ".env")
	if err := os.WriteFile(path, []byte("OPENCLAW_GATEWAY_TOKEN=existing\nINTERNAL_API_TOKEN=old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	configToken, changed, err := prepareOpenClawRAGToken(configPath, "new-token")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("token rotation was not reported as a configuration change")
	}
	if configToken != internalAPITokenEnvRef {
		t.Fatalf("config token = %q, want %q", configToken, internalAPITokenEnvRef)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "OPENCLAW_GATEWAY_TOKEN=existing\nINTERNAL_API_TOKEN=\"new-token\"\n"
	if string(content) != want {
		t.Fatalf("env content = %q, want %q", content, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("env permissions = %o, want 600", info.Mode().Perm())
	}
	_, changed, err = prepareOpenClawRAGToken(configPath, "new-token")
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("unchanged token was reported as a configuration change")
	}
}

func TestWriteOpenClawRAGWorkspaceAddsPolicyToExistingWorkspace(t *testing.T) {
	workspace := t.TempDir()
	// writeOpenClawRAGWorkspace writes AGENTS.md at the workspace root keyed
	// by accountKey (matches the live wa agent workspace layout).
	agentDir := filepath.Join(workspace, "wa_support")
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	policyPath := filepath.Join(agentDir, "AGENTS.md")
	if err := os.WriteFile(policyPath, []byte("# Existing instructions\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := writeOpenClawRAGWorkspace(workspace, "wa_support"); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	policy := string(content)
	for _, expected := range []string{
		"# Existing instructions",
		openClawRAGPolicyStart,
		"call search_knowledge",
		"original key terms plus relevant synonyms",
		"retry once with broader synonyms",
		"conversation history returned by the tool",
		"fresh, natural answer in your own words",
		"Never write, explain, debug, or execute code",
		"may call only search_knowledge and save_reply",
		"call save_reply with that answer before final delivery",
		"return exactly the same answer and nothing else",
	} {
		if !strings.Contains(policy, expected) {
			t.Fatalf("workspace policy missing %q: %q", expected, content)
		}
	}
}

func TestWriteOpenClawRAGWorkspaceRemovesLegacyDuplicatePolicy(t *testing.T) {
	workspace := t.TempDir()
	// writeOpenClawRAGWorkspace writes AGENTS.md at the workspace root keyed
	// by accountKey (matches the live wa agent workspace layout).
	agentDir := filepath.Join(workspace, "wa_support")
	if err := os.MkdirAll(agentDir, 0o700); err != nil {
		t.Fatal(err)
	}
	policyPath := filepath.Join(agentDir, "AGENTS.md")
	content := openClawRAGPolicyBody() + openClawRAGPolicy()
	if err := os.WriteFile(policyPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := writeOpenClawRAGWorkspace(workspace, "wa_support"); err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(updated), "# WhatsApp Knowledge-Base Reply Policy") != 1 {
		t.Fatalf("workspace policy was not deduplicated: %q", updated)
	}
}

func TestOpenClawCommandSpecUsesConfiguredDockerContainer(t *testing.T) {
	t.Setenv("OPENCLAW_DOCKER_CONTAINER", "openclaw")
	command, args := openClawCommandSpec("channels", "status", "--json")

	if command != "docker" {
		t.Fatalf("command = %q, want docker", command)
	}
	want := []string{"exec", "openclaw", "openclaw", "channels", "status", "--json"}
	if !slices.Equal(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestOpenClawCommandSpecDefaultsToHostCLI(t *testing.T) {
	t.Setenv("OPENCLAW_DOCKER_CONTAINER", "")
	command, args := openClawCommandSpec("channels", "status", "--json")

	if command != "openclaw" {
		t.Fatalf("command = %q, want openclaw", command)
	}
	want := []string{"channels", "status", "--json"}
	if !slices.Equal(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestValidateOpenClawRAGAgentModelAuthUsesAgentStatus(t *testing.T) {
	dir := t.TempDir()
	argsPath := filepath.Join(dir, "args")
	script := "#!/bin/sh\n" +
		"printf '%s' \"$*\" > " + argsPath + "\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "openclaw"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENCLAW_DOCKER_CONTAINER", "")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	if err := validateOpenClawRAGAgentModelAuth("wa_support"); err != nil {
		t.Fatal(err)
	}
	args, err := os.ReadFile(argsPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "models status --agent whatsapp-wa_support --check --plain"
	if string(args) != want {
		t.Fatalf("model auth command = %q, want %q", args, want)
	}
}

func TestOpenClawGatewayRestartUsesDockerContainerRestart(t *testing.T) {
	t.Setenv("OPENCLAW_DOCKER_CONTAINER", "openclaw")
	command, args := openClawGatewayRestartCommandSpec()
	if command != "docker" || !slices.Equal(args, []string{"restart", "openclaw"}) {
		t.Fatalf("restart command = %q %#v", command, args)
	}
}

func TestOpenClawBridgeCommandSpecUsesConfiguredDockerContainer(t *testing.T) {
	t.Setenv("OPENCLAW_DOCKER_CONTAINER", "openclaw")
	command, args := openClawBridgeCommandSpec("/home/node/login-qr-runtime.js", "wa_support")

	if command != "docker" {
		t.Fatalf("command = %q, want docker", command)
	}
	want := []string{"exec", "-i", "openclaw", "node", "--input-type=module", "-", "/home/node/login-qr-runtime.js", "wa_support"}
	if !slices.Equal(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestEnsureOpenClawAccountAtPathPreservesConcurrentRegistrations(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "openclaw.json")
	if err := os.WriteFile(configPath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	accountKeys := []string{"wa_sales", "wa_support", "wa_returns", "wa_vip"}
	var wg sync.WaitGroup
	errs := make(chan error, len(accountKeys))
	for _, key := range accountKeys {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- ensureOpenClawAccountAtPath(configPath, key)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	accounts := cfg["channels"].(map[string]any)["whatsapp"].(map[string]any)["accounts"].(map[string]any)
	for _, key := range accountKeys {
		if _, ok := accounts[key]; !ok {
			t.Fatalf("missing account %q in %#v", key, accounts)
		}
	}
}

func TestEnsureOpenClawAccountAtPathCreatesMissingConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "nested", "openclaw.json")
	if err := ensureOpenClawAccountAtPath(configPath, "wa_support"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	accounts := cfg["channels"].(map[string]any)["whatsapp"].(map[string]any)["accounts"].(map[string]any)
	if _, ok := accounts["wa_support"]; !ok {
		t.Fatalf("missing account in %#v", accounts)
	}
}

func TestRestartAndWaitForOpenClawAccountRequiresRunningConnection(t *testing.T) {
	restartCalls := 0
	statusCalls := 0
	err := restartAndWaitForOpenClawAccount(
		"wa_support",
		func() error {
			restartCalls++
			return nil
		},
		func(accountKey string) channelConnectionStatus {
			statusCalls++
			if accountKey != "wa_support" {
				t.Fatalf("status account = %q, want wa_support", accountKey)
			}
			if statusCalls == 1 {
				return channelConnectionStatus{Known: true, Linked: true, Connected: true}
			}
			return channelConnectionStatus{Known: true, Linked: true, Running: true, Connected: true}
		},
		func() {},
		3,
	)
	if err != nil {
		t.Fatalf("restart and wait: %v", err)
	}
	if restartCalls != 1 {
		t.Fatalf("restart calls = %d, want 1", restartCalls)
	}
	if statusCalls != 2 {
		t.Fatalf("status calls = %d, want 2", statusCalls)
	}
}

func TestApplyLiveAccountStatusesClearsStaleConnection(t *testing.T) {
	accounts := []model.Account{{
		ID:         "account-1",
		AccountKey: "wa_support",
		Status:     "connected",
	}}
	statuses := map[string]channelConnectionStatus{
		"wa_support": {Known: true, Linked: true, Running: false, Connected: false},
	}

	changed := applyLiveAccountStatuses(accounts, statuses)
	if accounts[0].Status != "pending" {
		t.Fatalf("account status = %q, want pending", accounts[0].Status)
	}
	if len(changed) != 1 || changed[0].ID != "account-1" {
		t.Fatalf("changed accounts = %#v, want account-1", changed)
	}
}

func TestReadAllWhatsAppStatusesCoalescesConcurrentRefreshes(t *testing.T) {
	dir := t.TempDir()
	countPath := filepath.Join(dir, "calls")
	script := "#!/bin/sh\n" +
		"echo call >> " + countPath + "\n" +
		"sleep 0.1\n" +
		"printf '%s\\n' '{\"channelAccounts\":{\"whatsapp\":[{\"accountId\":\"wa_support\",\"linked\":true,\"running\":true,\"connected\":true}]}}'\n"
	if err := os.WriteFile(filepath.Join(dir, "openclaw"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENCLAW_DOCKER_CONTAINER", "")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	resetChannelStatusCache(t)

	const callers = 12
	var wg sync.WaitGroup
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			statuses := readAllWhatsAppChannelStatuses()
			if !statuses["wa_support"].Connected {
				t.Errorf("status = %#v, want connected", statuses)
			}
		}()
	}
	wg.Wait()

	data, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatal(err)
	}
	if calls := len(strings.Fields(string(data))); calls != 1 {
		t.Fatalf("OpenClaw status calls = %d, want 1", calls)
	}
}

func TestReadAllWhatsAppStatusesCachesFailedRefreshes(t *testing.T) {
	dir := t.TempDir()
	countPath := filepath.Join(dir, "calls")
	script := "#!/bin/sh\n" +
		"echo call >> " + countPath + "\n" +
		"sleep 0.1\n" +
		"exit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "openclaw"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENCLAW_DOCKER_CONTAINER", "")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	resetChannelStatusCache(t)

	const callers = 12
	var wg sync.WaitGroup
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			if statuses := readAllWhatsAppChannelStatuses(); statuses != nil {
				t.Errorf("statuses = %#v, want nil", statuses)
			}
		}()
	}
	wg.Wait()

	data, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatal(err)
	}
	if calls := len(strings.Fields(string(data))); calls != 1 {
		t.Fatalf("failed OpenClaw status calls = %d, want 1", calls)
	}
}

func TestRefreshAllWhatsAppStatusesAsyncDoesNotBlockCaller(t *testing.T) {
	dir := t.TempDir()
	countPath := filepath.Join(dir, "calls")
	script := "#!/bin/sh\n" +
		"echo call >> " + countPath + "\n" +
		"sleep 0.3\n" +
		"printf '%s\\n' '{\"channelAccounts\":{\"whatsapp\":[{\"accountId\":\"wa_support\",\"linked\":true,\"running\":true,\"connected\":true}]}}'\n"
	if err := os.WriteFile(filepath.Join(dir, "openclaw"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENCLAW_DOCKER_CONTAINER", "")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	resetChannelStatusCache(t)

	started := time.Now()
	refreshAllWhatsAppChannelStatusesAsync()
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("async refresh blocked for %v", elapsed)
	}
	for range 5 {
		refreshAllWhatsAppChannelStatusesAsync()
	}
	if statuses := readAllWhatsAppChannelStatuses(); !statuses["wa_support"].Connected {
		t.Fatalf("status = %#v, want connected", statuses)
	}

	data, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatal(err)
	}
	if calls := len(strings.Fields(string(data))); calls != 1 {
		t.Fatalf("async OpenClaw status calls = %d, want 1", calls)
	}
}

func TestInvalidateWhatsAppStatusesDiscardsInFlightStaleResult(t *testing.T) {
	dir := t.TempDir()
	startedPath := filepath.Join(dir, "started")
	script := "#!/bin/sh\n" +
		"touch " + startedPath + "\n" +
		"sleep 0.2\n" +
		"printf '%s\\n' '{\"channelAccounts\":{\"whatsapp\":[{\"accountId\":\"wa_support\",\"linked\":true,\"running\":false,\"connected\":false}]}}'\n"
	if err := os.WriteFile(filepath.Join(dir, "openclaw"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OPENCLAW_DOCKER_CONTAINER", "")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	resetChannelStatusCache(t)

	refreshAllWhatsAppChannelStatusesAsync()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(startedPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("status refresh did not start")
		}
		time.Sleep(5 * time.Millisecond)
	}

	invalidateWhatsAppChannelStatuses()
	if statuses := readAllWhatsAppChannelStatuses(); statuses != nil {
		t.Fatalf("stale statuses = %#v, want discarded", statuses)
	}
	if statuses := cachedWhatsAppChannelStatuses(); statuses != nil {
		t.Fatalf("cached stale statuses = %#v, want nil", statuses)
	}
}

func resetChannelStatusCache(t *testing.T) {
	t.Helper()
	channelStatusCacheMu.Lock()
	defer channelStatusCacheMu.Unlock()
	if channelStatusRefresh {
		t.Fatal("cannot reset channel status cache during refresh")
	}
	channelStatusCache = nil
	channelStatusCacheAt = time.Time{}
	channelStatusDone = nil
	channelStatusVersion = 0
}

func TestParseWhatsAppChannelStatusUsesRequestedAccount(t *testing.T) {
	payload := []byte(`{
  "channels": {
    "whatsapp": {"linked": false, "connected": false}
  },
  "channelAccounts": {
    "whatsapp": [
      {"accountId": "default", "linked": false, "connected": false},
      {"accountId": "wa_support", "linked": true, "connected": false}
    ]
  }
}`)

	status := parseWhatsAppChannelStatus(payload, "wa_support")
	if !status.Known || !status.Linked || status.Connected {
		t.Fatalf("requested account status = %#v, want linked but not connected", status)
	}
}

func TestParseWhatsAppChannelStatusDoesNotFallbackToDefaultAccount(t *testing.T) {
	payload := []byte(`{
  "channels": {
    "whatsapp": {"linked": true, "running": true, "connected": true}
  },
  "channelAccounts": {
    "whatsapp": [
      {"accountId": "default", "linked": true, "running": true, "connected": true}
    ]
  }
}`)

	status := parseWhatsAppChannelStatus(payload, "wa_support")
	if status.Known || status.Linked || status.Running || status.Connected {
		t.Fatalf("missing account status = %#v, want unknown", status)
	}
}

func TestParseQrBridgeEventPreservesPngDataURL(t *testing.T) {
	event, err := parseQrBridgeEvent([]byte(`{"type":"qr","qrDataUrl":"data:image/png;base64,ZmFrZQ=="}`))
	if err != nil {
		t.Fatalf("parse QR bridge event: %v", err)
	}
	if event.Type != "qr" {
		t.Fatalf("event type = %q, want qr", event.Type)
	}
	if event.QrDataURL != "data:image/png;base64,ZmFrZQ==" {
		t.Fatalf("QR data URL was not preserved: %q", event.QrDataURL)
	}
}

func TestWhatsAppQrBridgeStreamsNativePngAndConnection(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}

	dir := t.TempDir()
	modulePath := filepath.Join(dir, "fake-login.mjs")
	module := `
export async function startWebLoginWithQr() {
  return { qrDataUrl: "data:image/png;base64,ZmFrZQ==", message: "ready" };
}

export async function waitForWebLogin(options) {
  if (options.timeoutMs !== 90000) {
    throw new Error("expected a 90-second bridge window");
  }
  return { connected: true, message: "connected" };
}
`
	if err := os.WriteFile(modulePath, []byte(module), 0o600); err != nil {
		t.Fatalf("write fake OpenClaw login module: %v", err)
	}

	bridgePath := filepath.Join(dir, "bridge.mjs")
	if err := os.WriteFile(bridgePath, whatsappQrBridgeScript, 0o600); err != nil {
		t.Fatalf("write QR bridge: %v", err)
	}

	output, err := exec.Command(node, bridgePath, modulePath, "account-1").CombinedOutput()
	if err != nil {
		t.Fatalf("run QR bridge: %v\n%s", err, output)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) != 2 {
		t.Fatalf("bridge events = %d, want 2: %s", len(lines), output)
	}

	qrEvent, err := parseQrBridgeEvent([]byte(lines[0]))
	if err != nil {
		t.Fatalf("parse QR event: %v", err)
	}
	if qrEvent.QrDataURL != "data:image/png;base64,ZmFrZQ==" {
		t.Fatalf("QR data URL = %q", qrEvent.QrDataURL)
	}

	statusEvent, err := parseQrBridgeEvent([]byte(lines[1]))
	if err != nil {
		t.Fatalf("parse status event: %v", err)
	}
	if statusEvent.Type != "status" || !statusEvent.Connected {
		t.Fatalf("status event = %#v, want connected", statusEvent)
	}
}

func TestNormalizeWhatsAppTarget(t *testing.T) {
	for _, tt := range []struct {
		input string
		want  string
		valid bool
	}{
		{input: "+8613800000000", want: "+8613800000000", valid: true},
		{input: "8613800000000@s.whatsapp.net", want: "+8613800000000", valid: true},
		{input: "8613800000000@c.us", want: "+8613800000000", valid: true},
		{input: "unknown", valid: false},
		{input: "1", valid: false},
	} {
		got, err := normalizeWhatsAppTarget(tt.input)
		if tt.valid && (err != nil || got != tt.want) {
			t.Fatalf("normalizeWhatsAppTarget(%q) = %q, %v; want %q, nil", tt.input, got, err, tt.want)
		}
		if !tt.valid && err == nil {
			t.Fatalf("normalizeWhatsAppTarget(%q) succeeded", tt.input)
		}
	}
}
