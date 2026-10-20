//go:build linux

package cleaner

import "testing"

func TestSamePathPreservesLinuxCase(t *testing.T) {
	if samePath("/tmp/Cache", "/tmp/cache") {
		t.Fatal("Linux paths differing by case must remain distinct")
	}
	if !samePath("/tmp/Cache", "/tmp/Cache") {
		t.Fatal("identical Linux paths must match")
	}
}
