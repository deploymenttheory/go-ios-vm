package inspection

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/blacktop/ipsw/pkg/devicetree"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

var (
	errDeviceTreeDepthNodeBudgetExceeded  = errors.New("device-tree depth/node budget exceeded")
	errTruncatedDeviceTreeNode            = errors.New("truncated device-tree node")
	errDeviceTreePropertyBudgetExceeded   = errors.New("device-tree property budget exceeded")
	errTruncatedDeviceTreeProperty        = errors.New("truncated device-tree property")
	errDeviceTreePropertyOutsidePayload   = errors.New("device-tree property outside payload")
	errDuplicateDeviceTreeProperty        = errors.New("duplicate device-tree property")
	errUnnamedDeviceTreeNode              = errors.New("unnamed device-tree node")
	errAmbiguousDuplicateDeviceTreePath   = errors.New("ambiguous duplicate device-tree path")
	errDeviceTreeTrailingBytes            = errors.New("device-tree trailing bytes")
	errUpstreamPropertyLacksSourceSpan    = errors.New("upstream property lacks source span")
	errUpstreamChildRepresentationChanged = errors.New("upstream child representation changed")
)

type propertySpan struct{ offset, length uint64 }

// Scan structure for bounds and byte coordinates before invoking the upstream value decoder.
// This adapter supplies provenance absent from the upstream map API; it is not a codec.
func treeSpans(data []byte) (map[string]map[string]propertySpan, error) {
	spans := map[string]map[string]propertySpan{}
	pos, nodes, properties := uint64(0), 0, uint64(0)
	var scan func(string, int) error
	scan = func(parent string, depth int) error {
		if depth > 128 || nodes >= 10000 {
			return errDeviceTreeDepthNodeBudgetExceeded
		}
		nodes++
		if uint64(len(data))-pos < 8 {
			return errTruncatedDeviceTreeNode
		}
		np, nc := binary.LittleEndian.Uint32(data[pos:]), binary.LittleEndian.Uint32(data[pos+4:])
		pos += 8
		if np > 10000 || nc > 10000 || properties+uint64(np) > 50000 {
			return errDeviceTreePropertyBudgetExceeded
		}
		properties += uint64(np)
		local := map[string]propertySpan{}
		name := ""
		for j := uint32(0); j < np; j++ {
			if uint64(len(data))-pos < 36 {
				return errTruncatedDeviceTreeProperty
			}
			key := strings.TrimRight(string(data[pos:pos+32]), "\x00")
			n := uint64(binary.LittleEndian.Uint32(data[pos+32:]) & 0x7fffffff)
			pos += 36
			padded := (n + 3) &^ uint64(3)
			if padded > uint64(len(data))-pos {
				return errDeviceTreePropertyOutsidePayload
			}
			if _, exists := local[key]; exists {
				return fmt.Errorf("%w: %q", errDuplicateDeviceTreeProperty, key)
			}
			local[key] = propertySpan{pos, n}
			if strings.EqualFold(key, "name") {
				name = strings.TrimRight(string(data[pos:pos+n]), "\x00")
			}
			pos += padded
		}
		if name == "" {
			return errUnnamedDeviceTreeNode
		}
		path := parent + "/" + escape(name)
		if _, exists := spans[path]; exists {
			return fmt.Errorf("%w: %q", errAmbiguousDuplicateDeviceTreePath, path)
		}
		spans[path] = local
		for j := uint32(0); j < nc; j++ {
			if err := scan(path, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if err := scan("", 0); err != nil {
		return nil, err
	}
	if pos != uint64(len(data)) {
		return nil, fmt.Errorf("%w: %d", errDeviceTreeTrailingBytes, uint64(len(data))-pos)
	}
	return spans, nil
}

func readTree(
	ctx context.Context,
	t model.Target,
	data, raw []byte,
	src model.Location,
) ([]model.Observation, error) {
	spans, err := treeSpans(data)
	if err != nil {
		return nil, err
	}
	tree, err := devicetree.ParseData(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("readTree: devicetree.ParseData: %w", err)
	}
	out := []model.Observation{}
	evidence := model.NewBlob(raw)
	var walk func(devicetree.DeviceTree, string) error
	walk = func(tree devicetree.DeviceTree, parent string) error {
		for name, props := range tree {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("readTree: Err: %w", err)
			}
			path := parent + "/" + escape(name)
			keys := make([]string, 0, len(props))
			for key := range props {
				if key != "children" {
					keys = append(keys, key)
				}
			}
			sort.Strings(keys)
			for _, key := range keys {
				span, ok := spans[path][key]
				if !ok {
					return fmt.Errorf("%w: %s/%s", errUpstreamPropertyLacksSourceSpan, path, key)
				}
				loc := src
				loc.Offset = span.offset
				loc.Length = span.length
				loc.Selector = path + "/" + escape(key)
				o, err := model.NewObservation(
					t,
					"devicetree"+path,
					key,
					exactJSON(props[key]),
					loc,
					evidence,
					Extractor,
				)
				if err != nil {
					return fmt.Errorf("readTree: model.NewObservation: %w", err)
				}
				out = append(out, o)
			}
			children, ok := props["children"].([]devicetree.DeviceTree)
			if !ok {
				return errUpstreamChildRepresentationChanged
			}
			for _, child := range children {
				if err := walk(child, path); err != nil {
					return err
				}
			}
		}
		return nil
	}
	err = walk(*tree, "")
	return out, err
}
