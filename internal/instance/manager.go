// Package instance manages OpenClaw instance lifecycle for each WhatsApp account.
package instance

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
	"whatsapp-ai-poc/internal/model"
	"whatsapp-ai-poc/internal/port"
)

const (
	maxRestartCount = 5
	restartBackoff   = 30 * time.Second
	maxBackoff       = 5 * time.Minute
)

// Manager manages OpenClaw instances for all accounts.
type Manager struct {
	mu        sync.RWMutex
	instances map[string]*Instance // accountID -> Instance
	store     Store
}

// Store is the interface for accessing account data.
type Store interface {
	AccountByID(tenantID, accountID string) (*model.AccountRow, error)
	AccountsForRestore() ([]AccountRowRef, error)
	UpdateAccountInstance(tenantID, accountID string, gatewayPort, gatewayWSPort, instancePID int, instanceStatus, configPath string) (*model.AccountRow, error)
	UpdateAccountHealthCheck(accountID string) error
	IncrementAccountRestartCount(accountID string) error
}

// AccountRowRef is a lightweight reference to an account for restore operations.
type AccountRowRef struct {
	ID       string
	TenantID string
	ProxyURL string
	ConfigPath string
}

// New creates a new instance manager.
func New(store Store) *Manager {
	return &Manager{
		instances: make(map[string]*Instance),
		store:     store,
	}
}

// Instance represents a running OpenClaw instance.
type Instance struct {
	mu              sync.Mutex
	accountID       string
	tenantID        string
	gatewayPort     int
	gatewayWSPort   int
	configPath      string
	proxyURL        string
	cmd             *exec.Cmd
	pid             int
	status          string
	restartCount    int
	lastRestartTime time.Time
	cancel          context.CancelFunc
	stopped         chan struct{}
}

// Start starts an OpenClaw instance for the given account.
func (m *Manager) Start(ctx context.Context, tenantID, accountID, proxyURL string) (*Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if instance already exists
	if existing, ok := m.instances[accountID]; ok {
		existing.mu.Lock()
		if existing.status == "running" {
			existing.mu.Unlock()
			return existing, nil
		}
		existing.mu.Unlock()
	}

	// Allocate ports
	gatewayPort, err := port.AllocateHTTP()
	if err != nil {
		return nil, fmt.Errorf("allocate gateway port: %w", err)
	}
	gatewayWSPort, err := port.AllocateWS()
	if err != nil {
		port.ReleaseHTTP(gatewayPort)
		return nil, fmt.Errorf("allocate gateway WS port: %w", err)
	}

	// Create instance directory
	home, err := os.UserHomeDir()
	if err != nil {
		port.ReleaseHTTP(gatewayPort)
		port.ReleaseWS(gatewayWSPort)
		return nil, fmt.Errorf("get home dir: %w", err)
	}

	instanceDir := filepath.Join(home, ".openclaw", "instances", accountID)
	if err := os.MkdirAll(instanceDir, 0o700); err != nil {
		port.ReleaseHTTP(gatewayPort)
		port.ReleaseWS(gatewayWSPort)
		return nil, fmt.Errorf("create instance dir: %w", err)
	}

	configPath := filepath.Join(instanceDir, "openclaw.json")

	// Create instance
	inst := &Instance{
		accountID:     accountID,
		tenantID:      tenantID,
		gatewayPort:   gatewayPort,
		gatewayWSPort: gatewayWSPort,
		configPath:    configPath,
		proxyURL:     proxyURL,
		status:        "starting",
		stopped:       make(chan struct{}),
	}

	// Start the OpenClaw process
	if err := inst.startProcess(ctx); err != nil {
		port.ReleaseHTTP(gatewayPort)
		port.ReleaseWS(gatewayWSPort)
		return nil, fmt.Errorf("start process: %w", err)
	}

	// Update database
	if _, err := m.store.UpdateAccountInstance(tenantID, accountID, gatewayPort, gatewayWSPort, inst.pid, "running", configPath); err != nil {
		slog.Error("Failed to update account instance", "accountID", accountID, "error", err)
	}

	m.instances[accountID] = inst

	// Start health checker
	go inst.healthCheck(m.store)

	return inst, nil
}

