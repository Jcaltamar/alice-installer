package ports

import (
	"context"
	"fmt"
	"net"
)

const maxSearchWindow = 100

// PortScanner checks port availability and finds free ports.
type PortScanner interface {
	// IsAvailable returns true iff the TCP port is not currently in use.
	IsAvailable(ctx context.Context, port int) bool

	// IsUDPAvailable returns true iff the UDP port is not currently in use.
	IsUDPAvailable(ctx context.Context, port int) bool

	// FirstAvailable returns start if it is free, otherwise iterates
	// start+1, start+2, … up to start+maxSearchWindow.
	// Returns an error if no free port is found in the window.
	FirstAvailable(ctx context.Context, start int) (int, error)
}

// NetPortScanner uses net.Listen / net.ListenPacket for port probing.
type NetPortScanner struct{}

// NewNetPortScanner creates a production PortScanner.
func NewNetPortScanner() *NetPortScanner {
	return &NetPortScanner{}
}

// IsAvailable returns true iff the TCP port can be bound (i.e. is free).
func (s *NetPortScanner) IsAvailable(_ context.Context, port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = l.Close()
	return true
}

// IsUDPAvailable returns true iff the UDP port can be bound.
func (s *NetPortScanner) IsUDPAvailable(_ context.Context, port int) bool {
	pc, err := net.ListenPacket("udp", fmt.Sprintf(":%d", port))
	if err != nil {
		return false
	}
	_ = pc.Close()
	return true
}

// FirstAvailable returns the first free TCP port starting from start.
// It searches up to maxSearchWindow ports. Returns an error if none found.
func (s *NetPortScanner) FirstAvailable(ctx context.Context, start int) (int, error) {
	for i := 0; i < maxSearchWindow; i++ {
		candidate := start + i
		if s.IsAvailable(ctx, candidate) {
			return candidate, nil
		}
	}
	return 0, fmt.Errorf("no free TCP port found in range [%d, %d)", start, start+maxSearchWindow)
}
