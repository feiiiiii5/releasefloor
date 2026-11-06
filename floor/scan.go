package floor

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// Scan reads an artifact without extracting or executing its members. Coverage
// gaps are inventory issues; malformed inputs and resource-limit failures also
// return an error. Even an error result retains the evidence collected so far.
func Scan(filename string, limits Limits) (inv Inventory, err error) {
	inv = Inventory{Schema: InventorySchema, Artifact: filepath.Base(filename), Binaries: []Binary{}, Issues: []Issue{}, Complete: true}
	defer func() {
		if err != nil {
			inv.Complete = false
			inv.Issues = append(inv.Issues, Issue{Code: "invalid_artifact", Severity: "error", Detail: err.Error()})
		}
		if len(inv.Binaries) == 0 {
			inv.Complete = false
			inv.Issues = append(inv.Issues, Issue{Code: "no_binaries", Severity: "error", Detail: "no supported runtime binary was inspected"})
		}
		sort.Slice(inv.Binaries, func(i, j int) bool {
			a, b := inv.Binaries[i], inv.Binaries[j]
			if a.Member != b.Member {
				return a.Member < b.Member
			}
			if a.Platform != b.Platform {
				return a.Platform < b.Platform
			}
			if a.Arch != b.Arch {
				return a.Arch < b.Arch
			}
			if a.Format != b.Format {
				return a.Format < b.Format
			}
			return a.SHA256 < b.SHA256
		})
		sort.Slice(inv.Issues, func(i, j int) bool {
			a, b := inv.Issues[i], inv.Issues[j]
			if a.Member != b.Member {
				return a.Member < b.Member
			}
			if a.Code != b.Code {
				return a.Code < b.Code
			}
			if a.Severity != b.Severity {
				return a.Severity < b.Severity
			}
			return a.Detail < b.Detail
		})
	}()
	limits, err = scanLimits(limits)
	if err != nil {
		return inv, err
	}
	st, statErr := os.Lstat(filename)
	if statErr != nil {
		return inv, errors.New("cannot stat input artifact")
	}
	if !st.Mode().IsRegular() {
		return inv, errors.New("input artifact must be a regular file, not a symlink or special file")
	}
	if st.Size() > limits.MaxInputBytes {
		return inv, errors.New("compressed input exceeds MaxInputBytes")
	}
	f, openErr := os.Open(filename)
	if openErr != nil {
		return inv, errors.New("cannot open input artifact")
	}
	data, readErr := io.ReadAll(io.LimitReader(f, limits.MaxInputBytes+1))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil {
		return inv, errors.New("cannot read input artifact")
	}
	if int64(len(data)) > limits.MaxInputBytes {
		return inv, errors.New("compressed input exceeds MaxInputBytes")
	}
	inv.SHA256 = scanHash(data)
	s := artifactScanner{inv: &inv, limits: limits, paths: map[string]bool{}, originalPaths: map[string]bool{}, identities: map[string]bool{}}
	lower := strings.ToLower(filename)
	switch {
	case scanZipMagic(data) || strings.HasSuffix(lower, ".zip"):
		err = s.zip(data)
	case bytes.HasPrefix(data, []byte{0x1f, 0x8b}) || strings.HasSuffix(lower, ".tar.gz") || strings.HasSuffix(lower, ".tgz"):
		err = s.tarGzip(data)
	default:
		inv.Files = 1
		if int64(len(data)) > limits.MaxMemberBytes || int64(len(data)) > limits.MaxTotalBytes {
			err = errors.New("raw binary exceeds member or total byte limit")
		} else {
			err = s.inspect(data, "binary")
		}
	}
	return inv, err
}

func scanLimits(l Limits) (Limits, error) {
	d := DefaultLimits()
	if l.MaxInputBytes == 0 {
		l.MaxInputBytes = d.MaxInputBytes
	}
	if l.MaxMemberBytes == 0 {
		l.MaxMemberBytes = d.MaxMemberBytes
	}
	if l.MaxTotalBytes == 0 {
		l.MaxTotalBytes = d.MaxTotalBytes
	}
	if l.MaxFiles == 0 {
		l.MaxFiles = d.MaxFiles
	}
	if l.MaxInputBytes < 0 || l.MaxMemberBytes < 0 || l.MaxTotalBytes < 0 || l.MaxFiles < 0 || l.StripComponents < 0 {
		return l, errors.New("limits and StripComponents must be nonnegative")
	}
	// Reserve room for bounded readers' sentinel byte and the tar metadata
	// budget. Limit values themselves must not overflow those calculations.
	if l.MaxInputBytes == math.MaxInt64 || l.MaxMemberBytes == math.MaxInt64 || l.MaxTotalBytes == math.MaxInt64 ||
		uint64(l.MaxFiles) > uint64(math.MaxInt64/1024) ||
		l.MaxTotalBytes > math.MaxInt64-l.MaxInputBytes-1 ||
		int64(l.MaxFiles)*1024 > math.MaxInt64-l.MaxTotalBytes-l.MaxInputBytes-1 {
		return l, errors.New("limits exceed supported numeric range")
	}
	return l, nil
}

