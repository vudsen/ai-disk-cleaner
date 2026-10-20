//go:build linux

package util

import (
	"bufio"
	"io"
	"os"
	"strconv"
	"strings"
)

// MountInfo describes one entry from Linux /proc/self/mountinfo.
type MountInfo struct {
	ID             int
	ParentID       int
	MajorMinor     string
	Root           string
	MountPoint     string
	Options        string
	OptionalFields []string
	FSType         string
	Source         string
	SuperOptions   string
}

type mountInfo = MountInfo

var mountInfoEscapes = strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)

func parseMountInfo(reader io.Reader) ([]mountInfo, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	entries := make([]mountInfo, 0)
	for scanner.Scan() {
		entry, ok := parseMountInfoLine(scanner.Text())
		if ok {
			entries = append(entries, entry)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// ReadMountInfo returns the complete mount table, including duplicate paths
// that may describe stacked or bind mounts.
func ReadMountInfo() ([]MountInfo, error) {
	file, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return parseMountInfo(file)
}

func parseMountInfoLine(line string) (mountInfo, bool) {
	separator := strings.Index(line, " - ")
	if separator < 0 {
		return mountInfo{}, false
	}
	left := strings.Fields(line[:separator])
	right := strings.Fields(line[separator+3:])
	if len(left) < 6 || len(right) < 3 {
		return mountInfo{}, false
	}
	id, err := strconv.Atoi(left[0])
	if err != nil {
		return mountInfo{}, false
	}
	parentID, err := strconv.Atoi(left[1])
	if err != nil {
		return mountInfo{}, false
	}
	entry := mountInfo{
		ID:             id,
		ParentID:       parentID,
		MajorMinor:     left[2],
		Root:           decodeMountInfoField(left[3]),
		MountPoint:     decodeMountInfoField(left[4]),
		Options:        left[5],
		OptionalFields: append([]string(nil), left[6:]...),
		FSType:         right[0],
		Source:         decodeMountInfoField(right[1]),
		SuperOptions:   strings.Join(right[2:], " "),
	}
	return entry, true
}

func decodeMountInfoField(value string) string { return mountInfoEscapes.Replace(value) }
