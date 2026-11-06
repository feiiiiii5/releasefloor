// This command creates original object-format metadata, not runnable programs.
package main

import (
	"archive/zip"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/feiiiiii5/releasefloor/floor"
	"github.com/feiiiiii5/releasefloor/internal/testformat"
)

func main() {
	out := flag.String("out", "output/demo", "new directory for synthetic examples")
	flag.Parse()
	if flag.NArg() != 0 {
		panic("unexpected argument")
	}
	if _, err := os.Lstat(*out); !os.IsNotExist(err) {
		panic("output directory must be new")
	}
	if err := os.MkdirAll(*out, 0700); err != nil {
		panic(err)
	}
	write := func(name string, content []byte) {
		if err := os.WriteFile(filepath.Join(*out, name), content, 0600); err != nil {
			panic(err)
		}
	}
	elf := func(version string) []byte {
		data := testformat.ELF([]testformat.Node{{Provider: "libc.so.6", Version: "GLIBC_" + version, Symbol: "example_import"}}, false)
		binary.LittleEndian.PutUint16(data[16:], 2)
		return data
	}
	plugin := func(version string) []byte {
		return testformat.ELF([]testformat.Node{{Provider: "libstdc++.so.6", Version: "GLIBCXX_" + version, Symbol: "example_cpp_import"}, {Provider: "libstdc++.so.6", Version: "CXXABI_1.3.11", Symbol: "example_abi_import"}}, false)
	}
	mac := func(cpu, subtype, minimum uint32) []byte {
		return testformat.Mach(cpu, subtype, minimum<<16, 26<<16, false)
	}
	old := map[string][]byte{
		"linux-amd64/tool":      elf("2.31"),
		"darwin-universal/tool": testformat.Fat(mac(0x01000007, 3, 12), mac(0x0100000c, 0, 12)),
		"plugins/libhelper.so":  plugin("3.4.26"),
		"README.txt":            []byte("Original synthetic object-format metadata. Not runnable programs.\n"),
	}
	current := map[string][]byte{
		"linux-amd64/tool":      elf("2.38"),
		"darwin-universal/tool": testformat.Fat(mac(0x01000007, 3, 14)),
		"plugins/libhelper.so":  plugin("3.4.29"),
		"README.txt":            old["README.txt"],
	}
	archive := func(name string, members map[string][]byte) {
		f, err := os.OpenFile(filepath.Join(*out, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			panic(err)
		}
		z := zip.NewWriter(f)
		keys := make([]string, 0, len(members))
		for k := range members {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, key := range keys {
			w, err := z.Create(key)
			if err != nil {
				panic(err)
			}
			if _, err = w.Write(members[key]); err != nil {
				panic(err)
			}
		}
		if err = z.Close(); err != nil {
			panic(err)
		}
		if err = f.Close(); err != nil {
			panic(err)
		}
	}
	archive("old.zip", old)
	archive("new.zip", current)
	policy := floor.Policy{Schema: floor.PolicySchema, Rules: []floor.Rule{
		{Platform: "elf", Arch: "amd64", MaxFloors: map[string]string{"GLIBC": "2.35", "GLIBCXX": "3.4.26", "CXXABI": "1.3.11"}},
		{Platform: "macos", Arch: "amd64", MaxOS: "12.0"},
		{Platform: "macos", Arch: "arm64", MaxOS: "12.0"},
	}}
	data, err := json.MarshalIndent(policy, "", "  ")
	if err != nil {
		panic(err)
	}
	write("policy.json", append(data, '\n'))
	write("linux-amd64", old["linux-amd64/tool"])
	write("macos-arm64", mac(0x0100000c, 0, 12))
	write("ABOUT.txt", []byte("These original synthetic metadata fixtures are NOT runnable programs. They demonstrate raised GLIBC/GLIBCXX requirements, raised macOS minimum OS, a removed universal architecture, and policy checking. They contain no third-party code.\n"))
	fmt.Println("Created original metadata fixtures in", *out)
}