type artifactScanner struct {
	inv           *Inventory
	limits        Limits
	paths         map[string]bool
	originalPaths map[string]bool
	identities    map[string]bool
	total         int64
}

func (s *artifactScanner) issue(code, member, detail string) {
	s.inv.Complete = false
	s.inv.Issues = append(s.inv.Issues, Issue{Code: code, Severity: "error", Member: member, Detail: detail})
}

func (s *artifactScanner) member(name string, directory bool) (string, error) {
	s.inv.Files++
	if s.inv.Files > s.limits.MaxFiles {
		return "", errors.New("archive exceeds MaxFiles")
	}
	if !utf8.ValidString(name) || strings.ContainsRune(name, 0) {
		return "", errors.New("archive member has invalid text in its path")
	}
	name = strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(name, "/") || (len(name) >= 2 && name[1] == ':') {
		return "", errors.New("archive member has an absolute path")
	}
	for _, part := range strings.Split(name, "/") {
		if part == ".." {
			return "", errors.New("archive member path contains parent traversal")
		}
	}
	name = path.Clean(name)
	if s.originalPaths[name] {
		return "", fmt.Errorf("duplicate normalized archive path %q", name)
	}
	s.originalPaths[name] = true
	if name == "." {
		if directory {
			return "", nil
		}
		return "", errors.New("archive member has an empty path")
	}
	parts := strings.Split(name, "/")
	if len(parts) <= s.limits.StripComponents {
		if directory {
			return "", nil
		}
		return "", errors.New("StripComponents removes an entire member path")
	}
	name = strings.Join(parts[s.limits.StripComponents:], "/")
	if s.paths[name] {
		return "", fmt.Errorf("duplicate normalized archive path %q", name)
	}
	s.paths[name] = true
	return name, nil
}

func (s *artifactScanner) readMember(r io.Reader, declared uint64) ([]byte, error) {
	if declared > uint64(s.limits.MaxMemberBytes) {
		return nil, errors.New("archive member exceeds MaxMemberBytes")
	}
	if declared > uint64(s.limits.MaxTotalBytes-s.total) {
		return nil, errors.New("archive exceeds MaxTotalBytes")
	}
	remaining := s.limits.MaxTotalBytes - s.total
	bound := s.limits.MaxMemberBytes
	if remaining < bound {
		bound = remaining
	}
	b, err := io.ReadAll(io.LimitReader(r, bound+1))
	if err != nil {
		return nil, errors.New("cannot read archive member: corrupt data or checksum")
	}
	if int64(len(b)) > bound {
		return nil, errors.New("archive member exceeds member or total byte limit")
	}
	if uint64(len(b)) != declared {
		return nil, errors.New("archive member size does not match header")
	}
	s.total += int64(len(b))
	return b, nil
}

func (s *artifactScanner) zip(data []byte) error {
	if err := scanZipDirectory(data, s.limits.MaxFiles); err != nil {
		return err
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return errors.New("invalid ZIP archive")
	}
	for _, f := range z.File {
		name, err := s.member(f.Name, f.FileInfo().IsDir())
		if err != nil {
			return err
		}
		if f.Flags&1 != 0 {
			return errors.New("encrypted ZIP member is unsupported")
		}
		r, err := f.Open()
		if err != nil {
			return errors.New("cannot open ZIP member")
		}
		b, readErr := s.readMember(r, f.UncompressedSize64)
		closeErr := r.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return errors.New("cannot close ZIP member")
		}
		mode := f.Mode()
		switch {
		case mode.IsDir():
			if len(b) != 0 {
				return errors.New("ZIP directory contains data")
			}
		case mode&os.ModeSymlink != 0:
			s.issue("symlink_not_followed", name, "archive symlink was not followed")
		case !mode.IsRegular():
			s.issue("unsupported_member", name, "archive special file was not followed")
		default:
			if err := s.inspect(b, name); err != nil {
				return err
			}
		}
	}
	return nil
}

