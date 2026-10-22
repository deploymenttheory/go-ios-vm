package inspection

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// IOKit serializes shared objects with ID and IDREF attributes. Ordinary plist
// decoders interpret a self-closing <integer IDREF="..."/> as an empty integer.
// The input is already capped at 16 MiB by readKernel; these limits also bound
// the object graph and the size of its expanded JSON representation.
const (
	prelinkSectionLimit  = 16 << 20
	prelinkNodeLimit     = 200_000
	prelinkDepthLimit    = 128
	prelinkExpandedLimit = 16 << 20
)

var errInvalidPrelink = errors.New("invalid prelink metadata")

const plistDTD = `DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd"`

type prelinkNode struct {
	kind     string
	text     []byte
	children []*prelinkNode
	target   *prelinkNode
	ref      uint64
	isRef    bool
	size     uint64
}

func parsePrelink(data []byte) (map[string]any, error) {
	if len(data) > prelinkSectionLimit {
		return nil, errDriverMetadataExceeds16MiB
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	definitions := map[uint64]*prelinkNode{}
	var references []*prelinkNode
	var stack []*prelinkNode
	var root *prelinkNode
	nodes := 0
	for {
		token, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("%w: prelink XML: %w", errInvalidPrelink, err)
		}
		switch t := token.(type) {
		case xml.StartElement:
			nodes++
			if nodes > prelinkNodeLimit || len(stack) >= prelinkDepthLimit {
				return nil, fmt.Errorf("%w: prelink object limit exceeded", errInvalidPrelink)
			}
			n, err := newPrelinkNode(t, definitions)
			if err != nil {
				return nil, err
			}
			if n.isRef {
				references = append(references, n)
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, fmt.Errorf("%w: prelink has multiple roots", errInvalidPrelink)
				}
				root = n
			} else {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) == 0 || stack[len(stack)-1].kind != t.Name.Local {
				return nil, fmt.Errorf("%w: prelink closing tag mismatch", errInvalidPrelink)
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 {
				if len(bytes.TrimSpace(t)) != 0 {
					return nil, fmt.Errorf("%w: prelink text outside root", errInvalidPrelink)
				}
				continue
			}
			n := stack[len(stack)-1]
			n.text = append(n.text, t...)
		case xml.Directive:
			if string(t) != plistDTD {
				return nil, fmt.Errorf(
					"%w: prelink XML directive is unsupported",
					errInvalidPrelink,
				)
			}
		}
	}
	if root == nil || len(stack) != 0 {
		return nil, fmt.Errorf("%w: prelink has no complete root", errInvalidPrelink)
	}
	for _, n := range references {
		target, ok := definitions[n.ref]
		if !ok {
			return nil, fmt.Errorf("%w: prelink dangling IDREF %d", errInvalidPrelink, n.ref)
		}
		if target.kind != n.kind {
			return nil, fmt.Errorf(
				"%w: prelink IDREF %d has type %s; definition is %s",
				errInvalidPrelink,
				n.ref,
				n.kind,
				target.kind,
			)
		}
		n.target = target
	}
	decoded := &prelinkDecoder{
		cache:  map[*prelinkNode]prelinkValue{},
		active: map[*prelinkNode]bool{},
	}
	value, err := decoded.value(root, 0)
	if err != nil {
		return nil, err
	}
	info, ok := value.value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: prelink root is not a dictionary", errInvalidPrelink)
	}
	return info, nil
}

