// Package health provides periodic health checking for OpenClaw instances.
package health

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Checker runs periodic health checks for all OpenClaw instances.
type Checker struct {
	mu       sync.Mutex
	store    Store
	stopCh   chan struct{}
	stopped  bool
	interval time.Duration
}

// Store is the interface for accessing account data.
type Store interface {
	AccountsNeedingHealthCheck(limit int) ([]AccountRef, error)
	UpdateAccountHealthCheck(accountID string) error
}

// AccountRef is a reference to an account for health checking.
type AccountRef struct {
	ID           string
	TenantID     string
	InstancePID  int
	ProxyURL     string
	GatewayPort  int
}

// New creates a new health checker.
func New(store Store, interval time.Duration) *Checker {
	if interval == 0 {
		interval = 30 * time.Second
	}
	return &Checker{
		store:    store,
		stopCh:   make(chan struct{}),
		interval: interval,
	}
}

// Start begins the health check loop.
func (c *Checker) Start(ctx context.Context) {
	slog.Info("Health checker started", "interval", c.interval)

	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Health checker stopped")
			return
		case <-c.stopCh:
			slog.Info("Health checker stopped")
			return
		case <-ticker.C:
			c.runCheck(ctx)
		}
	}
}

// Stop stops the health checker.
func (c *Checker) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.stopped {
		return
	}

	c.stopped = true
	close(c.stopCh)
}

// runCheck executes a single health check cycle.
func (c *Checker) runCheck(ctx context.Context) {
	// Get accounts that need health checking (running instances)
	// For now, we'll check a batch at a time
	const batchSize = 50

	accounts, err := c.store.AccountsNeedingHealthCheck(batchSize)
	if err != nil {
		slog.Error("Failed to get accounts for health check", "error", err)
		return
	}

	if len(accounts) == 0 {
		return
	}

	slog.Debug("Running health check", "accounts", len(accounts))

	var wg sync.WaitGroup
	for _, account := range accounts {
		wg.Add(1)
		go func(a AccountRef) {
			defer wg.Done()
			c.checkAccount(ctx, a)
		}(account)
	}

	wg.Wait()
}

// checkAccount checks the health of a single account's OpenClaw instance.
func (c *Checker) checkAccount(ctx context.Context, account AccountRef) {
	// Check if process is running
	if !c.isProcessRunning(account.InstancePID) {
		slog.Warn("Instance process not running",
			"accountID", account.ID,
			"pid", account.InstancePID,
		)
		// TODO: Trigger restart via instance manager
		return
	}

	// Check HTTP endpoint
	if !c.isHTTPHealthy(ctx, account.GatewayPort) {
		slog.Warn("Instance HTTP endpoint unhealthy",
			"accountID", account.ID,
			"port", account.GatewayPort,
		)
		// TODO: Trigger restart via instance manager
		return
	}

	// Update last health check time
	if err := c.store.UpdateAccountHealthCheck(account.ID); err != nil {
		slog.Error("Failed to update health check time",
			"accountID", account.ID,
			"error", err,
		)
	}
}

// isProcessRunning checks if a process with the given PID is running.
func (c *Checker) isProcessRunning(pid int) bool {
	if pid == 0 {
		return false
	}
	// Send signal 0 to check if process exists
	process, err := findProcess(pid)
	if err != nil {
		return false
	}
	return processRunning(process)
}

// isHTTPHealthy checks if the HTTP endpoint is responding.
func (c *Checker) isHTTPHealthy(ctx context.Context, port int) bool {
	if port == 0 {
		return false
	}

	// Simple HTTP GET to the health endpoint
	// TODO: Implement actual HTTP check
	return true
}

// The following are placeholder implementations
// In a real implementation, you would use os.FindProcess and similar

func findProcess(pid int) (interface{}, error) {
	return nil, nil
}

func processRunning(p interface{}) bool {
	return true
}