// Check the directory before archive/zip allocates one File for every entry.
// Both declared and actual record counts are bounded; a false EOCD count must
// not turn MaxFiles into a check made only after an unbounded allocation.
func scanZipDirectory(data []byte, maxFiles int) error {
	le := binary.LittleEndian
	end := -1
	for i := len(data) - 22; i >= 0 && i >= len(data)-22-65535; i-- {
		if string(data[i:i+4]) == "PK\x05\x06" && i+22+int(le.Uint16(data[i+20:i+22])) == len(data) {
			end = i
			break
		}
	}
	if end < 0 {
		return errors.New("invalid ZIP end record or trailing data")
	}
	if le.Uint16(data[end+4:]) != 0 || le.Uint16(data[end+6:]) != 0 {
		return errors.New("multi-disk ZIP archives are unsupported")
	}
	count := uint64(le.Uint16(data[end+10:]))
	if uint64(le.Uint16(data[end+8:])) != count {
		return errors.New("inconsistent ZIP directory counts")
	}
	size, off := uint64(le.Uint32(data[end+12:])), uint64(le.Uint32(data[end+16:]))
	directoryEnd := uint64(end)
	if count == 0xffff || size == 0xffffffff || off == 0xffffffff {
		locator := end - 20
		if locator < 0 || string(data[locator:locator+4]) != "PK\x06\x07" || le.Uint32(data[locator+4:]) != 0 || le.Uint32(data[locator+16:]) != 1 {
			return errors.New("invalid ZIP64 locator")
		}
		record := le.Uint64(data[locator+8:])
		if record > uint64(locator) || uint64(locator)-record < 56 {
			return errors.New("ZIP64 record is outside archive")
		}
		r := data[int(record):locator]
		if string(r[:4]) != "PK\x06\x06" || le.Uint64(r[4:]) < 44 || le.Uint64(r[4:]) != uint64(len(r)-12) {
			return errors.New("invalid ZIP64 end record")
		}
		if le.Uint32(r[16:]) != 0 || le.Uint32(r[20:]) != 0 || le.Uint64(r[24:]) != le.Uint64(r[32:]) {
			return errors.New("inconsistent or multi-disk ZIP64 archive")
		}
		count, size, off = le.Uint64(r[32:]), le.Uint64(r[40:]), le.Uint64(r[48:])
		directoryEnd = record
	}
	if count > uint64(maxFiles) {
		return errors.New("archive exceeds MaxFiles")
	}
	if off > directoryEnd || size != directoryEnd-off {
		return errors.New("ZIP directory bounds do not match archive")
	}
	pos, actual := off, uint64(0)
	for pos < directoryEnd {
		if directoryEnd-pos < 46 || string(data[int(pos):int(pos)+4]) != "PK\x01\x02" {
			return errors.New("invalid ZIP directory entry")
		}
		h := data[int(pos):]
		length := uint64(46) + uint64(le.Uint16(h[28:])) + uint64(le.Uint16(h[30:])) + uint64(le.Uint16(h[32:]))
		if length > directoryEnd-pos {
			return errors.New("truncated ZIP directory entry")
		}
		actual++
		if actual > uint64(maxFiles) {
			return errors.New("archive exceeds MaxFiles")
		}
		pos += length
	}
	if actual != count {
		return errors.New("ZIP directory count does not match its records")
	}
	return nil
}

func (s *artifactScanner) tarGzip(data []byte) error {
	compressed := bytes.NewReader(data)
	gz, err := gzip.NewReader(compressed)
	if err != nil {
		return errors.New("invalid gzip archive")
	}
	defer gz.Close()
	gz.Multistream(false)
	// In addition to the regular-byte limit, bound expansion of PAX headers,
	// padding and other framing. They must not evade limits by being invisible
	// to tar.Reader.Next. The input-sized allowance accommodates long metadata.
	budget := s.limits.MaxTotalBytes + s.limits.MaxInputBytes + int64(s.limits.MaxFiles)*1024
	observed := &scanTarReader{r: io.LimitReader(gz, budget+1), limit: budget}
	t := tar.NewReader(observed)
	var pendingPadding int64
	for {
		beforeHeader := observed.count
		h, nextErr := t.Next()
		if nextErr == io.EOF {
			// Zero payload bytes are not an archive terminator. Require two
			// complete marker blocks to have been read after the final body.
			if observed.count-beforeHeader < pendingPadding+1024 {
				return errors.New("tar archive is missing its complete zero end marker")
			}
			break
		}
		if nextErr != nil {
			return errors.New("invalid tar.gz member header or stream")
		}
		name, err := s.member(h.Name, h.Typeflag == tar.TypeDir)
		if err != nil {
			return err
		}
		if h.Size < 0 {
			return errors.New("negative tar member size")
		}
		b, err := s.readMember(t, uint64(h.Size))
		if err != nil {
			return err
		}
		pendingPadding = (512 - h.Size%512) % 512
		switch h.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			if err := s.inspect(b, name); err != nil {
				return err
			}
		case tar.TypeDir:
			if len(b) != 0 {
				return errors.New("tar directory contains data")
			}
		case tar.TypeSymlink, tar.TypeLink:
			s.issue("symlink_not_followed", name, "archive link was not followed")
		default:
			s.issue("unsupported_member", name, "archive special file was not followed")
		}
	}
	// tar.Reader stops at the end marker; consuming the gzip stream to EOF is
	// necessary to validate its CRC. Only zero tar padding may follow the marker.
	buf := make([]byte, 32<<10)
	for {
		n, err := observed.Read(buf)
		for _, b := range buf[:n] {
			if b != 0 {
				return errors.New("nonzero data follows tar end marker")
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.New("invalid gzip checksum, trailer or expansion size")
		}
	}
	if observed.count < 1024 || observed.count%512 != 0 || !observed.zeroTrailer() {
		return errors.New("tar archive is missing its complete zero end marker")
	}
	if compressed.Len() != 0 {
		return errors.New("trailing data or another gzip stream follows archive")
	}
	return nil
}

