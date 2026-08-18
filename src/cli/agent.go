package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// agentProtocolMarkdown is the agent interaction protocol — the minimal loop
// an agent follows to take, track, and release work without colliding with
// other agents: set FAROL_AGENT, grab with farol next, update with farol
// progress, release with farol unassign, and read the live claim set with
// farol work. It is the same prose docs/AGENT_PROTOCOL.md carries; farol
// skill is the full command reference this protocol is the working subset of.
// It is generated prose, so it is emitted verbatim and never transformed by a
// render path.
//
// The constant is built from backtick raw strings (prose) concatenated with
// double-quoted fragments that hold inline-code backticks, because a backtick
// raw string cannot itself contain a backtick.
const agentProtocolMarkdown = `Farol - agent interaction protocol

Farol is a to-do list with one store and two front ends: a TUI for the human
and a CLI for you (an agent). This is the minimal loop for taking, tracking,
and releasing work without colliding with other agents. ` + "`farol skill`" + ` is the
full command reference; this is the working subset.

== 1. Set your identity first ==

    export FAROL_AGENT=<unique-tag>

Set it before any farol command. The tag is your identity for both presence
(the TUI spinner, ` + "`farol work`" + `) and assignment (the task's assignee field).
Without it the CLI falls back to the shared tag "agent", so every
unconfigured agent acts as one agent and overwrites each other's work with no
refusal. Pick a tag unique to you and stable for the whole session.

== 2. Grab the top task ==

    farol next <list-id> --json

Atomically selects the top eligible task in the list (highest priority, then
tree order), assigns it to you, claims presence on it, and returns its full
payload in one call. Eligibility: not complete, unassigned, and no ancestor
or descendant assigned to another agent. An empty list is not an error - it
returns {"ok": false, "reason": "no eligible task in this list"}.

== 3. Update progress as you work ==

    farol progress <id> --mode <mode> [--percent N]

mode is one of:
- simple       in progress, no number
- percentage   requires --percent <0-100>
- subtasks     derived from completed children

== 4. Complete when done ==

    farol complete <id> [<id> ...]

Marking a task complete cascades to every descendant and auto-unassigns the
task and the whole cascade, so a completed task needs no explicit unassign.
` + "`farol <id>`" + ` is a shorthand for the same single-task action. Completing is
the explicit counterpart to setting progress: percentage at 100 does not
auto-complete, and only subtasks mode promotes a parent when all its children
are done.

== 5. Release when done ==

    farol unassign <id>

Clears your assignment on the task. ` + "`farol unassign --list <list-id>`" + `
releases every task in a list. Completing a task auto-unassigns it and every
descendant the cascade completes, so a completed task needs no explicit
unassign.

== 6. See who is working on what ==

    farol work --json

Lists every live presence claim (within the 120s TTL): who is working on
which task or list right now. --json emits a bare array of
{id, entity_type, entity_id, agent_id, kind, acquired_at}.

== Presence vs. assignment ==

Presence (claim / ` + "`farol work`" + `) is a UI signal - the TUI spinner - and expires
after 120s of inactivity. Assignment (` + "`farol next`" + ` / ` + "`farol assign`" + ` / ` + "`farol unassign`" + `)
is durable ownership with no TTL. They are separate axes: claiming a task
does not move it to in_progress, and assigning does not claim presence.
` + "`farol skill`" + ` is the full reference for the rest of the surface.
`

// newAgentCmd is the agent-protocol group: `farol agent` prints the protocol
// directly (the group's one action), and `farol agent help` spells it out as
// an explicit subcommand. It is a doc command like `farol skill` — static
// prose, no store read — so it never goes through runStore.
func newAgentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "agent interaction protocol: how an agent takes, tracks, and releases work",
		Long: `The agent-facing working loop: set FAROL_AGENT, grab the top task with
farol next, update progress with farol progress, release with farol unassign,
and read the live claim set with farol work. ` + "`farol agent help`" + ` prints the
protocol; ` + "`farol skill`" + ` is the full command reference.`,
		Args: cobra.NoArgs,
		RunE: runAgentHelp,
	}
	cmd.AddCommand(newAgentHelpCmd())
	return cmd
}

// newAgentHelpCmd is `farol agent help`, the explicit form of the group's one
// action: print the agent interaction protocol.
func newAgentHelpCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "help",
		Short: "print the agent interaction protocol (markdown)",
		Long: `Emit the agent interaction protocol: the minimal loop an agent follows to
take, track, and release work without colliding with other agents — set
FAROL_AGENT, grab with farol next, update with farol progress, release with
farol unassign, and read the live claim set with farol work.`,
		Args: cobra.NoArgs,
		RunE: runAgentHelp,
	}
	return cmd
}

// runAgentHelp prints the protocol markdown. It is a doc command, not a store
// read: the markdown is static prose, so it goes straight to stdout in human
// mode and as a single {"help": "..."} value in --json mode (one JSON value,
// per §9 — the same shape farol skill's {"skill": "..."} wraps its prose in).
func runAgentHelp(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	if !jsonMode {
		fmt.Fprint(os.Stdout, agentProtocolMarkdown)
		return nil
	}
	b, err := json.Marshal(struct {
		Help string `json:"help"`
	}{Help: agentProtocolMarkdown})
	if err != nil {
		printError(jsonMode, err)
		return domainError(err)
	}
	fmt.Println(string(b))
	return nil
}
