package handler

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
export async function waitForWebLogin() {
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
