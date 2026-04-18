package secrets

// FakeGenerator is a test double for PasswordGenerator.
// Set Val to control the returned string and Err to simulate errors.
type FakeGenerator struct {
	Val string
	Err error
}

// Generate returns the configured Val and Err, ignoring byteLen.
func (f FakeGenerator) Generate(_ int) (string, error) {
	return f.Val, f.Err
}