func newPrelinkNode(
	start xml.StartElement,
	definitions map[uint64]*prelinkNode,
) (*prelinkNode, error) {
	if start.Name.Space != "" {
		return nil, fmt.Errorf("%w: prelink XML namespaces are unsupported", errInvalidPrelink)
	}
	n := &prelinkNode{kind: start.Name.Local}
	switch n.kind {
	case "plist", "dict", "array", "key", "string", "integer", "data", "true", "false":
	default:
		return nil, fmt.Errorf("%w: prelink unsupported type %q", errInvalidPrelink, n.kind)
	}
	var id uint64
	hasID := false
	for _, a := range start.Attr {
		if a.Name.Space != "" {
			return nil, fmt.Errorf(
				"%w: prelink namespaced attribute %q",
				errInvalidPrelink,
				a.Name.Local,
			)
		}
		switch a.Name.Local {
		case "ID", "IDREF":
			if n.kind == "plist" || n.kind == "key" {
				return nil, fmt.Errorf(
					"%w: prelink %s cannot have %s",
					errInvalidPrelink,
					n.kind,
					a.Name.Local,
				)
			}
			value, err := strconv.ParseUint(a.Value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf(
					"%w: prelink invalid %s %q: %w",
					errInvalidPrelink,
					a.Name.Local,
					a.Value,
					err,
				)
			}
			if a.Name.Local == "ID" {
				if hasID || n.isRef {
					return nil, fmt.Errorf("%w: prelink conflicting object IDs", errInvalidPrelink)
				}
				id, hasID = value, true
			} else {
				if hasID || n.isRef {
					return nil, fmt.Errorf("%w: prelink conflicting object IDs", errInvalidPrelink)
				}
				n.ref, n.isRef = value, true
			}
		case "size":
			if n.kind != "integer" || n.size != 0 || n.isRef {
				return nil, fmt.Errorf(
					"%w: prelink invalid size attribute on %s",
					errInvalidPrelink,
					n.kind,
				)
			}
			size, err := strconv.ParseUint(a.Value, 10, 64)
			if err != nil || (size != 8 && size != 16 && size != 32 && size != 64) {
				return nil, fmt.Errorf(
					"%w: prelink invalid integer size %q",
					errInvalidPrelink,
					a.Value,
				)
			}
			n.size = size
		case "version":
			if n.kind != "plist" || a.Value != "1.0" {
				return nil, fmt.Errorf(
					"%w: prelink invalid plist version %q",
					errInvalidPrelink,
					a.Value,
				)
			}
		default:
			return nil, fmt.Errorf(
				"%w: prelink unsupported %s attribute %q",
				errInvalidPrelink,
				n.kind,
				a.Name.Local,
			)
		}
	}
	if hasID {
		if _, exists := definitions[id]; exists {
			return nil, fmt.Errorf("%w: prelink duplicate ID %d", errInvalidPrelink, id)
		}
		definitions[id] = n
	}
	if n.isRef && n.size != 0 {
		return nil, fmt.Errorf("%w: prelink IDREF %d cannot have size", errInvalidPrelink, n.ref)
	}
	return n, nil
}

type prelinkValue struct {
	value  any
	weight int
}

type prelinkDecoder struct {
	cache  map[*prelinkNode]prelinkValue
	active map[*prelinkNode]bool
}

func (d *prelinkDecoder) value(n *prelinkNode, depth int) (prelinkValue, error) {
	if depth >= prelinkDepthLimit {
		return prelinkValue{}, fmt.Errorf("%w: prelink reference depth exceeded", errInvalidPrelink)
	}
	if n.isRef {
		if len(n.children) != 0 || len(bytes.TrimSpace(n.text)) != 0 {
			return prelinkValue{}, fmt.Errorf(
				"%w: prelink IDREF %d has content",
				errInvalidPrelink,
				n.ref,
			)
		}
		return d.value(n.target, depth+1)
	}
	if d.active[n] {
		return prelinkValue{}, fmt.Errorf("%w: prelink cyclic IDREF", errInvalidPrelink)
	}
	if v, ok := d.cache[n]; ok {
		return v, nil
	}
	d.active[n] = true
	defer delete(d.active, n)
	if len(n.children) != 0 && n.kind != "dict" && n.kind != "array" && n.kind != "plist" {
		return prelinkValue{}, fmt.Errorf(
			"%w: prelink %s has child objects",
			errInvalidPrelink,
			n.kind,
		)
	}
	var result prelinkValue
	switch n.kind {
	case "plist":
		if len(n.children) != 1 || len(bytes.TrimSpace(n.text)) != 0 {
			return prelinkValue{}, fmt.Errorf(
				"%w: prelink plist wrapper is malformed",
				errInvalidPrelink,
			)
		}
		return d.value(n.children[0], depth+1)
	case "dict":
		var err error
		result, err = d.dictionary(n, depth)
		if err != nil {
			return prelinkValue{}, err
		}
	case "array":
		var err error
		result, err = d.array(n, depth)
		if err != nil {
			return prelinkValue{}, err
		}
	case "string":
		result.value = string(n.text)
		result.weight = len(n.text) + 2
	case "integer":
		value, err := parsePrelinkInteger(n)
		if err != nil {
			return prelinkValue{}, err
		}
		result.value, result.weight = value, len(value)+2
	case "data":
		encoded := strings.Join(strings.Fields(string(n.text)), "")
		value, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return prelinkValue{}, fmt.Errorf(
				"%w: prelink invalid data: %w",
				errInvalidPrelink,
				err,
			)
		}
		result.value, result.weight = value, len(encoded)+2
	case "true", "false":
		if len(bytes.TrimSpace(n.text)) != 0 {
			return prelinkValue{}, fmt.Errorf("%w: prelink boolean has text", errInvalidPrelink)
		}
		result.value, result.weight = n.kind == "true", 5
	case "key":
		return prelinkValue{}, fmt.Errorf("%w: prelink key outside dictionary", errInvalidPrelink)
	}
	d.cache[n] = result
	return result, nil
}

