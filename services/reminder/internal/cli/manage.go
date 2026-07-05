package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/reminder/internal/client"
)

// apiClient targets the local serve agent. The CLI, like the MCP, is a thin
// client of serve (the single writer) rather than touching the store directly.
func apiClient() *client.Client {
	return client.New(fmt.Sprintf("http://localhost:%d", flagPort))
}

// dueFormats are accepted by --due, tried in order and interpreted in the local
// timezone, then normalized to RFC3339 for the API.
var dueFormats = []string{
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02T15:04",
	"2006-01-02 15:04",
}

func parseDueFlag(s string) (string, error) {
	for _, f := range dueFormats {
		if t, err := time.ParseInLocation(f, s, time.Local); err == nil {
			return t.Format(time.RFC3339), nil
		}
	}
	return "", fmt.Errorf("invalid --due %q: use RFC3339 or '2006-01-02T15:04'", s)
}

func printReminder(w *cobra.Command, r client.Reminder) {
	status := r.Status
	if r.Overdue {
		status = "overdue"
	}
	repeat := "-"
	if r.Repeat != "" {
		repeat = r.Repeat
	}
	fmt.Fprintf(w.OutOrStdout(), "%s  %s  due %s  repeat %s  %s\n",
		r.ID, r.Title, r.Due.Local().Format("Mon Jan 2 15:04"), repeat, status)
}

var (
	addIn, addDue, addBody, addRepeat        string
	listStatus                               string
	editTitle, editBody, editDue, editRepeat string
)

var addCmd = &cobra.Command{
	Use:   "add <title>",
	Short: "Add a reminder (--in <dur> or --due <time>)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		body := client.CreateBody{Title: args[0], Body: addBody, In: addIn, Repeat: addRepeat}
		if addDue != "" {
			due, err := parseDueFlag(addDue)
			if err != nil {
				return err
			}
			body.Due = due
		}
		r, err := apiClient().Create(body)
		if err != nil {
			return err
		}
		printReminder(cmd, r)
		return nil
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List reminders, soonest due first (--status to filter)",
	RunE: func(cmd *cobra.Command, _ []string) error {
		rs, err := apiClient().List(listStatus)
		if err != nil {
			return err
		}
		if len(rs) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "no reminders")
			return nil
		}
		tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
		for _, r := range rs {
			status := r.Status
			if r.Overdue {
				status = "overdue"
			}
			repeat := "-"
			if r.Repeat != "" {
				repeat = r.Repeat
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				r.ID, r.Title, r.Due.Local().Format("Mon Jan 2 15:04"), repeat, status)
		}
		return tw.Flush()
	},
}

var getCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Show one reminder",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := apiClient().Get(args[0])
		if err != nil {
			return err
		}
		printReminder(cmd, r)
		return nil
	},
}

var cancelCmd = &cobra.Command{
	Use:   "cancel <id>",
	Short: "Soft-cancel a reminder (stops it firing, keeps the record)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := apiClient().Cancel(args[0])
		if err != nil {
			return err
		}
		printReminder(cmd, r)
		return nil
	},
}

var editCmd = &cobra.Command{
	Use:   "edit <id>",
	Short: "Edit a reminder's title/body/due/repeat",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var body client.UpdateBody
		if cmd.Flags().Changed("title") {
			body.Title = &editTitle
		}
		if cmd.Flags().Changed("body") {
			body.Body = &editBody
		}
		if cmd.Flags().Changed("repeat") {
			body.Repeat = &editRepeat
		}
		if cmd.Flags().Changed("due") {
			due, err := parseDueFlag(editDue)
			if err != nil {
				return err
			}
			body.Due = &due
		}
		r, err := apiClient().Update(args[0], body)
		if err != nil {
			return err
		}
		printReminder(cmd, r)
		return nil
	},
}

var testCmd = &cobra.Command{
	Use:   "test [id]",
	Short: "Send a test notification now to verify notifications work (no id = generic test)",
	Long: `Fire a notification immediately to confirm macOS notifications work on this
machine. With an id, it sends that reminder's exact notification without
changing its state (a dry run — no one-shot consumed, no schedule advanced).
With no id, it sends a generic "notifications are working" test.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			if err := apiClient().TestNotification(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "sent test notification")
			return nil
		}
		r, err := apiClient().Test(args[0])
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), "sent test notification for:")
		printReminder(cmd, r)
		return nil
	},
}

var fireCmd = &cobra.Command{
	Use:   "fire <id>",
	Short: "Fire a reminder for real now (advances recurring / marks one-shot fired)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := apiClient().Fire(args[0])
		if err != nil {
			return err
		}
		printReminder(cmd, r)
		return nil
	},
}

func init() {
	addCmd.Flags().StringVar(&addIn, "in", "", "relative due time as a Go duration (e.g. 2h30m)")
	addCmd.Flags().
		StringVar(&addDue, "due", "", "absolute due time (RFC3339 or '2006-01-02T15:04', local tz)")
	addCmd.Flags().StringVar(&addBody, "body", "", "optional note")
	addCmd.Flags().
		StringVar(&addRepeat, "repeat", "", "recurrence: daily, weekly, or a Go duration")

	listCmd.Flags().StringVar(&listStatus, "status", "", "filter: pending|fired|done|cancelled")

	editCmd.Flags().StringVar(&editTitle, "title", "", "new title")
	editCmd.Flags().StringVar(&editBody, "body", "", "new note")
	editCmd.Flags().
		StringVar(&editDue, "due", "", "new due time (RFC3339 or '2006-01-02T15:04', local tz)")
	editCmd.Flags().StringVar(&editRepeat, "repeat", "", "new recurrence (empty for one-shot)")

	rootCmd.AddCommand(addCmd, listCmd, getCmd, cancelCmd, editCmd, testCmd, fireCmd)
}
