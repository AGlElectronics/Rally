package files

import "os"

// envLookup is a thin shim so the files package can read env vars without
// taking a dependency on the config package (which would create an import
// cycle: files is imported by config-wiring code).
func envLookup(key string) string {
	v, _ := os.LookupEnv(key)
	return v
}
