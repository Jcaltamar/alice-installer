package ports_test

import (
	"context"
	"net"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/ports"
)

// bindTCPPort opens a TCP listener on a random port and returns it + port number.
// Caller must close the listener when done (t.Cleanup).
func bindTCPPort(t *testing.T) (net.Listener, int) {
	t.Helper()
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("bindTCPPort: could not bind: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	t.Cleanup(func() { _ = l.Close() })
	return l, port
}

// bindUDPPort opens a UDP listener on a random port and returns it + port number.
func bindUDPPort(t *testing.T) (net.PacketConn, int) {
	t.Helper()
	pc, err := net.ListenPacket("udp", ":0")
	if err != nil {
		t.Fatalf("bindUDPPort: could not bind: %v", err)
	}
	port := pc.LocalAddr().(*net.UDPAddr).Port
	t.Cleanup(func() { _ = pc.Close() })
	return pc, port
}

func TestNetPortScanner_IsAvailableTCP_Free(t *testing.T) {
	// Find a port that is free by binding then immediately releasing
	l, port := bindTCPPort(t)
	l.Close() // release so scanner can see it as free

	s := ports.NewNetPortScanner()
	if !s.IsAvailable(context.Background(), port) {
		t.Errorf("IsAvailable(%d) = false, want true (port was released)", port)
	}
}

func TestNetPortScanner_IsAvailableTCP_Occupied(t *testing.T) {
	// Bind a port and keep it held — scanner must see it as occupied
	_, port := bindTCPPort(t) // listener remains open via t.Cleanup

	s := ports.NewNetPortScanner()
	if s.IsAvailable(context.Background(), port) {
		t.Errorf("IsAvailable(%d) = true, want false (port still bound)", port)
	}
}

func TestNetPortScanner_IsUDPAvailable_Free(t *testing.T) {
	pc, port := bindUDPPort(t)
	pc.Close() // release

	s := ports.NewNetPortScanner()
	if !s.IsUDPAvailable(context.Background(), port) {
		t.Errorf("IsUDPAvailable(%d) = false, want true (port was released)", port)
	}
}

func TestNetPortScanner_IsUDPAvailable_Occupied(t *testing.T) {
	_, port := bindUDPPort(t) // listener remains open

	s := ports.NewNetPortScanner()
	if s.IsUDPAvailable(context.Background(), port) {
		t.Errorf("IsUDPAvailable(%d) = true, want false (port still bound)", port)
	}
}

func TestNetPortScanner_FirstAvailable_FreePort(t *testing.T) {
	// Pick a free port as starting point; FirstAvailable should return it immediately.
	l, start := bindTCPPort(t)
	l.Close()

	s := ports.NewNetPortScanner()
	got, err := s.FirstAvailable(context.Background(), start)
	if err != nil {
		t.Fatalf("FirstAvailable(%d) unexpected error: %v", start, err)
	}
	// Should return start itself or a port >= start within the search window
	if got < start {
		t.Errorf("FirstAvailable returned %d, want >= %d", got, start)
	}
}

func TestNetPortScanner_FirstAvailable_SkipsOccupied(t *testing.T) {
	// Bind the start port so FirstAvailable must skip it.
	_, start := bindTCPPort(t) // held open

	s := ports.NewNetPortScanner()
	got, err := s.FirstAvailable(context.Background(), start)
	if err != nil {
		t.Fatalf("FirstAvailable(%d) unexpected error: %v", start, err)
	}
	if got <= start {
		t.Errorf("FirstAvailable returned %d, want > %d (start is occupied)", got, start)
	}
}

func TestPortScanner_Interface(t *testing.T) {
	var _ ports.PortScanner = ports.NewNetPortScanner()
	t.Log("interface satisfied")
}
