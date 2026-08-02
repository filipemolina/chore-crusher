package cli

import (
	"github.com/spf13/cobra"

	"github.com/filipemolina/chore-crusher/src/mcpserver"
)

// newMcpCmd returns the `crush mcp` subcommand that runs the Model Context
// Protocol server over stdin/stdout. Clients connect to it and call tools
// instead of exec'ing the CLI for every operation (docs/DESIGN.md §9,
// docs/ROADMAP.md post-alpha backlog).
func newMcpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "run the Model Context Protocol server (stdio)",
		Long: `Run Chore Crusher as an MCP server on stdin/stdout.

Agents can call its tools directly instead of shelling out for every
operation. The tools mirror the CLI subcommands and return the same JSON
shapes that --json would emit on the command line.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			errSilence(cmd)
			return mcpserver.Run(cmd.Context())
		},
	}
}
