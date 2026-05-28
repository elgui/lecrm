package audit

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// FormatTable writes entries to w as a human-readable table.
func FormatTable(w io.Writer, entries []Entry) error {
	if len(entries) == 0 {
		_, err := fmt.Fprintln(w, "(no audit entries)")
		return err
	}
	header := fmt.Sprintf("%-25s  %-30s  %-12s  %s",
		"OCCURRED_AT", "EVENT", "ACTOR", "PAYLOAD")
	if _, err := fmt.Fprintln(w, header); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, strings.Repeat("-", len(header))); err != nil {
		return err
	}
	for _, e := range entries {
		actor := e.ActorType
		if actor == "" {
			actor = "-"
		}
		payload, _ := json.Marshal(e.Payload)
		if _, err := fmt.Fprintf(w, "%-25s  %-30s  %-12s  %s\n",
			e.OccurredAt.UTC().Format("2006-01-02 15:04:05Z"),
			truncate(e.Event, 30),
			truncate(actor, 12),
			string(payload)); err != nil {
			return err
		}
	}
	return nil
}

// FormatJSON writes entries to w as a JSON array (newline-terminated).
func FormatJSON(w io.Writer, entries []Entry) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(entries)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}
