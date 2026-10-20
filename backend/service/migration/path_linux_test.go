//go:build linux

package migration

import "testing"

func TestMigrationPathIdentityPreservesLinuxCase(t *testing.T) {
	if samePath("/tmp/Cache", "/tmp/cache") {
		t.Fatal("Linux paths differing by case must remain distinct")
	}
	if migrationStepKey("/tmp/Cache", "/tmp/dest") == migrationStepKey("/tmp/cache", "/tmp/dest") {
		t.Fatal("case-distinct Linux migration sources must have distinct keys")
	}
}
