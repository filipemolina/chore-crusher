package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/filipemolina/farol/src/store"
)

// attachmentCommands returns the attach/detach/attachments subcommands.
func attachmentCommands() []*cobra.Command {
	return []*cobra.Command{
		attachCmd(),
		detachCmd(),
		attachmentsCmd(),
	}
}

// attachCmd adds a file attachment to a task.
func attachCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attach <task-id> <path>",
		Short: "Add a file attachment to a task",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			errSilence(cmd)
			return runStore(cmd, func(s *store.Store) error {
				taskID := args[0]
				path := args[1]

				id, err := s.AddAttachment(taskID, path)
				if err != nil {
					return err
				}

				jsonMode, _ := cmd.Flags().GetBool("json")
				printResult(jsonMode, func() {
					fmt.Println(id)
				}, map[string]string{"id": id})
				return nil
			})
		},
	}
	return cmd
}

// detachCmd removes a file attachment from a task.
func detachCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "detach <attachment-id>",
		Short: "Remove a file attachment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			errSilence(cmd)
			return runStore(cmd, func(s *store.Store) error {
				id := args[0]

				if err := s.DeleteAttachment(id); err != nil {
					return err
				}

				jsonMode, _ := cmd.Flags().GetBool("json")
				printResult(jsonMode, func() {
					fmt.Println("OK")
				}, okPayload{OK: true})
				return nil
			})
		},
	}
	return cmd
}

// attachmentsCmd lists all attachments for a task.
func attachmentsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attachments <task-id>",
		Short: "List all attachments for a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			errSilence(cmd)
			return runStore(cmd, func(s *store.Store) error {
				taskID := args[0]

				attachments, err := s.ListAttachments(taskID)
				if err != nil {
					return err
				}

				jsonMode, _ := cmd.Flags().GetBool("json")
				printResult(jsonMode, func() {
					// Human-readable output
					if len(attachments) == 0 {
						fmt.Println("No attachments")
						return
					}

					for _, a := range attachments {
						fmt.Printf("%s  %s\n", a.ID, a.Path)
					}
				}, attachments)
				return nil
			})
		},
	}
	return cmd
}
