//go:build !linux

package scanner

func validateScanRoot(string) error { return nil }
