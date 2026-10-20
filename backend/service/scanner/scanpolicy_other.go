//go:build !linux

package scanner

import "context"

import modelscanner "ai-disk-cleanner/backend/model/scanner"

type scanPolicy struct {
	ignoreDirectory func(name, path string) bool
	ignoreFile      func(name string) bool
	summary         func() modelscanner.ScanSummary
}

func buildScanPolicy(ctx context.Context, _ string) (scanPolicy, error) {
	return scanPolicy{
		ignoreDirectory: func(string, string) bool { return ctx.Err() != nil },
		ignoreFile:      func(string) bool { return ctx.Err() != nil },
		summary:         func() modelscanner.ScanSummary { return modelscanner.ScanSummary{} },
	}, nil
}