// Stop stops an OpenClaw instance.
func (m *Manager) Stop(accountID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	inst, ok := m.instances[accountID]
	if !ok {
		return fmt.Errorf("instance not found: %s", accountID)
	}

	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.cmd == nil || inst.cmd.Process == nil {
		return nil
	}

	if err := inst.cmd.Process.Signal(os.Interrupt); err != nil {
		// If SIGINT fails, try SIGKILL
		slog.Warn("SIGINT failed, trying SIGKILL", "pid", inst.pid, "error", err)
		if err := inst.cmd.Process.Kill(); err != nil {
			slog.Error("Failed to kill process", "pid", inst.pid, "error", err)
		}
	}

	inst.status = "stopped"
	close(inst.stopped)

	// Release ports
	port.ReleaseHTTP(inst.gatewayPort)
	port.ReleaseWS(inst.gatewayWSPort)

	delete(m.instances, accountID)

	return nil
}

// Restart restarts an OpenClaw instance.
func (m *Manager) Restart(ctx context.Context, accountID string) error {
	m.mu.Lock()
	inst, ok := m.instances[accountID]
	m.mu.Unlock()

	if !ok {
		return fmt.Errorf("instance not found: %s", accountID)
	}

	tenantID := inst.tenantID
	proxyURL := inst.proxyURL

	// Stop the instance
	if err := m.Stop(accountID); err != nil {
		slog.Error("Failed to stop instance for restart", "accountID", accountID, "error", err)
	}

	// Increment restart count
	if err := m.store.IncrementAccountRestartCount(accountID); err != nil {
		slog.Error("Failed to increment restart count", "accountID", accountID, "error", err)
	}

	// Start again
	_, err := m.Start(ctx, tenantID, accountID, proxyURL)
	return err
}

// Get returns an instance by account ID.
func (m *Manager) Get(accountID string) (*Instance, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inst, ok := m.instances[accountID]
	return inst, ok
}

// Restore restores instances from the database (called on startup).
// Loads all accounts with instance_status='running' and restarts their instances.
func (m *Manager) Restore(ctx context.Context) error {
	// Get all accounts that need to be restored
	accounts, err := m.store.AccountsForRestore()
	if err != nil {
		return fmt.Errorf("failed to get accounts for restore: %w", err)
	}

	var restoreErrors []error
	for _, account := range accounts {
		// Skip if instance is already running (maybe manually started)
		if _, exists := m.Get(account.ID); exists {
			continue
		}

		slog.Info("Restoring OpenClaw instance",
			"accountID", account.ID,
			"tenantID", account.TenantID,
			"proxyURL", maskProxyURL(account.ProxyURL))

		// Start the instance
		if _, err := m.Start(ctx, account.TenantID, account.ID, account.ProxyURL); err != nil {
			slog.Error("Failed to restore instance",
				"accountID", account.ID,
				"error", err)
			restoreErrors = append(restoreErrors, err)
		}
	}

	if len(restoreErrors) > 0 {
		return fmt.Errorf("failed to restore %d instances: %w", len(restoreErrors), errors.Join(restoreErrors...))
	}

	slog.Info("Instance restore completed", "total", len(accounts))
	return nil
}

