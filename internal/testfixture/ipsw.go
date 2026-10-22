// Package testfixture builds small, synthetic format fixtures without Apple assets.
package testfixture

import (
	"archive/zip"
	"bytes"
	"encoding/asn1"
	"encoding/binary"
	"fmt"
	"os"

	"howett.net/plist"
)

func IM4P(kind string, data []byte) []byte {
	b, err := asn1.Marshal(struct {
		Tag, Type, Version string
		Data               []byte
	}{"IM4P", kind, "synthetic", data})
	if err != nil {
		panic(err)
	}
	return b
}
func Tree(value uint64) []byte {
	var b bytes.Buffer
	binary.Write(&b, binary.LittleEndian, uint32(3))
	binary.Write(&b, binary.LittleEndian, uint32(0))
	prop := func(key string, data []byte) {
		var name [32]byte
		copy(name[:], key)
		b.Write(name[:])
		binary.Write(&b, binary.LittleEndian, uint32(len(data)))
		b.Write(data)
		for len(data)%4 != 0 {
			b.WriteByte(0)
			data = append(data, 0)
		}
	}
	prop("name", []byte("device-tree\x00"))
	prop("compatible", []byte("synthetic,board\x00"))
	var valueBytes [8]byte
	binary.LittleEndian.PutUint64(valueBytes[:], value)
	prop("test-value", valueBytes[:])
	return b.Bytes()
}
func Kernel() []byte {
	info, _ := plist.Marshal(map[string]any{"_PrelinkInfoDictionary": []any{map[string]any{"CFBundleIdentifier": "test.synthetic.driver", "CFBundleVersion": "1", "OSBundleLibraries": map[string]string{"com.apple.kpi.iokit": "1.0"}}}}, plist.XMLFormat)
	var b bytes.Buffer
	write := func(v any) {
		if err := binary.Write(&b, binary.LittleEndian, v); err != nil {
			panic(err)
		}
	}
	write([8]uint32{0xfeedfacf, 0x100000c, 0, 2, 1, 152, 0, 0})
	write(uint32(0x19))
	write(uint32(152))
	var segment [16]byte
	copy(segment[:], "__PRELINK_INFO")
	b.Write(segment[:])
	write([4]uint64{0x100000000, uint64(len(info)), 184, uint64(len(info))})
	write([4]uint32{7, 1, 1, 0})
	var section [16]byte
	copy(section[:], "__info")
	b.Write(section[:])
	b.Write(segment[:])
	write([2]uint64{0x100000000, uint64(len(info))})
	write([8]uint32{184, 0, 0, 0, 0, 0, 0, 0})
	b.Write(info)
	return b.Bytes()
}
func IPSW(path, version string, value uint64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	z := zip.NewWriter(f)
	component := func(path string) any { return map[string]any{"Info": map[string]any{"Path": path}} }
	identity := func(variant string) any {
		return map[string]any{"Info": map[string]any{"DeviceClass": "n104ap", "Variant": variant}, "Manifest": map[string]any{"DeviceTree": component("Firmware/tree.im4p"), "KernelCache": component("kernelcache")}}
	}
	manifest, err := plist.Marshal(map[string]any{"ProductVersion": version, "ProductBuildVersion": "synthetic-" + version, "SupportedProductTypes": []string{"iPhone12,1"}, "BuildIdentities": []any{identity("Erase"), identity("Update")}}, plist.XMLFormat)
	if err != nil {
		return err
	}
	for _, member := range []struct {
		name string
		data []byte
	}{{"BuildManifest.plist", manifest}, {"Firmware/tree.im4p", IM4P("dtre", Tree(value))}, {"kernelcache", IM4P("krnl", Kernel())}} {
		w, err := z.CreateHeader(&zip.FileHeader{Name: member.name, Method: zip.Store})
		if err != nil {
			return err
		}
		if _, err = w.Write(member.data); err != nil {
			return fmt.Errorf("fixture: %w", err)
		}
	}
	return z.Close()
}
