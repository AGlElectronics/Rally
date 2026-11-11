package migrations

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/AGlElectronics/rally/backend/internal/denlink"
)

// The den_link table must match what the synced denlink package queries.
func TestDenLinkSchemaUpgradeIsMigrated(t *testing.T) {
	names, err := fs.Glob(FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		b, err := fs.ReadFile(FS, name)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), denlink.SchemaUpgrade) {
			return
		}
	}
	t.Fatalf("no migration contains denlink.SchemaUpgrade %q", denlink.SchemaUpgrade)
}
