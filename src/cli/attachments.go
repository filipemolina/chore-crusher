package cli

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/filipemolina/farol/src/config"
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

// attachCmd adds a file attachment to a task. The source argument may be a
// local file path (stored as-is, the original contract), "-" to read the
// attachment from stdin, or an http(s) URL that is downloaded first; with no
// source argument the attachment is read from stdin. Stdin and URL
// attachments are materialized as files under $XDG_DATA_HOME/farol/attachments
// (src/config/dirs.go) so the content lives with the store's other data, and
// that path is what gets stored — the store's Attachment model stores a path
// reference, not file content.
func attachCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "attach <task-id> [source]",
		Short: "Add a file attachment to a task",
		Long: `Attach a file to a task. The source argument may be a local file
path (stored as-is, no content is copied), "-" to read the attachment from
stdin, or an http(s) URL that is downloaded first. With no source argument the
attachment is read from stdin. Stdin and URL attachments are written under
$XDG_DATA_HOME/farol/attachments, and that path is what gets stored.`,
		Args: cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			errSilence(cmd)
			return runStore(cmd, func(s *store.Store) error {
				taskID := args[0]
				source := ""
				if len(args) == 2 {
					source = args[1]
				}

				path, created, err := resolveAttachmentSource(source, os.Stdin)
				if err != nil {
					return err
				}

				id, err := s.AddAttachment(taskID, path)
				if err != nil {
					// A stdin/URL attach materialized a file on disk; a failed
					// insert (unresolvable task id, ...) must not orphan it.
					if created {
						os.Remove(path)
					}
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

// resolveAttachmentSource turns the attach command's source argument into a
// local file path for the store, and reports whether it materialized a file
// (so a failed insert can clean it up). "-" or no source reads stdin into a
// file; an http(s) URL is downloaded into one; anything else is returned
// as-is, matching the store's store-a-path-unvalidated contract.
func resolveAttachmentSource(source string, stdin io.Reader) (path string, created bool, err error) {
	if source == "" || source == "-" {
		p, err := stdinAttachment(stdin)
		return p, true, err
	}

	u, perr := url.Parse(source)
	if perr != nil {
		// A string that clearly carried a scheme but failed to parse (a space
		// in the host, ...) is a broken URL, not a local path.
		if strings.Contains(source, "://") {
			return "", false, fmt.Errorf("invalid URL %q: %v", source, perr)
		}
		return source, false, nil
	}
	if u.Scheme != "" {
		if u.Scheme != "http" && u.Scheme != "https" {
			return "", false, fmt.Errorf("unsupported URL scheme %q in %q (use http or https)", u.Scheme, source)
		}
		p, err := downloadAttachment(u)
		return p, true, err
	}
	return source, false, nil
}

// attachmentDir returns (creating if needed) the directory where stdin and
// URL attachment content is stored: config.DataDir()/attachments. Content
// must survive as long as the store row referencing it, so it goes with the
// store's data — not into a temp dir that would vanish or a working
// directory the next command might not share.
func attachmentDir() (string, error) {
	dir := filepath.Join(config.DataDir(), "attachments")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// writeAttachment stores r under name inside the attachment dir and returns
// the stored path. The caller picks the name: a URL's basename for a URL
// attachment, a fresh ULID for stdin, which carries no name of its own.
func writeAttachment(name string, r io.Reader) (string, error) {
	dir, err := attachmentDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

// stdinAttachment stores the whole of stdin as an attachment. Stdin has no
// name, so the file gets a fresh ULID (store.NewID) rather than a guess.
func stdinAttachment(r io.Reader) (string, error) {
	return writeAttachment(store.NewID(), r)
}

// downloadAttachment fetches u and stores the response body as an attachment,
// named after the URL's path basename so a .pdf stays openable as a .pdf
// (filepath.Base strips the directory and any traversal, so the name cannot
// escape the attachment dir). A non-2xx response is an error, not a stored
// file.
func downloadAttachment(u *url.URL) (string, error) {
	// The default http client has no timeout; a hung download would block the
	// CLI — and an agent driving it — forever.
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(u.String())
	if err != nil {
		return "", fmt.Errorf("download %s: %w", u, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", u, resp.StatusCode)
	}

	name := filepath.Base(u.Path)
	if name == "" || name == "." || name == ".." || name == "/" {
		name = store.NewID()
	}
	return writeAttachment(name, resp.Body)
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
