package main

import "testing"

// No flags is the ordinary run: parseFlags succeeds and does not report
// --version. Phase 2 (docs/plans/phase-2-cli.md) replaces this file's flag
// handling with Cobra; this test only pins the phase-0 placeholder.
func TestParseFlagsWithoutFlags(t *testing.T) {
	if err := parseFlags(nil); err != nil {
		t.Fatalf("parseFlags(nil): %v", err)
	}
}

func TestParseFlagsAcceptsBothVersionSpellings(t *testing.T) {
	for _, args := range [][]string{{"--version"}, {"-v"}} {
		if err := parseFlags(args); err != errVersionRequested {
			t.Errorf("parseFlags(%v): got %v, want errVersionRequested", args, err)
		}
	}
}

func TestParseFlagsRejectsUnknownFlag(t *testing.T) {
	if err := parseFlags([]string{"--not-a-real-flag"}); err == nil {
		t.Error("expected an unknown flag to be refused")
	}
}
