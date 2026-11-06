package floor

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These bytes are an original minimal MH_EXECUTE, independent of other tests.
func scanTestMachO(build bool) []byte {
	b := make([]byte, 32)
	le := binary.LittleEndian
	le.PutUint32(b[0:], 0xfeedfacf)
	le.PutUint32(b[4:], 0x01000007) // x86_64
	le.PutUint32(b[8:], 3)
	le.PutUint32(b[12:], 2) // MH_EXECUTE
	if build {
		le.PutUint32(b[16:], 1)
		le.PutUint32(b[20:], 24)
		cmd := make([]byte, 24)
		le.PutUint32(cmd[0:], 0x32) // LC_BUILD_VERSION
		le.PutUint32(cmd[4:], 24)
		le.PutUint32(cmd[8:], 1) // macOS
		le.PutUint32(cmd[12:], 12<<16)
		le.PutUint32(cmd[16:], 13<<16)
		b = append(b, cmd...)
	}
	return b
}

type scanTestEntry struct {
	name string
	data []byte
	mode os.FileMode
	typ  byte
	link string
}

func scanTestZip(t *testing.T, entries ...scanTestEntry) []byte {
	t.Helper()
	var out bytes.Buffer
	w := zip.NewWriter(&out)
	for _, e := range entries {
		h := &zip.FileHeader{Name: e.name, Method: zip.Store}
		mode := e.mode
		if mode == 0 {
			mode = 0644
		}
		h.SetMode(mode)
		f, err := w.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(e.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func scanTestTarGzip(t *testing.T, entries ...scanTestEntry) []byte {
	t.Helper()
	var out bytes.Buffer
	g := gzip.NewWriter(&out)
	w := tar.NewWriter(g)
	for _, e := range entries {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		h := &tar.Header{Name: e.name, Mode: 0644, Size: int64(len(e.data)), Typeflag: typ, Linkname: e.link}
		if err := w.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(e.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := g.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func scanTestRun(t *testing.T, name string, data []byte, limits Limits) (Inventory, error) {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	return Scan(p, limits)
}

func scanTestHasIssue(inv Inventory, code string) bool {
	for _, issue := range inv.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func TestScanRawStableIdentityAndHashes(t *testing.T) {
	b := scanTestMachO(true)
	inv, err := scanTestRun(t, "renamed.release", b, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if !inv.Complete || len(inv.Binaries) != 1 || inv.Binaries[0].Member != "binary" {
		t.Fatalf("bad inventory: %+v", inv)
	}
	h := sha256.Sum256(b)
	want := hex.EncodeToString(h[:])
	if inv.SHA256 != want || inv.Binaries[0].SHA256 != want {
		t.Fatal("hashes do not describe original bytes")
	}
	if inv.Artifact != "renamed.release" || inv.Schema != InventorySchema || inv.Files != 1 {
		t.Fatalf("bad artifact metadata: %+v", inv)
	}
	if inv.Binaries[0].MinimumOS != "12.0.0" && inv.Binaries[0].MinimumOS != "12.0" && inv.Binaries[0].MinimumOS != "12" {
		t.Fatalf("lost minimum OS: %+v", inv.Binaries[0])
	}
}

func TestScanArchivesInspectMagicAndSort(t *testing.T) {
	entries := []scanTestEntry{{name: "pkg/z.dat", data: scanTestMachO(true)}, {name: "pkg/README", data: []byte("documentation")}, {name: "pkg/a.txt", data: scanTestMachO(true)}}
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"release.zip", scanTestZip(t, entries...)},
		{"release.tgz", scanTestTarGzip(t, entries...)},
		{"no-extension", scanTestTarGzip(t, entries...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inv, err := scanTestRun(t, tc.name, tc.data, Limits{StripComponents: 1})
			if err != nil {
				t.Fatal(err)
			}
			if !inv.Complete || inv.Files != 3 || inv.Ignored != 1 || len(inv.Binaries) != 2 {
				t.Fatalf("bad coverage: %+v", inv)
			}
			if inv.Binaries[0].Member != "a.txt" || inv.Binaries[1].Member != "z.dat" {
				t.Fatalf("members not sorted/normalized: %+v", inv.Binaries)
			}
		})
	}
}

func TestScanCoverageGapsBlock(t *testing.T) {
	nested := scanTestZip(t)
	for _, tc := range []struct {
		name  string
		entry scanTestEntry
		code  string
	}{
		{"pe", scanTestEntry{name: "windows.dat", data: []byte("MZunsupported")}, "unsupported_binary"},
		{"wasm", scanTestEntry{name: "module", data: []byte{0, 'a', 's', 'm', 1, 0, 0, 0}}, "unsupported_binary"},
		{"nested", scanTestEntry{name: "payload", data: nested}, "nested_archive"},
		{"symlink", scanTestEntry{name: "link", data: []byte("target"), mode: os.ModeSymlink | 0777}, "symlink_not_followed"},
		{"unknown", scanTestEntry{name: "unknown", data: scanTestMachO(false)}, "unknown_metadata"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := scanTestZip(t, scanTestEntry{name: "tool", data: scanTestMachO(true)}, tc.entry)
			inv, err := scanTestRun(t, "release.zip", data, Limits{})
			if err != nil {
				t.Fatal(err)
			}
			if inv.Complete || !scanTestHasIssue(inv, tc.code) {
				t.Fatalf("coverage gap passed: %+v", inv)
			}
		})
	}
}

func TestScanNoSupportedBinaryBlocks(t *testing.T) {
	for _, data := range [][]byte{[]byte("only documentation"), scanTestZip(t), scanTestTarGzip(t)} {
		inv, err := scanTestRun(t, "artifact", data, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		if inv.Complete || !scanTestHasIssue(inv, "no_binaries") {
			t.Fatalf("empty scope passed: %+v", inv)
		}
	}
}

func TestScanRejectsArchivePaths(t *testing.T) {
	for _, name := range []string{"../outside", "safe/../outside", "/absolute", `C:\outside`, `..\outside`, "bad\x00name"} {
		t.Run(strings.ReplaceAll(name, "/", "_"), func(t *testing.T) {
			inv, err := scanTestRun(t, "release.zip", scanTestZip(t, scanTestEntry{name: name, data: scanTestMachO(true)}), Limits{})
			if err == nil || inv.Complete {
				t.Fatalf("unsafe path accepted: %q %+v", name, inv)
			}
		})
	}
	for _, names := range [][2]string{{"a/tool", "a/./tool"}, {"a//tool", "a/tool"}, {`a\tool`, "a/tool"}, {"v1/tool", "v2/tool"}} {
		limits := Limits{}
		if names[0] == "v1/tool" {
			limits.StripComponents = 1
		}
		inv, err := scanTestRun(t, "release.zip", scanTestZip(t, scanTestEntry{name: names[0], data: scanTestMachO(true)}, scanTestEntry{name: names[1], data: scanTestMachO(true)}), limits)
		if err == nil || inv.Complete {
			t.Fatalf("duplicate normalized path accepted: %+v", names)
		}
	}
	data := scanTestZip(t, scanTestEntry{name: "root/", mode: os.ModeDir | 0755}, scanTestEntry{name: "root/./", mode: os.ModeDir | 0755})
	inv, err := scanTestRun(t, "release.zip", data, Limits{StripComponents: 1})
	if err == nil || inv.Complete {
		t.Fatal("duplicate directory hidden by stripping")
	}
}

func TestScanStripComponentsIsExplicit(t *testing.T) {
	data := scanTestZip(t, scanTestEntry{name: "version-1/tool", data: scanTestMachO(true)})
	inv, err := scanTestRun(t, "release.zip", data, Limits{})
	if err != nil || inv.Binaries[0].Member != "version-1/tool" {
		t.Fatalf("guessed prefix: %+v %v", inv, err)
	}
	for _, strip := range []int{-1, 2} {
		inv, err := scanTestRun(t, "release.zip", data, Limits{StripComponents: strip})
		if err == nil || inv.Complete {
			t.Fatalf("invalid strip accepted: %d", strip)
		}
	}
}

func TestScanResourceLimits(t *testing.T) {
	b := scanTestMachO(true)
	data := scanTestZip(t, scanTestEntry{name: "a", data: b}, scanTestEntry{name: "b", data: b})
	for _, tc := range []struct {
		name   string
		limits Limits
	}{
		{"input", Limits{MaxInputBytes: int64(len(data) - 1)}},
		{"member", Limits{MaxMemberBytes: int64(len(b) - 1)}},
		{"total", Limits{MaxTotalBytes: int64(len(b)*2 - 1)}},
		{"count", Limits{MaxFiles: 1}},
		{"negative", Limits{MaxInputBytes: -1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inv, err := scanTestRun(t, "release.zip", data, tc.limits)
			if err == nil || inv.Complete {
				t.Fatalf("limit bypassed: %+v", inv)
			}
		})
	}
	for _, limits := range []Limits{{MaxMemberBytes: int64(len(b) - 1)}, {MaxTotalBytes: int64(len(b) - 1)}} {
		inv, err := scanTestRun(t, "raw", b, limits)
		if err == nil || inv.Complete {
			t.Fatal("raw size limit bypassed")
		}
	}
	inv, err := scanTestRun(t, "release.zip", data, Limits{MaxInputBytes: int64(len(data)), MaxMemberBytes: int64(len(b)), MaxTotalBytes: int64(len(b) * 2), MaxFiles: 2})
	if err != nil || !inv.Complete {
		t.Fatalf("exact resource boundary rejected: %+v %v", inv, err)
	}
}

func TestScanZipIntegrity(t *testing.T) {
	original := scanTestZip(t, scanTestEntry{name: "tool", data: scanTestMachO(true)})
	crcBad := bytes.Clone(original)
	offset := bytes.Index(crcBad, scanTestMachO(true))
	if offset < 0 {
		t.Fatal("test ZIP payload missing")
	}
	crcBad[offset+10] ^= 1
	encrypted := bytes.Clone(original)
	central := bytes.Index(encrypted, []byte("PK\x01\x02"))
	if central < 0 {
		t.Fatal("central directory missing")
	}
	encrypted[central+8] |= 1
	encrypted[6] |= 1
	for _, data := range [][]byte{crcBad, encrypted, original[:len(original)-5], []byte("not a ZIP")} {
		inv, err := scanTestRun(t, "release.zip", data, Limits{})
		if err == nil || inv.Complete {
			t.Fatalf("invalid ZIP passed: %+v", inv)
		}
	}
}

func TestScanZipDirectoryBoundsBeforeParsing(t *testing.T) {
	original := scanTestZip(t, scanTestEntry{name: "a", data: scanTestMachO(true)}, scanTestEntry{name: "b", data: scanTestMachO(true)})
	falseCount := bytes.Clone(original)
	end := len(falseCount) - 22
	binary.LittleEndian.PutUint16(falseCount[end+8:], 1)
	binary.LittleEndian.PutUint16(falseCount[end+10:], 1)
	inv, err := scanTestRun(t, "release.zip", falseCount, Limits{MaxFiles: 1})
	if err == nil || inv.Complete {
		t.Fatal("false declared count bypassed actual-record bound")
	}
	trailing := append(bytes.Clone(original), 0)
	inv, err = scanTestRun(t, "release.zip", trailing, Limits{})
	if err == nil || inv.Complete {
		t.Fatal("unaccounted trailing ZIP bytes accepted")
	}
}

func TestScanZip64Directory(t *testing.T) {
	original := scanTestZip(t, scanTestEntry{name: "tool", data: scanTestMachO(true)})
	le := binary.LittleEndian
	end := len(original) - 22
	oldEnd := bytes.Clone(original[end:])
	size, off := le.Uint32(oldEnd[12:]), le.Uint32(oldEnd[16:])
	record := make([]byte, 56)
	copy(record, "PK\x06\x06")
	le.PutUint64(record[4:], 44)
	le.PutUint16(record[12:], 45)
	le.PutUint16(record[14:], 45)
	le.PutUint64(record[24:], 1)
	le.PutUint64(record[32:], 1)
	le.PutUint64(record[40:], uint64(size))
	le.PutUint64(record[48:], uint64(off))
	locator := make([]byte, 20)
	copy(locator, "PK\x06\x07")
	le.PutUint64(locator[8:], uint64(end))
	le.PutUint32(locator[16:], 1)
	le.PutUint16(oldEnd[8:], 0xffff)
	le.PutUint16(oldEnd[10:], 0xffff)
	le.PutUint32(oldEnd[12:], 0xffffffff)
	le.PutUint32(oldEnd[16:], 0xffffffff)
	data := append(bytes.Clone(original[:end]), record...)
	data = append(data, locator...)
	data = append(data, oldEnd...)
	inv, err := scanTestRun(t, "release.zip", data, Limits{})
	if err != nil || !inv.Complete || len(inv.Binaries) != 1 {
		t.Fatalf("valid ZIP64 rejected: %+v %v", inv, err)
	}
}

func TestScanTarIntegrityAndLinks(t *testing.T) {
	original := scanTestTarGzip(t, scanTestEntry{name: "tool", data: scanTestMachO(true)})
	crcBad := bytes.Clone(original)
	crcBad[len(crcBad)-8] ^= 1
	var tarBytes []byte
	r, err := gzip.NewReader(bytes.NewReader(original))
	if err != nil {
		t.Fatal(err)
	}
	tarBytes, err = io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	compress := func(data []byte) []byte {
		var out bytes.Buffer
		g := gzip.NewWriter(&out)
		if _, err := g.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := g.Close(); err != nil {
			t.Fatal(err)
		}
		return out.Bytes()
	}
	for _, data := range [][]byte{crcBad, original[:len(original)-5], compress(tarBytes[:len(tarBytes)-1024]), compress(append(bytes.Clone(tarBytes), []byte("hidden trailing data")...)), append(bytes.Clone(original), original...)} {
		inv, err := scanTestRun(t, "release.tgz", data, Limits{})
		if err == nil || inv.Complete {
			t.Fatalf("invalid tar.gz passed: %+v", inv)
		}
	}
	zeroPayload := scanTestTarGzip(t, scanTestEntry{name: "zero-data", data: make([]byte, 1024)})
	zeroReader, err := gzip.NewReader(bytes.NewReader(zeroPayload))
	if err != nil {
		t.Fatal(err)
	}
	zeroTar, err := io.ReadAll(zeroReader)
	if err != nil {
		t.Fatal(err)
	}
	if err := zeroReader.Close(); err != nil {
		t.Fatal(err)
	}
	inv, err := scanTestRun(t, "release.tgz", compress(zeroTar[:len(zeroTar)-1024]), Limits{})
	if err == nil || inv.Complete {
		t.Fatal("zero payload was mistaken for a tar end marker")
	}
	for _, typ := range []byte{tar.TypeSymlink, tar.TypeLink} {
		data := scanTestTarGzip(t, scanTestEntry{name: "tool", data: scanTestMachO(true)}, scanTestEntry{name: "link", typ: typ, link: "../../outside"})
		inv, err := scanTestRun(t, "release.tgz", data, Limits{})
		if err != nil || inv.Complete || !scanTestHasIssue(inv, "symlink_not_followed") {
			t.Fatalf("link coverage lost: %+v %v", inv, err)
		}
	}
}

func TestScanTarLimitsAndDuplicatePaths(t *testing.T) {
	b := scanTestMachO(true)
	data := scanTestTarGzip(t, scanTestEntry{name: "a", data: b}, scanTestEntry{name: "b", data: b})
	for _, limits := range []Limits{{MaxMemberBytes: int64(len(b) - 1)}, {MaxTotalBytes: int64(len(b)*2 - 1)}, {MaxFiles: 1}} {
		inv, err := scanTestRun(t, "release.tar.gz", data, limits)
		if err == nil || inv.Complete {
			t.Fatal("tar limit bypassed")
		}
	}
	for _, entries := range [][]scanTestEntry{
		{{name: "../outside", data: b}},
		{{name: "a/tool", data: b}, {name: "a/./tool", data: b}},
	} {
		inv, err := scanTestRun(t, "release.tar.gz", scanTestTarGzip(t, entries...), Limits{})
		if err == nil || inv.Complete {
			t.Fatal("tar unsafe/duplicate path accepted")
		}
	}
}

func TestScanOutputReproducibleAndNoAbsolutePaths(t *testing.T) {
	data := scanTestZip(t, scanTestEntry{name: "z", data: []byte("MZ")}, scanTestEntry{name: "b", data: scanTestMachO(true)}, scanTestEntry{name: "a", data: []byte("MZ")})
	one, err := scanTestRun(t, "release.zip", data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	two, err := scanTestRun(t, "release.zip", data, Limits{})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(one)
	b, _ := json.Marshal(two)
	if !bytes.Equal(a, b) {
		t.Fatal("same artifact produced different inventories")
	}
	if strings.Contains(string(a), t.TempDir()) {
		t.Fatal("absolute input path leaked")
	}
	if len(one.Issues) != 2 || one.Issues[0].Member != "a" || one.Issues[1].Member != "z" {
		t.Fatalf("issues not sorted: %+v", one.Issues)
	}
}

func TestScanRejectsFilesystemSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	if err := os.WriteFile(target, scanTestMachO(true), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	inv, err := Scan(link, Limits{})
	if err == nil || inv.Complete {
		t.Fatal("filesystem symlink followed")
	}
}
