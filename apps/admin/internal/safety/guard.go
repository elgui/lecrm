// Package safety holds the startup defense-in-depth checks for
// lecrm-admin. AC-D5 (Story 8.1): the admin binary refuses to start if
// any LECRM_API_* env var is present in its process — Winston's R2
// condition for accepting binary co-location in the same Docker image.
//
// The check is cheap (single env scan) so we run it unconditionally on
// every invocation, not just inside the Docker entrypoint. If a future
// contributor reuses the binary for a non-CLI codepath the guard still
// fires.
package safety

import (
	"fmt"
	"os"
	"strings"
)

// CheckAPIEnvLeak scans the process environment for LECRM_API_* keys
// and returns a descriptive error if any are present. Call this before
// opening DB connections or doing any work.
func CheckAPIEnvLeak() error {
	const prefix = "LECRM_API_"
	var leaked []string
	for _, kv := range os.Environ() {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			continue
		}
		if strings.HasPrefix(kv[:eq], prefix) {
			leaked = append(leaked, kv[:eq])
		}
	}
	if len(leaked) == 0 {
		return nil
	}
	return fmt.Errorf("refusing to start: %s env vars present in process (%s); lecrm-admin must run in a clean environment per AC-D5",
		prefix, strings.Join(leaked, ", "))
}