func addPrelinkWeight(total *int, weight int) error {
	if weight > prelinkExpandedLimit-*total {
		return fmt.Errorf("%w: prelink expanded object exceeds 16 MiB", errInvalidPrelink)
	}
	*total += weight
	return nil
}

func (d *prelinkDecoder) dictionary(n *prelinkNode, depth int) (prelinkValue, error) {
	if len(bytes.TrimSpace(n.text)) != 0 || len(n.children)%2 != 0 {
		return prelinkValue{}, fmt.Errorf("%w: prelink dictionary is malformed", errInvalidPrelink)
	}
	m := make(map[string]any, len(n.children)/2)
	result := prelinkValue{value: m}
	for i := 0; i < len(n.children); i += 2 {
		key := n.children[i]
		if key.kind != "key" || key.isRef || len(key.children) != 0 {
			return prelinkValue{}, fmt.Errorf(
				"%w: prelink dictionary key is malformed",
				errInvalidPrelink,
			)
		}
		name := string(key.text)
		if _, exists := m[name]; exists {
			return prelinkValue{}, fmt.Errorf(
				"%w: prelink duplicate dictionary key %q",
				errInvalidPrelink,
				name,
			)
		}
		v, err := d.value(n.children[i+1], depth+1)
		if err != nil {
			return prelinkValue{}, err
		}
		if err = addPrelinkWeight(&result.weight, len(name)+v.weight+8); err != nil {
			return prelinkValue{}, err
		}
		m[name] = v.value
	}
	return result, nil
}

func (d *prelinkDecoder) array(n *prelinkNode, depth int) (prelinkValue, error) {
	if len(bytes.TrimSpace(n.text)) != 0 {
		return prelinkValue{}, fmt.Errorf("%w: prelink array has text", errInvalidPrelink)
	}
	values := make([]any, len(n.children))
	result := prelinkValue{value: values}
	for i, child := range n.children {
		v, err := d.value(child, depth+1)
		if err != nil {
			return prelinkValue{}, err
		}
		if err = addPrelinkWeight(&result.weight, v.weight+2); err != nil {
			return prelinkValue{}, err
		}
		values[i] = v.value
	}
	return result, nil
}

func parsePrelinkInteger(n *prelinkNode) (json.Number, error) {
	value := strings.TrimSpace(string(n.text))
	if value == "" {
		return "", fmt.Errorf("%w: prelink empty integer", errInvalidPrelink)
	}
	base := 10
	if strings.HasPrefix(value, "0x") || strings.HasPrefix(value, "0X") {
		base, value = 16, value[2:]
	}
	if strings.HasPrefix(value, "-") {
		if base != 10 {
			return "", fmt.Errorf(
				"%w: prelink invalid signed hexadecimal integer",
				errInvalidPrelink,
			)
		}
		signed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || (n.size != 0 && n.size < 64 && signed < -(int64(1)<<(n.size-1))) {
			return "", fmt.Errorf("%w: prelink invalid integer %q", errInvalidPrelink, value)
		}
		return json.Number(strconv.FormatInt(signed, 10)), nil
	}
	unsigned, err := strconv.ParseUint(value, base, 64)
	if err != nil || (n.size != 0 && n.size < 64 && unsigned >= uint64(1)<<n.size) {
		return "", fmt.Errorf("%w: prelink invalid integer %q", errInvalidPrelink, value)
	}
	return json.Number(strconv.FormatUint(unsigned, 10)), nil
}
