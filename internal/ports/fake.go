package ports

import (
	"context"
	"fmt"
)

// FakePortScanner is a test double for PortScanner.
// OccupiedPorts lists ports that should appear as NOT available.
// All other ports are considered free.
type FakePortScanner struct {
	OccupiedPorts []int
}

func (f *FakePortScanner) isOccupied(port int) bool {
	for _, p := range f.OccupiedPorts {
		if p == port {
			return true
		}
	}
	return false
}

// IsAvailable returns false if port is in OccupiedPorts, true otherwise.
func (f *FakePortScanner) IsAvailable(_ context.Context, port int) bool {
	return !f.isOccupied(port)
}

// IsUDPAvailable follows the same logic as IsAvailable.
func (f *FakePortScanner) IsUDPAvailable(_ context.Context, port int) bool {
	return !f.isOccupied(port)
}

// FirstAvailable iterates from start until it finds a free port or exhausts
// a 100-port window.
func (f *FakePortScanner) FirstAvailable(ctx context.Context, start int) (int, error) {
	for i := 0; i < 100; i++ {
		candidate := start + i
		if f.IsAvailable(ctx, candidate) {
			return candidate, nil
		}
	}
	return 0, fmt.Errorf("no free port found in range [%d, %d)", start, start+100)
}
