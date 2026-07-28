package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/wire/internal/client"
)

// followWait is how long each poll in `wire follow` blocks. It matches the
// server's stream keepalive, so a quiet channel costs one request a minute.
const followWait = 60

var (
	flagTopic  string
	flagAll    bool
	flagSince  int64
	flagWait   int
	flagLimit  int
	flagNote   string
	flagOutput string
)

// api returns a client for the running serve instance. Every mutation goes
// through it so serve stays the single writer.
func api() *client.Client {
	return client.New(fmt.Sprintf("http://localhost:%d", flagPort))
}

var openCmd = &cobra.Command{
	Use:   "open [name]",
	Short: "Open a channel and print the handle to give another session",
	Long: `Open a channel. The name is the entire join protocol: give it to another
session and it can post and read straight away. Omit the name and one is
generated.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var name string
		if len(args) == 1 {
			name = args[0]
		}
		c, err := api().Open(cmd.Context(), client.OpenBody{Name: name, Topic: flagTopic, From: flagFrom})
		if err != nil {
			return err
		}
		if flagOutput == "json" {
			return writeJSON(cmd.OutOrStdout(), c)
		}
		// The connection string goes first and alone on its line: it is the
		// thing to copy, and everything else is context.
		fmt.Fprintln(cmd.OutOrStdout(), c.Connect)
		fmt.Fprintf(cmd.OutOrStdout(), "channel %s · %s\n", c.Name, c.ID)
		fmt.Fprintln(cmd.OutOrStdout(), "paste that first line into the other session")
		return nil
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List channels, most recently active first",
	RunE: func(cmd *cobra.Command, _ []string) error {
		cs, err := api().List(cmd.Context(), flagAll)
		if err != nil {
			return err
		}
		if flagOutput == "json" {
			return writeJSON(cmd.OutOrStdout(), map[string]any{"channels": cs})
		}
		if len(cs) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "no channels")
			return nil
		}
		for _, c := range cs {
			state := "open"
			if c.Closed() {
				state = "closed"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-24s %-6s %3d msg  %s\n",
				c.Name, state, c.Messages, strings.Join(c.Participants, ", "))
		}
		return nil
	},
}

var postCmd = &cobra.Command{
	Use:   "post <channel|connection-string> [message]",
	Short: "Post a message to a channel",
	Long: `Post a message to a channel by name, id, or the connection string another
session handed you. With no message argument the body is read from stdin, so a
command's output can be piped into a conversation.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		body, err := messageBody(cmd.InOrStdin(), args[1:])
		if err != nil {
			return err
		}
		m, err := api().Post(cmd.Context(), args[0], client.PostBody{From: flagFrom, Body: body})
		if err != nil {
			return err
		}
		if flagOutput == "json" {
			return writeJSON(cmd.OutOrStdout(), m)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "#%d  %s\n", m.Seq, m.From)
		return nil
	},
}

// messageBody takes the message from the arguments, or from stdin when there
// are none.
func messageBody(stdin io.Reader, args []string) (string, error) {
	if len(args) > 0 {
		return strings.Join(args, " "), nil
	}
	raw, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("read message from stdin: %w", err)
	}
	return string(raw), nil
}

var readCmd = &cobra.Command{
	Use:   "read <channel|connection-string>",
	Short: "Read a channel's messages, optionally waiting for the next one",
	Long: `Read the messages on a channel. With --since only what came after that cursor
is returned; with --wait the call blocks until a message arrives or the wait
expires, which is how you wait for another session's reply.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		b, err := api().Read(cmd.Context(), args[0], client.ReadOptions{
			Since: flagSince, Limit: flagLimit, Wait: flagWait,
		})
		if err != nil {
			return err
		}
		if flagOutput == "json" {
			return writeJSON(cmd.OutOrStdout(), b)
		}
		printMessages(cmd.OutOrStdout(), b.Messages)
		fmt.Fprintf(cmd.OutOrStdout(), "-- cursor %d%s\n", b.Cursor, closedSuffix(b.Channel))
		return nil
	},
}

var followCmd = &cobra.Command{
	Use:   "follow <channel|connection-string>",
	Short: "Print messages as they arrive until the channel closes",
	Long: `Follow a channel, printing each message as it lands. It blocks between
messages rather than polling, and returns when the channel is closed or you
interrupt it.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return follow(ctx, cmd.OutOrStdout(), args[0], flagSince)
	},
}