// scanTarReader retains only the final two tar blocks for trailer validation.
type scanTarReader struct {
	r            io.Reader
	limit, count int64
	tail         [1024]byte
}

func (r *scanTarReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	for _, b := range p[:n] {
		r.tail[r.count%1024] = b
		r.count++
	}
	if r.count > r.limit {
		return n, errors.New("tar expansion exceeds framing budget")
	}
	return n, err
}

func (r *scanTarReader) zeroTrailer() bool {
	for _, b := range r.tail {
		if b != 0 {
			return false
		}
	}
	return true
}

func (s *artifactScanner) inspect(data []byte, member string) error {
	if scanNestedArchive(data) {
		s.issue("nested_archive", member, "nested archive was not recursively inspected")
		return nil
	}
	binaries, err := Inspect(data, member)
	if errors.Is(err, ErrUnsupported) || (errors.Is(err, ErrNotBinary) && scanUnsupportedMagic(data)) {
		s.issue("unsupported_binary", member, "recognizable binary format is outside the supported inspection scope")
		return nil
	}
	if errors.Is(err, ErrNotBinary) {
		s.inv.Ignored++
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot inspect member %q: %w", member, err)
	}
	if len(binaries) == 0 {
		s.issue("empty_inspection", member, "inspection returned no runtime binary records")
		return nil
	}
	for _, b := range binaries {
		b.Member = member
		b.SHA256 = scanHash(data)
		key := member + "\x00" + b.Platform + "\x00" + b.Arch
		if s.identities[key] {
			return fmt.Errorf("duplicate member/platform/architecture identity for %q", member)
		}
		s.identities[key] = true
		sort.Strings(b.Unknown)
		if len(b.Unknown) != 0 {
			s.issue("unknown_metadata", member, strings.Join(b.Unknown, "; "))
		}
		s.inv.Binaries = append(s.inv.Binaries, b)
	}
	return nil
}

func scanHash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

func scanZipMagic(b []byte) bool {
	return bytes.HasPrefix(b, []byte("PK\x03\x04")) || bytes.HasPrefix(b, []byte("PK\x05\x06")) || bytes.HasPrefix(b, []byte("PK\x07\x08"))
}

func scanNestedArchive(b []byte) bool {
	return scanZipMagic(b) || bytes.HasPrefix(b, []byte{0x1f, 0x8b}) ||
		bytes.HasPrefix(b, []byte("!<arch>\n")) || bytes.HasPrefix(b, []byte("7z\xbc\xaf\x27\x1c")) ||
		bytes.HasPrefix(b, []byte("Rar!\x1a\x07")) || bytes.HasPrefix(b, []byte("BZh")) ||
		bytes.HasPrefix(b, []byte{0xfd, '7', 'z', 'X', 'Z', 0}) || bytes.HasPrefix(b, []byte{0x28, 0xb5, 0x2f, 0xfd}) ||
		(len(b) >= 262 && string(b[257:262]) == "ustar")
}

func scanUnsupportedMagic(b []byte) bool {
	return bytes.HasPrefix(b, []byte("MZ")) || bytes.HasPrefix(b, []byte{0, 'a', 's', 'm'}) ||
		bytes.HasPrefix(b, []byte("BC\xc0\xde"))
}
