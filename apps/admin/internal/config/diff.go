package config

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

type DiffOptions struct {
	SlugA string
	SlugB string
}

type DiffEntry struct {
	Path   string
	Type   string // "added", "removed", "changed"
	OldVal string
	NewVal string
}

type DiffResult struct {
	SlugA   string
	SlugB   string
	Entries []DiffEntry
}

func Diff(ctx context.Context, conn *pgx.Conn, opts DiffOptions, stdout io.Writer) (*DiffResult, error) {
	refA, err := ResolveSlug(ctx, conn, opts.SlugA)
	if err != nil {
		return nil, err
	}
	refB, err := ResolveSlug(ctx, conn, opts.SlugB)
	if err != nil {
		return nil, err
	}

	cfgA, err := loadConfig(ctx, conn, refA, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", opts.SlugA, err)
	}
	cfgB, err := loadConfig(ctx, conn, refB, 0)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", opts.SlugB, err)
	}

	mapA, err := toFlatMap(cfgA)
	if err != nil {
		return nil, err
	}
	mapB, err := toFlatMap(cfgB)
	if err != nil {
		return nil, err
	}

	// version_seq and template_name are metadata, not methodology content
	skipKeys := map[string]bool{"version_seq": true, "template_name": true}

	var entries []DiffEntry
	allKeys := mergeKeys(mapA, mapB)
	for _, k := range allKeys {
		if skipKeys[k] {
			continue
		}
		va, inA := mapA[k]
		vb, inB := mapB[k]
		switch {
		case inA && !inB:
			entries = append(entries, DiffEntry{Path: k, Type: "removed", OldVal: va})
		case !inA && inB:
			entries = append(entries, DiffEntry{Path: k, Type: "added", NewVal: vb})
		case va != vb:
			entries = append(entries, DiffEntry{Path: k, Type: "changed", OldVal: va, NewVal: vb})
		}
	}

	result := &DiffResult{SlugA: opts.SlugA, SlugB: opts.SlugB, Entries: entries}

	if len(entries) == 0 {
		_, _ = fmt.Fprintf(stdout, "Tenants %q (v%d) and %q (v%d) — methodology configs are identical.\n",
			opts.SlugA, cfgA.VersionSeq, opts.SlugB, cfgB.VersionSeq)
		return result, nil
	}

	_, _ = fmt.Fprintf(stdout, "Tenants %q (v%d) vs %q (v%d) — %d difference(s):\n\n",
		opts.SlugA, cfgA.VersionSeq, opts.SlugB, cfgB.VersionSeq, len(entries))
	for _, e := range entries {
		switch e.Type {
		case "added":
			_, _ = fmt.Fprintf(stdout, "  + %s: %s\n", e.Path, e.NewVal)
		case "removed":
			_, _ = fmt.Fprintf(stdout, "  - %s: %s\n", e.Path, e.OldVal)
		case "changed":
			_, _ = fmt.Fprintf(stdout, "  ~ %s: %s → %s\n", e.Path, e.OldVal, e.NewVal)
		}
	}
	_, _ = fmt.Fprintln(stdout)
	return result, nil
}

func toFlatMap(cfg *MethodologyConfig) (map[string]string, error) {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	flat := make(map[string]string)
	flatten("", m, flat)
	return flat, nil
}

func flatten(prefix string, val any, out map[string]string) {
	switch v := val.(type) {
	case map[string]any:
		for k, child := range v {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			flatten(p, child, out)
		}
	case []any:
		for i, child := range v {
			p := fmt.Sprintf("%s[%d]", prefix, i)
			flatten(p, child, out)
		}
	default:
		out[prefix] = fmt.Sprintf("%v", v)
	}
}

func mergeKeys(a, b map[string]string) []string {
	seen := make(map[string]bool)
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// groupByTopLevel groups diff entries by their top-level path segment for
// summary output. Not currently used but available for future --summary flag.
func groupByTopLevel(entries []DiffEntry) map[string][]DiffEntry {
	groups := make(map[string][]DiffEntry)
	for _, e := range entries {
		top := e.Path
		if dot := strings.IndexByte(e.Path, '.'); dot > 0 {
			top = e.Path[:dot]
		}
		if bracket := strings.IndexByte(top, '['); bracket > 0 {
			top = top[:bracket]
		}
		groups[top] = append(groups[top], e)
	}
	return groups
}
