package scanner

import "testing"

func TestScanSummaryBoundsWarningDetailsButKeepsFullCounts(t *testing.T) {
	var summary ScanSummary
	for index := 0; index < MaxScanWarnings+7; index++ {
		summary.ReadErrorCount++
		summary.AddWarning("/path", "read_error")
	}
	if summary.ReadErrorCount != MaxScanWarnings+7 {
		t.Fatalf("ReadErrorCount = %d", summary.ReadErrorCount)
	}
	if len(summary.Warnings) != MaxScanWarnings {
		t.Fatalf("warning count = %d, want %d", len(summary.Warnings), MaxScanWarnings)
	}
}