// startProcess starts the OpenClaw process for this instance.
func (inst *Instance) startProcess(ctx context.Context) error {
	// Create context for cancellation
	cmdCtx, cancel := context.WithCancel(ctx)
	inst.cancel = cancel

	// Build OpenClaw command with custom config
	args := []string{
		"--config", inst.configPath,
		"gateway",
		"run",
		"--gateway-http-port", fmt.Sprintf("%d", inst.gatewayPort),
		"--gateway-ws-port", fmt.Sprintf("%d", inst.gatewayWSPort),
	}

	cmd := exec.CommandContext(cmdCtx, "openclaw", args...)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("OPENCLAW_INSTANCE_ID=%s", inst.accountID),
	)

	// Set proxy if configured
	if inst.proxyURL != "" {
		cmd.Env = append(cmd.Env, fmt.Sprintf("OPENCLAW_PROXY_URL=%s", inst.proxyURL))
	}

	// Redirect output to log files
	logDir := filepath.Dir(inst.configPath)
	logFile := filepath.Join(logDir, "gateway.log")
	logW, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	cmd.Stdout = logW
	cmd.Stderr = logW

	// Start process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start command: %w", err)
	}

	inst.cmd = cmd
	inst.pid = cmd.Process.Pid
	inst.status = "running"

	slog.Info("OpenClaw instance started",
		"accountID", inst.accountID,
		"pid", inst.pid,
		"httpPort", inst.gatewayPort,
		"wsPort", inst.gatewayWSPort,
		"proxy", maskProxyURL(inst.proxyURL),
	)

	return nil
}

// healthCheck runs periodic health checks for the instance.
func (inst *Instance) healthCheck(store Store) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-inst.stopped:
			return
		case <-ticker.C:
			if inst.isHealthy() {
				inst.mu.Lock()
				inst.restartCount = 0
				inst.mu.Unlock()
				if err := store.UpdateAccountHealthCheck(inst.accountID); err != nil {
					slog.Error("Failed to update health check", "accountID", inst.accountID, "error", err)
				}
			} else {
				inst.handleFailure(store)
			}
		}
	}
}

// isHealthy checks if the OpenClaw instance is healthy.
func (inst *Instance) isHealthy() bool {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.cmd == nil || inst.cmd.Process == nil {
		return false
	}

	// Check if process is still running
	if err := inst.cmd.Process.Signal(os.Interrupt); err != nil {
		return false
	}

	// TODO: Add HTTP health check to gateway endpoint
	return true
}

// handleFailure handles instance failure with exponential backoff.
func (inst *Instance) handleFailure(store Store) {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	inst.restartCount++

	if inst.restartCount > maxRestartCount {
		slog.Error("Instance restart count exceeded, giving up",
			"accountID", inst.accountID,
			"restartCount", inst.restartCount,
		)
		inst.status = "error"
		close(inst.stopped)
		return
	}

	// Calculate backoff duration
	backoff := time.Duration(inst.restartCount) * restartBackoff
	if backoff > maxBackoff {
		backoff = maxBackoff
	}

	slog.Warn("Instance unhealthy, will restart after backoff",
		"accountID", inst.accountID,
		"restartCount", inst.restartCount,
		"backoff", backoff,
	)

	// Wait for backoff
	select {
	case <-inst.stopped:
		return
	case <-time.After(backoff):
	}

	// Restart process
	ctx := context.Background()
	if err := inst.startProcess(ctx); err != nil {
		slog.Error("Failed to restart instance", "accountID", inst.accountID, "error", err)
		inst.status = "error"
		return
	}

	slog.Info("Instance restarted successfully",
		"accountID", inst.accountID,
		"restartCount", inst.restartCount,
	)
}

// Status returns the current status of the instance.
func (inst *Instance) Status() string {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	return inst.status
}

// maskProxyURL masks the password in a proxy URL for logging.
func maskProxyURL(url string) string {
	if url == "" {
		return "none"
	}
	// Simple mask: show protocol and host, hide password
	// http://user:pass@host:port -> http://user:***@host:port
	// This is a basic implementation - for production use proper URL parsing
	return "configured"
}

// GenerateInstanceID generates a unique instance ID.
func GenerateInstanceID() string {
	return uuid.New().String()
}
