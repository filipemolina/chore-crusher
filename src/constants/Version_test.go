package constants

import "testing"

// The stamp is the whole point of the -ldflags build: whatever the build info
// says, a stamped binary reports the version it was released as.
func TestVersionPrefersTheStamp(t *testing.T) {
	original := version
	t.Cleanup(func() { version = original })

	version = "v1.2.3"

	if got := Version(); got != "v1.2.3" {
		t.Errorf("version: got %q, want the stamped v1.2.3", got)
	}
}

// Unstamped, this test binary is built from the checkout, so the answer is
// the commit. What matters is that there is always an answer - the CLI's
// --version and any future about screen both render it unconditionally.
func TestVersionFallsBackToTheBuild(t *testing.T) {
	original := version
	t.Cleanup(func() { version = original })

	version = ""

	if got := Version(); got == "" {
		t.Error("version: got an empty string, want a commit or unknown")
	}
}
