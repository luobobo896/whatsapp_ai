package handler

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"whatsapp-ai-poc/internal/model"
)

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

func TestEnsureOpenClawAccountConfigPreservesExistingSettings(t *testing.T) {
	cfg := map[string]any{
		"channels": map[string]any{
			"whatsapp": map[string]any{
				"dmPolicy": "pairing",
			},
		},
	}

	if !ensureOpenClawAccountConfig(cfg, "wa_support") {
		t.Fatal("expected initial registration to change config")
	}
	if ensureOpenClawAccountConfig(cfg, "wa_support") {
		t.Fatal("duplicate registration changed config")
	}

	channels := cfg["channels"].(map[string]any)
	whatsApp := channels["whatsapp"].(map[string]any)
	if whatsApp["dmPolicy"] != "pairing" {
		t.Fatalf("dm policy = %v, want pairing", whatsApp["dmPolicy"])
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
	cfg := map[string]any{}
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
	sales := servers[openClawRAGMCPName("wa_sales")].(map[string]any)
	support := servers[openClawRAGMCPName("wa_support")].(map[string]any)
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
		allow := tools["allow"].([]string)
		if len(allow) != 1 || allow[0] != agent["id"].(string)+"__*" {
			t.Fatalf("agent tool policy = %#v, want only its own MCP tools", tools)
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

func TestSameOpenClawConfigIgnoresTrailingNewline(t *testing.T) {
	config := []byte("{\n  \"mcp\": {}\n}")
	if !sameOpenClawConfig(append(config, '\n'), config) {
		t.Fatal("trailing newline must not trigger an OpenClaw gateway restart")
	}
}

func TestWriteOpenClawRAGWorkspaceAddsPolicyToExistingWorkspace(t *testing.T) {
	workspace := t.TempDir()
	agentDir := filepath.Join(workspace, openClawRAGAgentID("wa_support"))
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
	if !strings.Contains(string(content), "# Existing instructions") || !strings.Contains(string(content), openClawRAGPolicyStart) || !strings.Contains(string(content), "call search_knowledge") {
		t.Fatalf("workspace policy = %q", content)
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

func TestAccountStatusSyncUsesIndependentCopy(t *testing.T) {
	responseAccounts := []model.Account{{
		ID: "account-1", AccountKey: "wa_support", Status: "connected",
	}}
	syncAccounts := accountsForLiveStatusSync(responseAccounts)
	applyLiveAccountStatuses(syncAccounts, map[string]channelConnectionStatus{
		"wa_support": {Known: true},
	})

	if responseAccounts[0].Status != "connected" {
		t.Fatalf("response status = %q, want connected", responseAccounts[0].Status)
	}
	if syncAccounts[0].Status != "pending" {
		t.Fatalf("sync status = %q, want pending", syncAccounts[0].Status)
	}
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
