package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// skillMarkdown is the agent-facing reference — the CLI replacement for the
// retired farol_inbox / farol_breakdown MCP prompts (cli-first migration
// Phase 2). It documents the command surface, the common working-loop
// workflow, and the gotchas an agent hits the first time it drives farol:
// the FAROL_AGENT identity contract, presence-vs-assignment, and the list
// ownership gate. It is generated prose, so it is emitted verbatim and never
// transformed by a render path.
//
// The constant is built from backtick raw strings (prose) concatenated with
// double-quoted fragments that hold inline-code backticks, because a backtick
// raw string cannot itself contain a backtick.
const skillMarkdown = `Farol - agent command reference

Farol is a to-do list with a CLI you can drive. The TUI is the human's live
dashboard; you (an agent) act through the CLI against the same SQLite store.
Changes from either side are visible to the other within ~1s (the TUI polls).

== Identity: FAROL_AGENT ==

Every write is attributed to an agent identity. It comes from the FAROL_AGENT
environment variable. If you do not set it, the CLI falls back to the shared
tag ` + "`agent`" + ` - so every unconfigured agent acts as one agent and overwrites
each other's work with no refusal. Export FAROL_AGENT once per session to get
a stable, unique tag:

    export FAROL_AGENT=pi

== Start of session ==

    farol inbox            # your list + every foreign list, top 20 pending tasks each
    farol inbox --json     # one JSON value: {mine, foreign_lists}

` + "`farol inbox`" + ` is the cheapest way to learn what is on your plate and what
is available to pick up. ` + "`farol work`" + ` shows who is actively on what right
now (live presence claims).

== Agent interaction protocol ==

The minimal loop for taking, tracking, and releasing work:

    export FAROL_AGENT=<unique-tag>   # once per session, before any farol command
    farol next <list-id> --json       # grab the top eligible task; assigns it to you and claims presence
    farol progress <id> --mode <mode> [--percent N]   # update progress as you work
    farol complete <id>               # mark complete when done (cascades; auto-unassigns)
    farol unassign <id>               # release the task when done
    farol work --json                 # see every live claim: who is on what right now

` + "`farol next`" + ` is the anti-race grab: it atomically picks the highest-priority
unassigned task, assigns it to you, and returns its full payload in one call.
An empty list is not an error - it returns ` + "`{ok: false, reason: \"no eligible task in this list\"}`" + `.
` + "`farol progress`" + ` takes --mode simple, percentage (with --percent 0-100), or
subtasks. ` + "`farol complete`" + ` marks a task complete (cascading to descendants and
auto-unassigning the cascade); ` + "`farol <id>`" + ` is a shorthand for the same
single-task action. ` + "`farol unassign`" + ` releases your assignment (or --list
<list-id> for a whole list); completing a task auto-unassigns it. ` + "`farol work`" + ` lists live
presence claims, not assignments - see "Presence vs. assignment" below.
` + "`farol agent help`" + ` prints this protocol; docs/AGENT_PROTOCOL.md carries the
full version.

== Working a task ==

    farol next <list-id>   # grab the top eligible task (highest priority, then tree order) and show it
    farol show <id>...     # show 1..50 tasks with full bodies, subtrees included
    farol tasks <list-id>  # the list as a tree (--status, --flat, --since, --include)

Assigning yourself (next or ` + "`farol assign`" + `) reserves the subtree: no other
agent can take an ancestor or descendant. That is how two agents avoid
researching the same work.

== Writes (status, progress, notes, comments, structure) ==

    farol <id>             # mark complete (cascades to descendants)
    farol complete <id>    # mark complete (cascades to descendants)
    farol toggle <id>      # complete <-> reopen, whichever applies
    farol reopen <id>      # back to pending (does not cascade)
    farol progress <id> --mode percentage --percent <0-100>
    farol notes <id> "<text>"        # replaces the whole notes field
    farol comment <id> "<text>"      # append a comment; prints its id
    farol add <list-id> "<title>" [--parent <id>] [--notes <text>]
    farol rename <id> "<title>"
    farol mv <id> [--parent <id>]    # re-parent; empty --parent moves to list root
    farol rm <id> --force            # delete a task and its descendants (--force required)
    farol priority <id> --level none|low|medium|high

Every write auto-claims presence under FAROL_AGENT, so the human's TUI
shows a spinner on what you are touching. You do not need to claim manually.
If you want an explicit, durable claim (e.g. you are inspecting but not
writing), use:

    farol claim <id> [--kind working|inspecting]   # light the spinner
    farol release <id>                              # clear it (no-op if you don't hold it)
    farol release --all                            # clear every claim you hold

== Presence vs. assignment (don't confuse them) ==

- Assignment (farol assign / next) means "this agent owns this work." It is
  the reservation that prevents two agents colliding.
- Presence (claim / the auto-claim on writes / farol work) is only a UI signal
  - a spinner in the TUI. It does NOT move a task to in_progress and it is NOT
  ownership. Reading either axis: farol show carries assignee and assignee_live;
  farol work lists live presence only.

== List ownership gate ==

Structural writes (add, rename, notes, mv, priority, rm) refuse to run on a
list owned by another agent unless you pass --force. Status/progress writes,
assignment, comments, and all reads are ungated - any agent may update those
cooperatively. An untagged list (created by a human, no owner) is foreign to
every agent: read + status/progress only.

== Change detection ==

    farol tasks <list-id> --since <unix-seconds>

Returns only rows whose updated_at changed since the timestamp (and widens the
default status filter to all). Deletions are not representable by a row filter
- diff id sets against your last read to detect them.

== JSON contract ==

With --json, stdout is exactly one JSON value - the payload on success or
{"error": ...} on failure - and nothing else. Parse stdout, then check the
exit code (0 ok, 1 domain failure like a missing id, 2 usage error). Human
mode prints tables/ids to stdout; errors go to stderr as "farol: ...".

== Gotchas ==

- Set FAROL_AGENT or you act as the shared tag "agent" - every unconfigured
  agent collides with you.
- farol rm, farol lists rm, and farol comment rm need --force - there is no
  confirm prompt in the CLI.
- An ambiguous id prefix (matches more than one row) is an error, never a
  silent pick of the first match. Copy a long-enough prefix.
- farol show with a single bad id is a hard failure (exit 1); with multiple
  ids, a bad id returns a per-row {id, error} and the rest still succeed.
`

func newSkillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "print the agent command reference (markdown)",
		Long: `Emit the agent-facing reference for driving farol from the CLI: the
FAROL_AGENT identity contract, the start-of-session reads, the write surface,
the presence-vs-assignment distinction, the list-ownership gate, and the --json
contract. This is the CLI replacement for the retired farol_inbox /
farol_breakdown MCP prompts (cli-first migration Phase 2).`,
		Args: cobra.NoArgs,
		RunE: runSkill,
	}
	return cmd
}

// runSkill prints the reference markdown. It is a doc command, not a store
// read: the markdown is static prose, so it goes straight to stdout in human
// mode and as a single {"skill": "..."} value in --json mode (one JSON value,
// per §9).
func runSkill(cmd *cobra.Command, args []string) error {
	errSilence(cmd)
	jsonMode, _ := cmd.Flags().GetBool("json")
	if !jsonMode {
		fmt.Fprint(os.Stdout, skillMarkdown)
		return nil
	}
	b, err := json.Marshal(struct {
		Skill string `json:"skill"`
	}{Skill: skillMarkdown})
	if err != nil {
		printError(jsonMode, err)
		return domainError(err)
	}
	fmt.Println(string(b))
	return nil
}
