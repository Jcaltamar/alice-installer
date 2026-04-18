package secrets_test

import (
	"encoding/base64"
	"testing"

	"github.com/jcaltamar/alice-installer/internal/secrets"
)

func TestCryptoRandGenerator_Generate(t *testing.T) {
	t.Run("produces base64 of exact byte length", func(t *testing.T) {
		tests := []struct {
			name    string
			byteLen int
		}{
			{"16 bytes", 16},
			{"32 bytes", 32},
			{"64 bytes", 64},
		}

		gen := secrets.CryptoRandGenerator{}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				out, err := gen.Generate(tt.byteLen)
				if err != nil {
					t.Fatalf("Generate(%d) unexpected error: %v", tt.byteLen, err)
				}

				decoded, decErr := base64.StdEncoding.DecodeString(out)
				if decErr != nil {
					t.Fatalf("output is not valid base64: %v", decErr)
				}

				if len(decoded) != tt.byteLen {
					t.Errorf("decoded len = %d, want %d", len(decoded), tt.byteLen)
				}
			})
		}
	})

	t.Run("consecutive calls produce different values (entropy smoke test)", func(t *testing.T) {
		gen := secrets.CryptoRandGenerator{}

		first, err := gen.Generate(32)
		if err != nil {
			t.Fatalf("first Generate error: %v", err)
		}

		second, err := gen.Generate(32)
		if err != nil {
			t.Fatalf("second Generate error: %v", err)
		}

		if first == second {
			t.Error("two consecutive 32-byte generates returned identical values — crypto/rand not working")
		}
	})

	t.Run("zero byte length returns empty string", func(t *testing.T) {
		gen := secrets.CryptoRandGenerator{}

		out, err := gen.Generate(0)
		if err != nil {
			t.Fatalf("Generate(0) unexpected error: %v", err)
		}

		// base64 of zero bytes is the empty string
		if out != "" {
			t.Errorf("Generate(0) = %q, want empty string", out)
		}
	})
}

func TestFakeGenerator(t *testing.T) {
	t.Run("returns configured value and no error", func(t *testing.T) {
		fake := secrets.FakeGenerator{Val: "secret-value"}

		out, err := fake.Generate(32)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if out != "secret-value" {
			t.Errorf("got %q, want %q", out, "secret-value")
		}
	})

	t.Run("returns configured error", func(t *testing.T) {
		fake := secrets.FakeGenerator{Err: errFake}

		_, err := fake.Generate(32)
		if err != errFake {
			t.Errorf("got %v, want %v", err, errFake)
		}
	})
}

// sentinel error for fake tests
var errFake = fakeError("fake generate error")

type fakeError string

func (e fakeError) Error() string { return string(e) }
