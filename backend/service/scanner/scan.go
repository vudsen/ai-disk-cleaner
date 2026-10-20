package scanner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	modelscanner "ai-disk-cleanner/backend/model/scanner"

	"github.com/dundee/gdu/v5/build"
	"github.com/dundee/gdu/v5/pkg/analyze"
	"github.com/dundee/gdu/v5/pkg/fs"
)

func scanGDUContext(
	ctx context.Context,
	directoryPath string,
	onProgress func(modelscanner.ScanProgress),
) (*modelscanner.FileTree, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if strings.TrimSpace(directoryPath) == "" {
		return nil, errors.New("parse gdu directory: directory path is empty")
	}

	absoluteDirectoryPath, err := filepath.Abs(directoryPath)
	if err != nil {
		return nil, fmt.Errorf("resolve directory path %q: %w", directoryPath, err)
	}
	info, err := os.Stat(absoluteDirectoryPath)
	if err != nil {
		return nil, fmt.Errorf("stat directory %q: %w", absoluteDirectoryPath, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("parse gdu directory %q: path is not a directory", absoluteDirectoryPath)
	}
	if err := validateScanRoot(absoluteDirectoryPath); err != nil {
		return nil, fmt.Errorf("validate scan root %q: %w", absoluteDirectoryPath, err)
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("analyze directory %q: %w", absoluteDirectoryPath, err)
	}
	policy, err := buildScanPolicy(ctx, absoluteDirectoryPath)
	if err != nil {
		return nil, fmt.Errorf("prepare scan policy for %q: %w", absoluteDirectoryPath, err)
	}

	analyzer := analyze.CreateAnalyzer()
	stopProgress := func() {}
	if onProgress != nil {
		progressDone := make(chan struct{})
		var progressConsumer sync.WaitGroup
		progressConsumer.Add(1)
		go func() {
			defer progressConsumer.Done()
			for {
				select {
				case progress, ok := <-analyzer.GetProgressChan():
					if !ok {
						return
					}
					onProgress(modelscanner.ScanProgress{
						CurrentPath: progress.CurrentItemName,
						ItemCount:   int64(progress.ItemCount),
						ScannedSize: progress.TotalSize,
					})
				case <-progressDone:
					return
				case <-ctx.Done():
					return
				}
			}
		}()
		stopProgress = func() { stopProgressConsumer(progressDone, &progressConsumer) }
	}

	directory := analyzer.AnalyzeDir(
		absoluteDirectoryPath,
		policy.ignoreDirectory,
		policy.ignoreFile,
	)
	stopProgress()
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("analyze directory %q: %w", absoluteDirectoryPath, err)
	}
	if directory == nil {
		return nil, fmt.Errorf("analyze directory %q: gdu returned no directory", absoluteDirectoryPath)
	}
	summary := policy.summary()
	if analyzedDirectory, ok := directory.(*analyze.Dir); ok {
		collectGDUReadErrors(analyzedDirectory, &summary)
	}
	directory.UpdateStats(make(fs.HardLinkedItems, 10))

	var reportBuffer bytes.Buffer
	fmt.Fprintf(
		&reportBuffer,
		`[1,2,{"progname":"gdu","progver":%q,"timestamp":%d},`+"\n",
		build.Version,
		time.Now().Unix(),
	)
	if err := directory.EncodeJSON(&reportBuffer, true); err != nil {
		return nil, fmt.Errorf("encode analysis for directory %q: %w", absoluteDirectoryPath, err)
	}
	if _, err := reportBuffer.WriteString("]\n"); err != nil {
		return nil, fmt.Errorf("encode analysis for directory %q: %w", absoluteDirectoryPath, err)
	}

	tree, err := parseGDUJSON(&reportBuffer)
	if err != nil {
		return nil, fmt.Errorf("parse analysis for directory %q: %w", absoluteDirectoryPath, err)
	}
	if summary.Warnings == nil {
		summary.Warnings = make([]modelscanner.ScanWarning, 0)
	}
	tree.ScanSummary = summary
	return tree, nil
}

func collectGDUReadErrors(directory *analyze.Dir, summary *modelscanner.ScanSummary) {
	if directory == nil || summary == nil {
		return
	}
	if directory.GetFlag() == '!' {
		summary.ReadErrorCount++
		summary.AddWarning(directory.GetPath(), "read_error")
	}
	for _, item := range directory.Files {
		if child, ok := item.(*analyze.Dir); ok {
			collectGDUReadErrors(child, summary)
		}
	}
}

func stopProgressConsumer(stop chan struct{}, consumer *sync.WaitGroup) {
	close(stop)
	consumer.Wait()
}
