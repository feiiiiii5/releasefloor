package floor

import (
	"bytes"
	"crypto/sha256"
	"debug/elf"
	"debug/macho"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestInspectELFRecordedRequirements(t *testing.T) {
	data := fixtureELF([]fixtureNode{
		{Provider: "libc.so.6", Version: "GLIBC_2.9", Symbol: "old_api", WeakNode: false, WeakSymbol: false},
		{Provider: "libc.so.6", Version: "GLIBC_2.10", Symbol: "new_api", WeakNode: false, WeakSymbol: false},
		{Provider: "libstdc++.so.6", Version: "GLIBCXX_3.4.31", Symbol: "weak_cpp", WeakNode: true, WeakSymbol: true},
		{Provider: "libstdc++.so.6", Version: "CXXABI_1.3.13", Symbol: "cpp_abi", WeakNode: false, WeakSymbol: false},
	}, false)
	// Independent standard-library decoding checks the generated ELF/version
	// structures, including the weak node that ImportedSymbols deliberately omits.
	f, err := elf.NewFile(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	needs, err := f.DynamicVersionNeeds()
	if err != nil || len(needs) != 2 {
		t.Fatalf("fixture version needs: %#v, %v", needs, err)
	}
	syms, err := f.DynamicSymbols()
	if err != nil || len(syms) != 4 {
		t.Fatalf("fixture dynamic symbols: %#v, %v", syms, err)
	}
	bins, err := Inspect(data, "bin/tool")
	if err != nil {
		t.Fatal(err)
	}
	if len(bins) != 1 {
		t.Fatalf("binaries: %#v", bins)
	}
	b := bins[0]
	if b.Platform != "elf" || b.Arch != "amd64" || b.Kind != "shared-library" || b.Member != "bin/tool" {
		t.Fatalf("identity: %#v", b)
	}
	if b.SHA256 != fmt.Sprintf("%x", sha256.Sum256(data)) {
		t.Fatal("hash does not describe original member")
	}
	if !reflect.DeepEqual(b.Floors, map[string]string{"GLIBC": "2.10", "GLIBCXX": "3.4.31", "CXXABI": "1.3.13"}) {
		t.Fatalf("floors: %#v", b.Floors)
	}
	if len(b.Unknown) != 0 || len(b.Requirements) != 4 {
		t.Fatalf("requirements: %#v", b)
	}
	var weak *Requirement
	for i := range b.Requirements {
		if b.Requirements[i].Version == "GLIBCXX_3.4.31" {
			weak = &b.Requirements[i]
		}
	}
	if weak == nil || !weak.Weak || !reflect.DeepEqual(weak.Symbols, []string{"weak_cpp"}) {
		t.Fatalf("weak requirement lost: %#v", weak)
	}
	if !reflect.DeepEqual(b.Libraries, []string{"libc.so.6", "libstdc++.so.6"}) {
		t.Fatalf("libraries: %#v", b.Libraries)
	}
	again, err := Inspect(data, "bin/tool")
	if err != nil || !reflect.DeepEqual(bins, again) {
		t.Fatal("non-deterministic inspection")
	}
}

func TestInspectELFSpecialNodesAndRELR(t *testing.T) {
	data := fixtureELF([]fixtureNode{{Provider: "libc.so.6", Version: "GLIBC_ABI_DT_RELR", Symbol: "relocated", WeakNode: false, WeakSymbol: false}, {Provider: "libc.so.6", Version: "GLIBC_2.28", Symbol: "startup", WeakNode: false, WeakSymbol: false}, {Provider: "libc.so.6", Version: "GLIBC_PRIVATE", Symbol: "private_api", WeakNode: false, WeakSymbol: false}, {Provider: "libstdc++.so.6", Version: "CXXABI_TM_1", Symbol: "transaction", WeakNode: false, WeakSymbol: false}}, true)
	bins, err := Inspect(data, "tool")
	if err != nil {
		t.Fatal(err)
	}
	b := bins[0]
	if b.Floors["GLIBC"] != "2.36" || len(b.Unknown) != 3 || len(b.Requirements) != 4 {
		t.Fatalf("special nodes: %#v", b)
	}
	for _, r := range b.Requirements {
		if r.Version == "GLIBC_2.36" {
			t.Fatal("inferred floor fabricated as recorded node")
		}
	}
	nongnu := fixtureELF([]fixtureNode{{Provider: "libvendor.so", Version: "VENDOR_1.0", Symbol: "vendor_api", WeakNode: false, WeakSymbol: false}}, true)
	bins, err = Inspect(nongnu, "tool")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := bins[0].Floors["GLIBC"]; ok || len(bins[0].Unknown) != 1 {
		t.Fatalf("invented glibc dependency: %#v", bins)
	}
}

func TestInspectEndianAnd32BitHeaders(t *testing.T) {
	// A valid big-endian ARM ELF32 static image: one load segment and the
	// mandatory null section. No dynamic requirements means an empty map,
	// never a fabricated glibc version of zero.
	data := make([]byte, 52+32+40)
	bo := binary.BigEndian
	copy(data, []byte{0x7f, 'E', 'L', 'F', 1, 2, 1})
	bo.PutUint16(data[16:], 2)
	bo.PutUint16(data[18:], uint16(elf.EM_ARM))
	bo.PutUint32(data[20:], 1)
	bo.PutUint32(data[24:], 0x10000)
	bo.PutUint32(data[28:], 52)
	bo.PutUint32(data[32:], 84)
	bo.PutUint16(data[40:], 52)
	bo.PutUint16(data[42:], 32)
	bo.PutUint16(data[44:], 1)
	bo.PutUint16(data[46:], 40)
	bo.PutUint16(data[48:], 1)
	for i, n := range []uint32{1, 0, 0x10000, 0x10000, uint32(len(data)), uint32(len(data)), 5, 4096} {
		bo.PutUint32(data[52+i*4:], n)
	}
	if _, err := elf.NewFile(bytes.NewReader(data)); err != nil {
		t.Fatalf("ELF32 fixture: %v", err)
	}
	bins, err := Inspect(data, "arm")
	if err != nil || bins[0].Arch != "arm-be" || len(bins[0].Floors) != 0 || len(bins[0].Unknown) != 0 {
		t.Fatalf("ELF32: %#v %v", bins, err)
	}
	// All words in this no-string fixture can be byte-swapped independently.
	le := fixtureMach(0x1000007, 3, 0x0a0f00, 0x0f0000, false)
	be := make([]byte, len(le))
	for i := 0; i < len(le); i += 4 {
		bo.PutUint32(be[i:], binary.LittleEndian.Uint32(le[i:]))
	}
	if _, err := macho.NewFile(bytes.NewReader(be)); err != nil {
		t.Fatalf("big-endian Mach-O fixture: %v", err)
	}
	bins, err = Inspect(be, "big")
	if err != nil || bins[0].MinimumOS != "10.15.0" {
		t.Fatalf("big-endian Mach-O: %#v %v", bins, err)
	}
	thin32 := append([]byte(nil), le[:28]...)
	thin32 = append(thin32, le[32:]...)
	binary.LittleEndian.PutUint32(thin32, 0xfeedface)
	binary.LittleEndian.PutUint32(thin32[4:], 7)
	if _, err := macho.NewFile(bytes.NewReader(thin32)); err != nil {
		t.Fatalf("Mach-O32 fixture: %v", err)
	}
	bins, err = Inspect(thin32, "i386")
	if err != nil || bins[0].Arch != "386" {
		t.Fatalf("Mach-O32: %#v %v", bins, err)
	}
}

func TestInspectMachDeploymentAndDependencyKinds(t *testing.T) {
	var commands [][]byte
	for i, cmd := range []uint32{0xc, 0x80000018, 0x8000001f, 0x20, 0x80000023} {
		commands = append(commands, fixtureDylib(cmd, fmt.Sprintf("@rpath/lib%d.dylib", i)))
	}
	data := fixtureMach(0x100000c, 0, 0x0b0201, 0x0f0000, false, commands...)
	if _, err := macho.NewFile(bytes.NewReader(data)); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	bins, err := Inspect(data, "app")
	if err != nil {
		t.Fatal(err)
	}
	b := bins[0]
	if b.Platform != "macos" || b.Arch != "arm64" || b.MinimumOS != "11.2.1" || b.SDK != "15.0.0" || len(b.Unknown) != 0 || len(b.Libraries) != 5 {
		t.Fatalf("Mach-O: %#v", b)
	}
	if len(b.Requirements) != 5 || !b.Requirements[1].Weak || b.Requirements[1].Version != "2.1.0" {
		t.Fatalf("Mach-O dependency metadata: %#v", b.Requirements)
	}
	legacy := fixtureMach(0x1000007, 3, 0x0a0f00, 0x0b0100, true)
	bins, err = Inspect(legacy, "app")
	if err != nil || bins[0].MinimumOS != "10.15.0" {
		t.Fatalf("legacy: %#v %v", bins, err)
	}
}

func TestInspectUniversalSlicesAndSubtypes(t *testing.T) {
	x86 := fixtureMach(0x1000007, 8, 0x0a0f00, 0x0f0000, false)
	arm := fixtureMach(0x100000c, 2, 0x0b0000, 0x0f0000, false)
	data := fixtureFat(arm, x86)
	if f, err := macho.NewFatFile(bytes.NewReader(data)); err != nil || len(f.Arches) != 2 {
		t.Fatalf("universal fixture: %#v %v", f, err)
	}
	bins, err := Inspect(data, "app")
	if err != nil {
		t.Fatal(err)
	}
	if len(bins) != 2 || bins[0].Arch != "amd64-subtype-00000008" || bins[1].Arch != "arm64-subtype-00000002" || bins[0].MinimumOS != "10.15.0" || bins[1].MinimumOS != "11.0.0" {
		t.Fatalf("universal: %#v", bins)
	}
	for _, b := range bins {
		if b.SHA256 != fmt.Sprintf("%x", sha256.Sum256(data)) || b.Member != "app" {
			t.Fatal("slice lost original member identity")
		}
	}
	_, err = Inspect(fixtureFat(arm, arm), "app")
	if err == nil {
		t.Fatal("duplicate universal architecture accepted")
	}
}

func TestInspectRejectsMalformedAndIncomplete(t *testing.T) {
	elfdata := fixtureELF([]fixtureNode{{Provider: "libc.so.6", Version: "GLIBC_2.28", Symbol: "api", WeakNode: false, WeakSymbol: false}}, false)
	mach := fixtureMach(0x100000c, 0, 0x0b0000, 0x0f0000, false)
	mutate := func(data []byte, f func([]byte)) []byte { out := append([]byte(nil), data...); f(out); return out }
	tests := map[string][]byte{
		"truncated-elf":           []byte("\x7fELF"),
		"invalid-elf-count":       mutate(elfdata, func(b []byte) { binary.LittleEndian.PutUint16(b[60:], 65535) }),
		"overflow-section-offset": mutate(elfdata, func(b []byte) { binary.LittleEndian.PutUint64(b[40:], ^uint64(0)) }),
		"sectionless":             mutate(elfdata, func(b []byte) { binary.LittleEndian.PutUint16(b[60:], 0) }),
		"bad-version-next": mutate(elfdata, func(b []byte) {
			f, _ := elf.NewFile(bytes.NewReader(b))
			s := f.SectionByType(elf.SHT_GNU_VERNEED)
			binary.LittleEndian.PutUint32(b[s.Offset+12:], 4)
		}),
		"mismatched-dynamic-address": mutate(elfdata, func(b []byte) {
			f, _ := elf.NewFile(bytes.NewReader(b))
			s := f.SectionByType(elf.SHT_DYNAMIC)
			binary.LittleEndian.PutUint64(b[s.Offset+16+8:], 1)
		}),
		"missing-dynamic-terminator": mutate(elfdata, func(b []byte) {
			f, _ := elf.NewFile(bytes.NewReader(b))
			s := f.SectionByType(elf.SHT_DYNAMIC)
			binary.LittleEndian.PutUint64(b[s.Offset+s.Size-16:], 1)
		}),
		"truncated-mach":        mach[:31],
		"excessive-mach-count":  mutate(mach, func(b []byte) { binary.LittleEndian.PutUint32(b[16:], ^uint32(0)) }),
		"bad-mach-command-size": mutate(mach, func(b []byte) { binary.LittleEndian.PutUint32(b[36:], ^uint32(0)) }),
		"excessive-build-tools": mutate(mach, func(b []byte) { binary.LittleEndian.PutUint32(b[52:], ^uint32(0)) }),
		"fat64":                 {0xca, 0xfe, 0xba, 0xbf, 0, 0, 0, 1},
		"fat-truncated":         {0xca, 0xfe, 0xba, 0xbe, 0, 0, 0, 1},
		"fat-excessive":         {0xca, 0xfe, 0xba, 0xbe, 0xff, 0xff, 0xff, 0xff},
	}
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Inspect(data, "input"); err == nil {
				t.Fatal("malformed or unsupported input accepted")
			}
		})
	}
	missing := append([]byte(nil), mach[:32]...)
	binary.LittleEndian.PutUint32(missing[16:], 0)
	binary.LittleEndian.PutUint32(missing[20:], 0)
	bins, err := Inspect(missing, "input")
	if err != nil || len(bins[0].Unknown) != 1 {
		t.Fatalf("missing deployment metadata must stay unknown: %#v %v", bins, err)
	}
	if _, err := Inspect([]byte("ordinary document"), "readme"); !errors.Is(err, ErrNotBinary) {
		t.Fatalf("document: %v", err)
	}
	oversize := make([]byte, maxBinaryBytes+1)
	copy(oversize, "\x7fELF")
	if _, err := Inspect(oversize, "large"); err == nil || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("oversize: %v", err)
	}
}

func TestInspectSingleByteMutationsDoNotPanic(t *testing.T) {
	fixtures := [][]byte{fixtureELF([]fixtureNode{{Provider: "libc.so.6", Version: "GLIBC_2.28", Symbol: "api", WeakNode: false, WeakSymbol: false}}, false), fixtureFat(fixtureMach(0x100000c, 0, 0x0b0000, 0x0f0000, false))}
	for _, seed := range fixtures {
		for i := range seed {
			data := append([]byte(nil), seed...)
			data[i] ^= 0xff
			Inspect(data, "mutated")
		}
	}
}

func TestInspectEveryFixturePrefixFailsCleanly(t *testing.T) {
	fixtures := [][]byte{fixtureELF([]fixtureNode{{Provider: "libc.so.6", Version: "GLIBC_2.28", Symbol: "api", WeakNode: false, WeakSymbol: false}}, false), fixtureMach(0x100000c, 0, 0x0b0000, 0x0f0000, false)}
	for _, data := range fixtures {
		for n := 4; n < len(data); n++ {
			if bins, err := Inspect(data[:n], "truncated"); err == nil && len(bins) > 0 {
				t.Fatalf("truncated prefix %d/%d passed", n, len(data))
			}
		}
	}
}