// follow prints messages as they arrive, blocking between them, until the
// channel closes or ctx is done. The blocking read carries ctx, so an
// interrupt lands immediately instead of after the current wait.
func follow(ctx context.Context, w io.Writer, ref string, since int64) error {
	c := api()
	for {
		b, err := c.Read(ctx, ref, client.ReadOptions{Since: since, Wait: followWait})
		if err != nil {
			if ctx.Err() != nil {
				return nil // interrupted; a partial transcript is not a failure
			}
			return err
		}
		printMessages(w, b.Messages)
		since = b.Cursor
		if b.Channel.Closed() {
			fmt.Fprintf(w, "-- channel closed%s\n", closedSuffix(b.Channel))
			return nil
		}
	}
}

var connectCmd = &cobra.Command{
	Use:   "connect <channel|connection-string>",
	Short: "Print a channel's connection string",
	Long: `Print the connection string for an existing channel and nothing else, so it
can be piped or copied straight into another session:

  wire connect refactor-auth | pbcopy`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := api().Get(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), c.Connect)
		return nil
	},
}

var closeCmd = &cobra.Command{
	Use:   "close <channel|connection-string>",
	Short: "End a conversation and wake everyone waiting on it",
	Long: `Close a channel. It takes no further messages and every session blocked on a
read wakes immediately instead of waiting for a reply that will never come.
The transcript stays readable.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := api().Close(cmd.Context(), args[0], flagNote)
		if err != nil {
			return err
		}
		if flagOutput == "json" {
			return writeJSON(cmd.OutOrStdout(), c)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s closed  %d messages\n", c.Name, c.Messages)
		return nil
	},
}

// printMessages renders a transcript: a header line per turn, then the body
// indented under it.
func printMessages(w io.Writer, msgs []client.Message) {
	for _, m := range msgs {
		fmt.Fprintf(w, "#%d  %s  %s\n", m.Seq, m.From, m.CreatedAt.Local().Format(time.Stamp))
		for _, line := range strings.Split(m.Body, "\n") {
			fmt.Fprintf(w, "    %s\n", line)
		}
	}
}

// closedSuffix renders a closed channel's parting note, or nothing at all.
func closedSuffix(c client.Channel) string {
	if !c.Closed() {
		return ""
	}
	if c.CloseNote == "" {
		return "  (closed)"
	}
	return "  (closed: " + c.CloseNote + ")"
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func init() {
	openCmd.Flags().StringVar(&flagTopic, "topic", "", "one-line description of what the conversation is for")
	listCmd.Flags().BoolVar(&flagAll, "all", false, "include closed channels")
	readCmd.Flags().Int64Var(&flagSince, "since", 0, "only messages after this cursor")
	readCmd.Flags().IntVar(&flagWait, "wait", 0, "seconds to block waiting for a new message (max 120)")
	readCmd.Flags().IntVar(&flagLimit, "limit", 0, "maximum messages to return")
	followCmd.Flags().Int64Var(&flagSince, "since", 0, "start after this cursor instead of the beginning")
	closeCmd.Flags().StringVar(&flagNote, "note", "", "parting note: how the conversation ended")

	for _, c := range []*cobra.Command{openCmd, listCmd, postCmd, readCmd, closeCmd} {
		c.Flags().StringVarP(&flagOutput, "output", "o", "text", "Output format: text or json")
		rootCmd.AddCommand(c)
	}
	rootCmd.AddCommand(followCmd)
	rootCmd.AddCommand(connectCmd)
}
