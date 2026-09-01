package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/events/internal/client"
)

// apiClient targets the local serve agent. The CLI, like the MCP, is a thin
// client of serve (the single writer) rather than touching the store directly.
func apiClient() *client.Client {
	return client.New(fmt.Sprintf("http://localhost:%d", flagPort))
}

var (
	emitSource, emitTitle, emitComponent, emitLevel, emitMessage, emitData string
	emitTags                                                               []string
	listSource, listLevel, listQ                                           string
	listLimit                                                              int
	purgeSource, purgeBefore                                               string
)

var emitCmd = &cobra.Command{
	Use:   "emit",
	Short: "Record a single event",
	RunE: func(cmd *cobra.Command, _ []string) error {
		tags, err := parseTags(emitTags)
		if err != nil {
			return err
		}
		body := client.EmitBody{
			Source:    emitSource,
			Title:     emitTitle,
			Component: emitComponent,
			Level:     emitLevel,
			Message:   emitMessage,
			Tags:      tags,
		}
		if emitData != "" {
			if !json.Valid([]byte(emitData)) {
				return fmt.Errorf("--data is not valid JSON")
			}
			body.Data = json.RawMessage(emitData)
		}
		id, err := apiClient().Emit(body)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), id)
		return nil
	},
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent events, newest first",
	RunE: func(cmd *cobra.Command, _ []string) error {
		evs, err := apiClient().Query(client.QueryFilter{
			Source: listSource,
			Level:  listLevel,
			Q:      listQ,
			Limit:  listLimit,
		})
		if err != nil {
			return err
		}
		if len(evs) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "no events")
			return nil
		}
		tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
		for _, e := range evs {
			src := e.Source
			if e.Component != "" {
				src += "/" + e.Component
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
				e.Time.Local().Format("Jan 2 15:04:05"), src, e.Level, e.Title)
		}
		return tw.Flush()
	},
}

var purgeCmd = &cobra.Command{
	Use:   "purge",
	Short: "Delete events from one source (all, or only those at/before an id)",
	Long: "Delete events from one source. Without --before the source is removed\n" +
		"entirely (memory + JSONL file); with --before <id> only events with\n" +
		"id <= that cursor are dropped. Meant for cleaning junk (e.g. leaked test\n" +
		"events) out of the timeline: the log is otherwise append-only.",
	RunE: func(cmd *cobra.Command, _ []string) error {
		n, err := apiClient().Purge(purgeSource, purgeBefore)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "purged %d event(s) from %s\n", n, purgeSource)
		return nil
	},
}

// parseTags turns repeated --tag key=value flags into a map.
func parseTags(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	tags := make(map[string]string, len(pairs))
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return nil, fmt.Errorf("invalid --tag %q: want key=value", p)
		}
		tags[k] = v
	}
	return tags, nil
}

func init() {
	emitCmd.Flags().StringVar(&emitSource, "source", "", "event source (required)")
	emitCmd.Flags().StringVar(&emitTitle, "title", "", "short event summary (required)")
	emitCmd.Flags().StringVar(&emitComponent, "component", "", "optional sub-area within the source")
	emitCmd.Flags().StringVar(&emitLevel, "level", "", "info (default) | warn | error")
	emitCmd.Flags().StringVar(&emitMessage, "message", "", "optional longer detail")
	emitCmd.Flags().StringArrayVar(&emitTags, "tag", nil, "key=value label, repeatable")
	emitCmd.Flags().StringVar(&emitData, "data", "", "optional structured JSON payload")
	_ = emitCmd.MarkFlagRequired("source")
	_ = emitCmd.MarkFlagRequired("title")

	listCmd.Flags().StringVar(&listSource, "source", "", "filter by source")
	listCmd.Flags().StringVar(&listLevel, "level", "", "filter by level: info|warn|error")
	listCmd.Flags().StringVar(&listQ, "q", "", "case-insensitive substring filter")
	listCmd.Flags().IntVar(&listLimit, "limit", 50, "max events to show")

	purgeCmd.Flags().StringVar(&purgeSource, "source", "", "source to purge (required)")
	purgeCmd.Flags().StringVar(&purgeBefore, "before", "", "only drop events with id <= this cursor")
	_ = purgeCmd.MarkFlagRequired("source")

	rootCmd.AddCommand(emitCmd, listCmd, purgeCmd)
}
