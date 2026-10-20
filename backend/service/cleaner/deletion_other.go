//go:build !linux

package cleaner

func validateLinuxDeletionBoundary(string, string) error { return nil }
