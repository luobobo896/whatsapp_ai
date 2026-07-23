// Package port manages dynamic port allocation for OpenClaw instances.
package port

import (
	"fmt"
	"net"
	"sync"
)

const (
	// Default gateway HTTP port range
	minGatewayPort = 19000
	maxGatewayPort = 29999

	// Default gateway WebSocket port range
	minGatewayWSPort = 30000
	maxGatewayWSPort = 39999
)

// Allocator manages dynamic port allocation.
type Allocator struct {
	mu             sync.Mutex
	usedHTTPPorts  map[int]struct{}
	usedWSPorts    map[int]struct{}
	nextHTTPPort   int
	nextWSPort     int
}

// New creates a new port allocator.
func New() *Allocator {
	return &Allocator{
		usedHTTPPorts: make(map[int]struct{}),
		usedWSPorts:   make(map[int]struct{}),
		nextHTTPPort:  minGatewayPort,
		nextWSPort:   minGatewayWSPort,
	}
}

// global allocator instance
var global = New()

// AllocateHTTP allocates a new HTTP port for an OpenClaw gateway.
func AllocateHTTP() (int, error) {
	return global.allocateHTTP()
}

// AllocateWS allocates a new WebSocket port for an OpenClaw gateway.
func AllocateWS() (int, error) {
	return global.allocateWS()
}

// ReleaseHTTP marks an HTTP port as available for reuse.
func ReleaseHTTP(port int) {
	global.releaseHTTP(port)
}

// ReleaseWS marks a WebSocket port as available for reuse.
func ReleaseWS(port int) {
	global.releaseWS(port)
}

// allocateHTTP allocates a new HTTP port.
func (a *Allocator) allocateHTTP() (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Try to find an available port in the used range
	for i := 0; i < (maxGatewayPort - minGatewayPort); i++ {
		port := a.nextHTTPPort
		a.nextHTTPPort++
		if a.nextHTTPPort > maxGatewayPort {
			a.nextHTTPPort = minGatewayPort
		}

		if _, ok := a.usedHTTPPorts[port]; !ok {
			if a.isPortAvailable(port) {
				a.usedHTTPPorts[port] = struct{}{}
				return port, nil
			}
		}
	}

	return 0, fmt.Errorf("no available HTTP ports in range %d-%d", minGatewayPort, maxGatewayPort)
}

// allocateWS allocates a new WebSocket port.
func (a *Allocator) allocateWS() (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Try to find an available port in the used range
	for i := 0; i < (maxGatewayWSPort - minGatewayWSPort); i++ {
		port := a.nextWSPort
		a.nextWSPort++
		if a.nextWSPort > maxGatewayWSPort {
			a.nextWSPort = minGatewayWSPort
		}

		if _, ok := a.usedWSPorts[port]; !ok {
			if a.isPortAvailable(port) {
				a.usedWSPorts[port] = struct{}{}
				return port, nil
			}
		}
	}

	return 0, fmt.Errorf("no available WebSocket ports in range %d-%d", minGatewayWSPort, maxGatewayWSPort)
}

// releaseHTTP marks an HTTP port as available.
func (a *Allocator) releaseHTTP(port int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.usedHTTPPorts, port)
}

// releaseWS marks a WebSocket port as available.
func (a *Allocator) releaseWS(port int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.usedWSPorts, port)
}

// isPortAvailable checks if a port is actually available (not in use by OS).
func (a *Allocator) isPortAvailable(port int) bool {
	addr := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// MarkHTTPUsed marks an HTTP port as used (for restoring state).
func MarkHTTPUsed(port int) {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.usedHTTPPorts[port] = struct{}{}
}

// MarkWSUsed marks a WebSocket port as used (for restoring state).
func MarkWSUsed(port int) {
	global.mu.Lock()
	defer global.mu.Unlock()
	global.usedWSPorts[port] = struct{}{}
}
