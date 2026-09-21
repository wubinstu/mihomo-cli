package geo

import (
	"fmt"
	"testing"
)

func TestRegionDirect(t *testing.T) {
	for _, s := range []string{"104.17.186.37", "162.159.38.144", "1.1.1.1"} {
		fmt.Printf("%s -> %q -> %q\n", s, Region(s), Name(Region(s)))
	}
}
