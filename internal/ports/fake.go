package ports

import (
	"context"
	"fmt"
)

// FakePortScanner is a test double for PortScanner.
// OccupiedPorts lists ports that should appear as NOT available.
// All other ports are considered free.
type FakePortScanner struct {
	OccupiedPorts    []int
	OccupiedTCPPorts []int
	OccupiedUDPPorts []int
}

func isOccupied(port int, occupied ...[]int) bool {
	for _, ports := range occupied {
		for _, p := range ports {
			if p == port {
				return true
			}
		}
	}
	return false
}

// IsAvailable returns false if port is in OccupiedPorts, true otherwise.
func (f *FakePortScanner) IsAvailable(_ context.Context, port int) bool {
	return !isOccupied(port, f.OccupiedPorts, f.OccupiedTCPPorts)
}

// IsUDPAvailable follows the same logic as IsAvailable.
func (f *FakePortScanner) IsUDPAvailable(_ context.Context, port int) bool {
	return !isOccupied(port, f.OccupiedPorts, f.OccupiedUDPPorts)
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
