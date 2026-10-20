package scansnapshot

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	modelscanner "ai-disk-cleanner/backend/model/scanner"
)

const snapshotVersion = 1

type treeHeader struct {
	Version  int
	RootPath string
	Metadata modelscanner.GDUReportMetadata
	Summary  modelscanner.ScanSummary
}

// Paths and lookup maps are reconstructed; storing them per node would multiply snapshot size.
type storedNode struct {
	End                    bool
	Parent                 int
	Name                   string
	Type                   modelscanner.NodeType
	ApparentSize, DiskSize int64
	ModifiedAt             time.Time
	NotRegular, HardLink   bool
	Inode                  uint64
	ItemCount              int64
}

func encodeTree(ctx context.Context, tree *modelscanner.FileTree) ([]byte, error) {
	if tree == nil || tree.Root == nil {
		return nil, errors.New("scan tree is empty")
	}
	var output bytes.Buffer
	writer, _ := gzip.NewWriterLevel(&output, gzip.BestSpeed)
	encoder := gob.NewEncoder(writer)
	if err := encoder.Encode(treeHeader{snapshotVersion, tree.RootPath, tree.Metadata, tree.ScanSummary}); err != nil {
		return nil, err
	}
	index := 0
	var visit func(*modelscanner.FileNode, int) error
	visit = func(node *modelscanner.FileNode, parent int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if node == nil {
			return errors.New("nil scan tree node")
		}
		current := index
		index++
		row := storedNode{Parent: parent, Name: node.Name, Type: node.Type, ApparentSize: node.ApparentSize, DiskSize: node.DiskSize, ModifiedAt: node.ModifiedAt, NotRegular: node.NotRegular, HardLink: node.HardLink, Inode: node.Inode, ItemCount: node.ItemCount}
		if err := encoder.Encode(row); err != nil {
			return err
		}
		for _, child := range node.Children {
			if err := visit(child, current); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(tree.Root, -1); err != nil {
		writer.Close()
		return nil, err
	}
	if err := encoder.Encode(storedNode{End: true}); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
func decodeTree(ctx context.Context, payload []byte) (*modelscanner.FileTree, error) {
	reader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	decoder := gob.NewDecoder(reader)
	var header treeHeader
	if err := decoder.Decode(&header); err != nil {
		return nil, err
	}
	if header.Version != snapshotVersion {
		return nil, fmt.Errorf("unsupported snapshot version %d", header.Version)
	}
	tree := &modelscanner.FileTree{RootPath: header.RootPath, Metadata: header.Metadata, ScanSummary: header.Summary}
	nodes := make([]*modelscanner.FileNode, 0)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var row storedNode
		if err := decoder.Decode(&row); err != nil {
			return nil, err
		}
		if row.End {
			break
		}
		if row.Type != modelscanner.NodeTypeDirectory && row.Type != modelscanner.NodeTypeFile {
			return nil, errors.New("invalid snapshot node type")
		}
		node := &modelscanner.FileNode{Name: row.Name, Type: row.Type, ApparentSize: row.ApparentSize, DiskSize: row.DiskSize, ModifiedAt: row.ModifiedAt, NotRegular: row.NotRegular, HardLink: row.HardLink, Inode: row.Inode, ItemCount: row.ItemCount}
		if node.IsDir() {
			node.ChildrenByName = make(map[string]*modelscanner.FileNode)
		}
		if len(nodes) == 0 {
			if row.Parent != -1 {
				return nil, errors.New("invalid snapshot root")
			}
			node.RelativePath = "."
			node.FullPath = header.RootPath
			tree.Root = node
		} else {
			if row.Parent < 0 || row.Parent >= len(nodes) || !nodes[row.Parent].IsDir() || row.Name == "" || row.Name == "." || row.Name == ".." || strings.Contains(row.Name, "/") {
				return nil, errors.New("invalid snapshot parent or name")
			}
			parent := nodes[row.Parent]
			if _, exists := parent.ChildrenByName[node.Name]; exists {
				return nil, errors.New("duplicate snapshot child")
			}
			node.RelativePath = node.Name
			if parent.RelativePath != "." {
				node.RelativePath = parent.RelativePath + "/" + node.Name
			}
			separator := "/"
			if strings.HasPrefix(header.RootPath, `\\`) || len(header.RootPath) > 2 && header.RootPath[1] == ':' {
				if strings.LastIndex(header.RootPath, `\`) > strings.LastIndex(header.RootPath, "/") {
					separator = `\`
				}
			}
			node.FullPath = strings.TrimRight(parent.FullPath, separator) + separator + node.Name
			parent.Children = append(parent.Children, node)
			parent.ChildrenByName[node.Name] = node
		}
		nodes = append(nodes, node)
	}
	if tree.Root == nil {
		return nil, errors.New("snapshot has no root")
	}
	var trailing storedNode
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = errors.New("trailing snapshot data")
		}
		return nil, err
	}
	return tree, nil
}
