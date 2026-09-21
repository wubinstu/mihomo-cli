package geo

import (
	"fmt"
	"net"
	"testing"

	"github.com/oschwald/maxminddb-golang"

	"github.com/wubinstu/mihomo-cli/internal/app"
	"path/filepath"
)

func TestDump(t *testing.T) {
	db, err := maxminddb.Open(filepath.Join(app.RuntimeDir, "geoip.metadb"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, ip := range []string{"1.1.1.1", "8.8.8.8", "223.5.5.5"} {
		var rec any
		if err := db.Lookup(net.ParseIP(ip), &rec); err != nil {
			t.Fatal(err)
		}
		fmt.Printf("%s -> %#v\n", ip, rec)
	}
}
