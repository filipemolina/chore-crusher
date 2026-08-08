// Internal test: serverIdentity, createdByRE and ownerTagPattern are
// unexported, and the tag-generation contract is exactly what needs pinning
// (docs/plan/session-scoped-agent-identity.md decision 1). The rest of the
// package is exercised from mcpserver_test through the MCP surface.
package mcpserver

import (
	"strings"
	"testing"
)

// TestServerIdentityIsUniquePerProcess pins docs/plan/session-scoped-agent-identity.md
// decision 1: CRUSH_AGENT wins verbatim, and when it is unset the tag is
// unique per resolution rather than the constant "agent" it used to be. The
// constant is what made two unconfigured clients compare equal and write over
// each other; see TestTwoUnconfiguredSessionsCannotWriteEachOthersTasks.
//
// The generated tag must satisfy ownerTagPattern because it is written to
// list.created_by, where add_list validates it — a tag that failed the pattern
// would make my_list unusable on a default setup.
func TestServerIdentityIsUniquePerProcess(t *testing.T) {
	t.Setenv("CRUSH_AGENT", "")

	first, second := serverIdentity(), serverIdentity()
	if first == second {
		t.Fatalf("serverIdentity returned %q twice with CRUSH_AGENT unset; "+
			"two concurrent sessions would share a tag and every guard would compare equal", first)
	}
	for _, got := range []string{first, second} {
		if !createdByRE.MatchString(got) {
			t.Fatalf("generated identity %q does not match %s, so add_list would refuse it",
				got, ownerTagPattern)
		}
		if !strings.HasPrefix(got, "agent-") {
			t.Fatalf("generated identity %q should be recognisable as an auto-assigned tag", got)
		}
	}

	// Set, it is used exactly as given — a human who wants a stable tag keeps one.
	t.Setenv("CRUSH_AGENT", "pi")
	if got := serverIdentity(); got != "pi" {
		t.Fatalf("serverIdentity with CRUSH_AGENT=pi = %q, want pi", got)
	}
}
