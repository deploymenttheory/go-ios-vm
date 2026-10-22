package inspection

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPrelinkSharedReferencesAndExactInteger(t *testing.T) {
	info, err := parsePrelink([]byte(`<dict>
		<key>first</key><dict ID="1"><key>value</key><integer ID="2" size="64">0xffffffffffffffff</integer></dict>
		<key>second</key><dict IDREF="1"/>
		<key>number</key><integer IDREF="2"/>
	</dict>`))
	if err != nil {
		t.Fatal(err)
	}
	first := info["first"].(map[string]any)
	second := info["second"].(map[string]any)
	first["shared"] = true
	if second["shared"] != true {
		t.Fatal("IDREF did not preserve the shared dictionary")
	}
	if info["number"] != json.Number("18446744073709551615") || first["value"] != info["number"] {
		t.Fatalf("integer reference was rounded or changed: %#v", info["number"])
	}
	encoded, err := json.Marshal(exactJSON(info["number"]))
	if err != nil || string(encoded) != `{"decimal":"18446744073709551615","type":"number"}` {
		t.Fatalf("exact JSON: %s, %v", encoded, err)
	}
}

func TestPrelinkRejectsInvalidReferencesAndCycles(t *testing.T) {
	for _, tc := range []struct {
		name string
		xml  string
		want string
	}{
		{"dangling", `<array><integer IDREF="1"/></array>`, "dangling IDREF"},
		{"duplicate", `<array><integer ID="1">1</integer><integer ID="1">2</integer></array>`, "duplicate ID"},
		{"type", `<array><string ID="1">x</string><integer IDREF="1"/></array>`, "has type"},
		{"content", `<array><integer ID="1">1</integer><integer IDREF="1">2</integer></array>`, "has content"},
		{"cycle", `<dict ID="1"><key>self</key><dict IDREF="1"/></dict>`, "cyclic IDREF"},
		{"overflow", `<dict><key>x</key><integer size="64">0x10000000000000000</integer></dict>`, "invalid integer"},
		{"invalid type", `<dict><key>x</key><real>1.0</real></dict>`, "unsupported type"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parsePrelink([]byte(tc.xml))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v; want %q", err, tc.want)
			}
		})
	}
}

func TestPrelinkAcceptsForwardReferenceAndRejectsExcessiveDepth(t *testing.T) {
	info, err := parsePrelink([]byte(`<dict><key>first</key><string IDREF="7"/><key>later</key><string ID="7">found</string></dict>`))
	if err != nil || info["first"] != "found" {
		t.Fatalf("forward reference: %#v, %v", info, err)
	}
	deep := strings.Repeat("<array>", prelinkDepthLimit+1) + `<string>x</string>` + strings.Repeat("</array>", prelinkDepthLimit+1)
	if _, err := parsePrelink([]byte(deep)); err == nil || !strings.Contains(err.Error(), "object limit") {
		t.Fatalf("deep object error = %v", err)
	}
	var expanded strings.Builder
	expanded.WriteString(`<dict><key>x</key><array><string ID="1">`)
	expanded.WriteString(strings.Repeat("a", 8192))
	expanded.WriteString(`</string>`)
	for range 3000 {
		expanded.WriteString(`<string IDREF="1"/>`)
	}
	expanded.WriteString(`</array></dict>`)
	if _, err := parsePrelink([]byte(expanded.String())); err == nil || !strings.Contains(err.Error(), "expanded object exceeds") {
		t.Fatalf("expanded object error = %v", err)
	}
}
