package floor

import (
	"crypto/sha256"
	"debug/elf"
	"encoding/binary"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
)

// These limits also apply to direct callers, independently of archive limits.
// They bound work on publisher-built inputs; they are not an isolation boundary.
const (
	maxBinaryBytes         = 64 << 20
	maxMetadata            = 65536
	maxSections            = 4096
	maxLoadCommands        = 4096
	maxFatSlices           = 32
	maxStringEvidenceBytes = 16 << 20
)

// Inspect reads recorded metadata, without executing data or invoking a loader.
func Inspect(data []byte, member string) ([]Binary, error) {
	if len(data) < 4 {
		return nil, ErrNotBinary
	}
	isELF := string(data[:4]) == "\x7fELF"
	magic := binary.BigEndian.Uint32(data[:4])
	isMach := magic == 0xfeedface || magic == 0xcefaedfe || magic == 0xfeedfacf || magic == 0xcffaedfe
	isFat := magic == 0xcafebabe || magic == 0xbebafeca
	if magic == 0xcafebabf || magic == 0xbfbafeca {
		return nil, fmt.Errorf("%w: FAT64 Mach-O", ErrUnsupported)
	}
	if !isELF && !isMach && !isFat {
		return nil, ErrNotBinary
	}
	if len(data) > maxBinaryBytes {
		return nil, fmt.Errorf("binary exceeds %d-byte inspection limit", maxBinaryBytes)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	var bins []Binary
	var err error
	if isELF {
		var b Binary
		b, err = inspectELF(data)
		bins = []Binary{b}
	} else if isFat {
		bins, err = inspectFat(data, magic)
	} else {
		var b Binary
		b, err = inspectMach(data)
		bins = []Binary{b}
	}
	if err != nil {
		return nil, err
	}
	for i := range bins {
		bins[i].Member, bins[i].SHA256 = member, hash
		sort.Strings(bins[i].Libraries)
		bins[i].Libraries = uniqueStrings(bins[i].Libraries)
		sort.Strings(bins[i].Unknown)
		bins[i].Unknown = uniqueStrings(bins[i].Unknown)
		for j := range bins[i].Requirements {
			sort.Strings(bins[i].Requirements[j].Symbols)
			bins[i].Requirements[j].Symbols = uniqueStrings(bins[i].Requirements[j].Symbols)
		}
		sort.Slice(bins[i].Requirements, func(a, c int) bool {
			x, y := bins[i].Requirements[a], bins[i].Requirements[c]
			if x.Provider != y.Provider {
				return x.Provider < y.Provider
			}
			if x.Version != y.Version {
				return x.Version < y.Version
			}
			return !x.Weak && y.Weak
		})
	}
	sort.Slice(bins, func(i, j int) bool {
		if bins[i].Platform != bins[j].Platform {
			return bins[i].Platform < bins[j].Platform
		}
		return bins[i].Arch < bins[j].Arch
	})
	return bins, nil
}

func uniqueStrings(s []string) []string {
	if len(s) == 0 {
		return s
	}
	n := 1
	for i := 1; i < len(s); i++ {
		if s[i] != s[n-1] {
			s[n] = s[i]
			n++
		}
	}
	return s[:n]
}

func region(data []byte, off, size uint64) ([]byte, error) {
	if off > uint64(len(data)) || size > uint64(len(data))-off {
		return nil, fmt.Errorf("metadata range outside binary: offset=%d size=%d", off, size)
	}
	return data[off : off+size], nil
}

func cstringAt(data []byte, off uint64) (string, error) {
	if off >= uint64(len(data)) {
		return "", fmt.Errorf("string offset outside table: %d", off)
	}
	for i := off; i < uint64(len(data)); i++ {
		if data[i] == 0 {
			if i == off {
				return "", fmt.Errorf("empty metadata string")
			}
			if i-off > 16384 {
				return "", fmt.Errorf("metadata string exceeds limit")
			}
			return string(data[off:i]), nil
		}
		if i-off > 16384 {
			break
		}
	}
	return "", fmt.Errorf("unterminated or excessive metadata string")
}

type elfSection struct {
	typ                    uint32
	flags, addr, off, size uint64
	link, info             uint32
	ent                    uint64
}

func inspectELF(data []byte) (Binary, error) {
	b := Binary{Format: "elf", Platform: "elf", Floors: map[string]string{}}
	if len(data) < 16 {
		return b, fmt.Errorf("truncated ELF identification")
	}
	class := data[4]
	var bo binary.ByteOrder
	switch data[5] {
	case 1:
		bo = binary.LittleEndian
	case 2:
		bo = binary.BigEndian
	default:
		return b, fmt.Errorf("invalid ELF encoding")
	}
	if data[6] != 1 {
		return b, fmt.Errorf("unsupported ELF identification version")
	}
	hsize, shsize, phsize := 64, uint16(64), uint16(56)
	if class == 1 {
		hsize, shsize, phsize = 52, 40, 32
	} else if class != 2 {
		return b, fmt.Errorf("%w: ELF class %d", ErrUnsupported, class)
	}
	if len(data) < hsize {
		return b, fmt.Errorf("truncated ELF header")
	}
	typ, machine := bo.Uint16(data[16:18]), elf.Machine(bo.Uint16(data[18:20]))
	if bo.Uint32(data[20:24]) != 1 {
		return b, fmt.Errorf("invalid ELF version")
	}
	if typ != uint16(elf.ET_EXEC) && typ != uint16(elf.ET_DYN) {
		if typ == uint16(elf.ET_REL) {
			return b, ErrNotBinary
		}
		return b, fmt.Errorf("%w: ELF type %d", ErrUnsupported, typ)
	}
	b.Kind = "executable"
	if typ == uint16(elf.ET_DYN) {
		b.Kind = "shared-library"
	}
	switch machine {
	case elf.EM_X86_64:
		if class != 2 {
			return b, fmt.Errorf("%w: x32 ELF ABI", ErrUnsupported)
		}
		b.Arch = "amd64"
	case elf.EM_386:
		if class != 1 {
			return b, fmt.Errorf("invalid i386 ELF class")
		}
		b.Arch = "386"
	case elf.EM_AARCH64:
		if class != 2 {
			return b, fmt.Errorf("%w: AArch64 ELF class", ErrUnsupported)
		}
		b.Arch = "arm64"
	case elf.EM_ARM:
		if class != 1 {
			return b, fmt.Errorf("invalid ARM ELF class")
		}
		b.Arch = "arm"
	default:
		return b, fmt.Errorf("%w: ELF machine %d", ErrUnsupported, machine)
	}
	if data[5] == 2 {
		b.Arch += "-be"
	}
	var phoff, shoff uint64
	var ehsize, phent, phnum, shent, shnum uint16
	if class == 2 {
		phoff, shoff = bo.Uint64(data[32:40]), bo.Uint64(data[40:48])
		ehsize, phent, phnum, shent, shnum = bo.Uint16(data[52:54]), bo.Uint16(data[54:56]), bo.Uint16(data[56:58]), bo.Uint16(data[58:60]), bo.Uint16(data[60:62])
	} else {
		phoff, shoff = uint64(bo.Uint32(data[28:32])), uint64(bo.Uint32(data[32:36]))
		ehsize, phent, phnum, shent, shnum = bo.Uint16(data[40:42]), bo.Uint16(data[42:44]), bo.Uint16(data[44:46]), bo.Uint16(data[46:48]), bo.Uint16(data[48:50])
	}
	if int(ehsize) != hsize {
		return b, fmt.Errorf("invalid ELF header size")
	}
	if phnum == 0xffff || shnum == 0 {
		return b, fmt.Errorf("%w: sectionless or extended-numbered ELF", ErrUnsupported)
	}
	if shnum > maxSections || phnum > maxSections {
		return b, fmt.Errorf("ELF header count exceeds limit")
	}
	if shent != shsize || (phnum > 0 && phent != phsize) {
		return b, fmt.Errorf("invalid ELF table entry size")
	}
	if shoff < uint64(hsize) {
		return b, fmt.Errorf("ELF section table overlaps header")
	}
	shdata, err := region(data, shoff, uint64(shnum)*uint64(shent))
	if err != nil {
		return b, err
	}
	var interpreter string
	hasProgramDynamic := false
	var programDynamicOff, programDynamicSize uint64
	type elfSegment struct {
		vaddr  uint64
		off    uint64
		filesz uint64
	}
	var loadSegments []elfSegment
	if phnum > 0 {
		if phoff < uint64(hsize) {
			return b, fmt.Errorf("ELF program table overlaps header")
		}
		phdata, err := region(data, phoff, uint64(phnum)*uint64(phent))
		if err != nil {
			return b, err
		}
		for i := 0; i < int(phnum); i++ {
			p := phdata[i*int(phent) : (i+1)*int(phent)]
			pt := bo.Uint32(p[:4])
			var off, size uint64
			if class == 2 {
				off, size = bo.Uint64(p[8:16]), bo.Uint64(p[32:40])
			} else {
				off, size = uint64(bo.Uint32(p[4:8])), uint64(bo.Uint32(p[16:20]))
			}
			body, err := region(data, off, size)
			if err != nil {
				return b, err
			}
			if pt == uint32(elf.PT_LOAD) {
				var segVaddr, segFilesz uint64
				if class == 2 {
					segVaddr, segFilesz = bo.Uint64(p[16:24]), bo.Uint64(p[32:40])
				} else {
					segVaddr, segFilesz = uint64(bo.Uint32(p[8:12])), uint64(bo.Uint32(p[16:20]))
				}
				loadSegments = append(loadSegments, elfSegment{vaddr: segVaddr, off: off, filesz: segFilesz})
			}
			if pt == uint32(elf.PT_DYNAMIC) {
				if hasProgramDynamic {
					return b, fmt.Errorf("duplicate ELF dynamic program header")
				}
				hasProgramDynamic = true
				programDynamicOff, programDynamicSize = off, size
			}
			if pt == uint32(elf.PT_INTERP) {
				if interpreter != "" || len(body) < 2 || body[len(body)-1] != 0 {
					return b, fmt.Errorf("invalid ELF interpreter")
				}
				interpreter, err = cstringAt(body, 0)
				if err != nil {
					return b, err
				}
				b.Kind = "executable"
			}
		}
	}
	sections := make([]elfSection, shnum)
	var dynamic, needs, versym, dynsym *elfSection
	for i := range sections {
		s := shdata[i*int(shent) : (i+1)*int(shent)]
		x := &sections[i]
		x.typ = bo.Uint32(s[4:8])
		if class == 2 {
			x.flags, x.addr, x.off, x.size = bo.Uint64(s[8:16]), bo.Uint64(s[16:24]), bo.Uint64(s[24:32]), bo.Uint64(s[32:40])
			x.link, x.info, x.ent = bo.Uint32(s[40:44]), bo.Uint32(s[44:48]), bo.Uint64(s[56:64])
		} else {
			x.flags, x.addr, x.off, x.size = uint64(bo.Uint32(s[8:12])), uint64(bo.Uint32(s[12:16])), uint64(bo.Uint32(s[16:20])), uint64(bo.Uint32(s[20:24]))
			x.link, x.info, x.ent = bo.Uint32(s[24:28]), bo.Uint32(s[28:32]), uint64(bo.Uint32(s[36:40]))
		}
		if x.typ != uint32(elf.SHT_NOBITS) {
			if _, err := region(data, x.off, x.size); err != nil {
				return b, err
			}
		}
		var slot **elfSection
		switch elf.SectionType(x.typ) {
		case elf.SHT_DYNAMIC:
			slot = &dynamic
		case elf.SHT_GNU_VERNEED:
			slot = &needs
		case elf.SHT_GNU_VERSYM:
			slot = &versym
		case elf.SHT_DYNSYM:
			slot = &dynsym
		}
		if slot != nil {
			if *slot != nil {
				return b, fmt.Errorf("duplicate ELF metadata section type %#x", x.typ)
			}
			if x.flags&0x800 != 0 {
				return b, fmt.Errorf("%w: compressed runtime metadata", ErrUnsupported)
			}
			*slot = x
		}
	}
	sectionData := func(s *elfSection) []byte { return data[s.off : s.off+s.size] }
	mapVAddr := func(vaddr, size uint64) (uint64, error) {
		for _, seg := range loadSegments {
			if vaddr >= seg.vaddr && vaddr <= seg.vaddr+seg.filesz {
				if size > (seg.vaddr+seg.filesz)-vaddr {
					return 0, fmt.Errorf("%w: ELF virtual address range %#x+%d exceeds PT_LOAD segment filesz", ErrUnsupported, vaddr, size)
				}
				return seg.off + (vaddr - seg.vaddr), nil
			}
		}
		return 0, fmt.Errorf("%w: ELF virtual address %#x is not within any PT_LOAD segment", ErrUnsupported, vaddr)
	}
	checkSectionSegmentConsistency := func(s *elfSection) error {
		if s == nil || s.addr == 0 || len(loadSegments) == 0 {
			return nil
		}
		mappedOff, err := mapVAddr(s.addr, s.size)
		if err != nil {
			return err
		}
		if mappedOff != s.off {
			return fmt.Errorf("%w: ELF section %#x file offset %d does not match PT_LOAD segment mapping %d", ErrUnsupported, s.typ, s.off, mappedOff)
		}
		return nil
	}
	if err := checkSectionSegmentConsistency(dynamic); err != nil {
		return b, err
	}
	if err := checkSectionSegmentConsistency(needs); err != nil {
		return b, err
	}
	if err := checkSectionSegmentConsistency(versym); err != nil {
		return b, err
	}
	if err := checkSectionSegmentConsistency(dynsym); err != nil {
		return b, err
	}
	// Offsets refer to the one loader string table. Cache strings so repeated
	// names do not multiply allocations, and bound overlapping-name expansion.
	stringCache := map[uint64]string{}
	stringBytes := 0
	readString := func(str []byte, off uint64) (string, error) {
		if value, ok := stringCache[off]; ok {
			return value, nil
		}
		value, err := cstringAt(str, off)
		if err != nil {
			return "", err
		}
		if len(value) > maxStringEvidenceBytes-stringBytes {
			return "", fmt.Errorf("ELF string evidence exceeds limit")
		}
		stringBytes += len(value)
		stringCache[off] = value
		return value, nil
	}
	stringsFor := func(s *elfSection) ([]byte, error) {
		if int(s.link) >= len(sections) || sections[s.link].typ != uint32(elf.SHT_STRTAB) {
			return nil, fmt.Errorf("invalid linked ELF string table")
		}
		x := &sections[s.link]
		if x.flags&0x800 != 0 {
			return nil, fmt.Errorf("%w: compressed runtime strings", ErrUnsupported)
		}
		if err := checkSectionSegmentConsistency(x); err != nil {
			return nil, err
		}
		return sectionData(x), nil
	}
	tags := map[uint64]uint64{}
	if dynamic != nil {
		if !hasProgramDynamic || dynamic.off != programDynamicOff || dynamic.size != programDynamicSize {
			return b, fmt.Errorf("%w: ELF dynamic section does not match loader program header", ErrUnsupported)
		}
		str, err := stringsFor(dynamic)
		if err != nil {
			return b, err
		}
		ent := uint64(16)
		if class == 1 {
			ent = 8
		}
		if dynamic.ent != ent || dynamic.size%ent != 0 || dynamic.size/ent > maxMetadata {
			return b, fmt.Errorf("invalid or excessive ELF dynamic entries")
		}
		d := sectionData(dynamic)
		terminated := false
		for len(d) > 0 {
			var tag, value uint64
			if class == 2 {
				tag, value = bo.Uint64(d[:8]), bo.Uint64(d[8:16])
			} else {
				tag, value = uint64(bo.Uint32(d[:4])), uint64(bo.Uint32(d[4:8]))
			}
			d = d[ent:]
			if tag == 0 {
				terminated = true
				break
			}
			if tag == uint64(elf.DT_NEEDED) {
				lib, err := readString(str, value)
				if err != nil {
					return b, err
				}
				b.Libraries = append(b.Libraries, lib)
			}
			if tag != uint64(elf.DT_NEEDED) {
				if old, ok := tags[tag]; ok && old != value {
					return b, fmt.Errorf("conflicting ELF dynamic tag %#x", tag)
				}
				tags[tag] = value
			}
		}
		if !terminated {
			return b, fmt.Errorf("unterminated ELF dynamic table")
		}
		if addr, ok := tags[uint64(elf.DT_STRTAB)]; !ok || addr != sections[dynamic.link].addr {
			return b, fmt.Errorf("%w: ELF dynamic string table does not match section", ErrUnsupported)
		}
		if size, ok := tags[uint64(elf.DT_STRSZ)]; !ok || size != uint64(len(str)) {
			return b, fmt.Errorf("ELF dynamic string table size mismatch")
		}
	} else if hasProgramDynamic || needs != nil || versym != nil {
		return b, fmt.Errorf("%w: missing ELF dynamic section", ErrUnsupported)
	}
	if addr, ok := tags[uint64(elf.DT_VERNEED)]; ok && (needs == nil || needs.addr != addr) {
		return b, fmt.Errorf("%w: version needs not represented by matching section", ErrUnsupported)
	}
	if addr, ok := tags[uint64(elf.DT_VERSYM)]; ok && (versym == nil || versym.addr != addr) {
		return b, fmt.Errorf("%w: symbol versions not represented by matching section", ErrUnsupported)
	}
	if count := tags[uint64(elf.DT_VERNEEDNUM)]; count != 0 && needs == nil {
		return b, fmt.Errorf("%w: loader version requirement count has no section", ErrUnsupported)
	}
	if _, ok := tags[uint64(elf.DT_SYMTAB)]; ok && dynsym == nil {
		return b, fmt.Errorf("%w: loader symbols have no matching section", ErrUnsupported)
	}
	indices := map[uint16]int{}
	if needs != nil {
		if dynamic == nil || needs.link != dynamic.link {
			return b, fmt.Errorf("ELF version needs use a different dynamic string table")
		}
		if _, ok := tags[uint64(elf.DT_VERNEED)]; !ok {
			return b, fmt.Errorf("version needs section lacks dynamic reference")
		}
		str, err := stringsFor(needs)
		if err != nil {
			return b, err
		}
		d := sectionData(needs)
		off := uint64(0)
		records := uint64(0)
		for {
			h, err := region(d, off, 16)
			if err != nil {
				return b, err
			}
			if bo.Uint16(h[:2]) != 1 {
				return b, fmt.Errorf("%w: version need record version", ErrUnsupported)
			}
			count := uint64(bo.Uint16(h[2:4]))
			aux, next := uint64(bo.Uint32(h[8:12])), uint64(bo.Uint32(h[12:16]))
			provider, err := readString(str, uint64(bo.Uint32(h[4:8])))
			if err != nil {
				return b, err
			}
			if count == 0 || aux < 16 || aux%4 != 0 {
				return b, fmt.Errorf("invalid ELF version need auxiliary chain")
			}
			pos := off + aux
			for j := uint64(0); j < count; j++ {
				if len(b.Requirements) >= maxMetadata {
					return b, fmt.Errorf("ELF version requirement count exceeds limit")
				}
				a, err := region(d, pos, 16)
				if err != nil {
					return b, err
				}
				flags, idx := bo.Uint16(a[4:6]), bo.Uint16(a[6:8])&0x7fff
				name, err := readString(str, uint64(bo.Uint32(a[8:12])))
				if err != nil {
					return b, err
				}
				if idx < 2 {
					return b, fmt.Errorf("invalid ELF version requirement index")
				}
				if _, exists := indices[idx]; exists {
					return b, fmt.Errorf("duplicate ELF version requirement index")
				}
				indices[idx] = len(b.Requirements)
				b.Requirements = append(b.Requirements, Requirement{Provider: provider, Version: name, Weak: flags&2 != 0})
				if flags & ^uint16(2) != 0 {
					b.Unknown = append(b.Unknown, fmt.Sprintf("version requirement %s has flags %#x", name, flags))
				}
				anext := uint64(bo.Uint32(a[12:16]))
				if j+1 == count {
					if anext != 0 {
						return b, fmt.Errorf("ELF version auxiliary count disagrees with chain")
					}
				} else {
					if anext < 16 || anext%4 != 0 {
						return b, fmt.Errorf("invalid ELF version auxiliary next offset")
					}
					pos += anext
				}
			}
			records++
			if records > maxMetadata {
				return b, fmt.Errorf("ELF version provider count exceeds limit")
			}
			if next == 0 {
				break
			}
			if next < 16 || next%4 != 0 || off+next < pos+16 {
				return b, fmt.Errorf("overlapping ELF version provider chain")
			}
			off += next
		}
		if uint64(needs.info) != records {
			return b, fmt.Errorf("ELF version provider count disagrees with section")
		}
		if n, ok := tags[uint64(elf.DT_VERNEEDNUM)]; !ok || n != records {
			return b, fmt.Errorf("ELF version provider count disagrees with dynamic table")
		}
	}
	if dynsym != nil {
		if dynamic == nil {
			return b, fmt.Errorf("%w: ELF dynamic symbols lack loader metadata", ErrUnsupported)
		}
		if dynsym.link != dynamic.link {
			return b, fmt.Errorf("ELF symbols use a different dynamic string table")
		}
		str, err := stringsFor(dynsym)
		if err != nil {
			return b, err
		}
		ent := uint64(24)
		if class == 1 {
			ent = 16
		}
		if dynsym.ent != ent || dynsym.size%ent != 0 || dynsym.size/ent > maxMetadata {
			return b, fmt.Errorf("invalid or excessive ELF dynamic symbols")
		}
		if addr, ok := tags[uint64(elf.DT_SYMTAB)]; !ok || addr != dynsym.addr {
			return b, fmt.Errorf("%w: ELF dynamic symbol address does not match section", ErrUnsupported)
		}
		if size, ok := tags[uint64(elf.DT_SYMENT)]; !ok || size != ent {
			return b, fmt.Errorf("ELF dynamic symbol entry size mismatch")
		}
		n := dynsym.size / ent
		var versions []byte
		if versym != nil {
			if versym.link >= uint32(len(sections)) || &sections[versym.link] != dynsym || versym.size != n*2 {
				return b, fmt.Errorf("ELF symbol version table size or link mismatch")
			}
			versions = sectionData(versym)
		}
		d := sectionData(dynsym)
		for i := uint64(1); i < n; i++ {
			s := d[i*ent : (i+1)*ent]
			shndx := bo.Uint16(s[6:8])
			if class == 1 {
				shndx = bo.Uint16(s[14:16])
			}
			if shndx != 0 || versions == nil {
				continue
			}
			idx := bo.Uint16(versions[i*2:i*2+2]) & 0x7fff
			if idx < 2 {
				continue
			}
			j, ok := indices[idx]
			if !ok {
				return b, fmt.Errorf("undefined ELF symbol references missing version requirement %d", idx)
			}
			name, err := readString(str, uint64(bo.Uint32(s[:4])))
			if err != nil {
				return b, err
			}
			b.Requirements[j].Symbols = append(b.Requirements[j].Symbols, name)
		}
	} else if needs != nil || versym != nil {
		return b, fmt.Errorf("missing ELF dynamic symbols for version metadata")
	}
	if needs != nil && versym == nil {
		b.Unknown = append(b.Unknown, "version requirements have no symbol version table")
	}
	loader := path.Base(interpreter)
	glibc := map[string]bool{"ld-linux-x86-64.so.2": true, "ld-linux.so.2": true, "ld-linux-aarch64.so.1": true, "ld-linux-armhf.so.3": true, "ld-linux-armel.so.3": true}[loader]
	for _, r := range b.Requirements {
		prefix, version, ok := splitVersionNode(r.Version)
		if strings.HasPrefix(r.Version, "GLIBC_") {
			glibc = true
		}
		if !ok {
			b.Unknown = append(b.Unknown, "non-numeric version requirement "+r.Provider+":"+r.Version)
			continue
		}
		if prefix == "GLIBC" || prefix == "GLIBCXX" || prefix == "CXXABI" {
			setFloor(b.Floors, prefix, version)
		}
	}
	if _, relr := tags[36]; relr {
		word := uint64(8)
		if class == 1 {
			word = 4
		}
		size, hasSize := tags[35]
		entry, hasEntry := tags[37]
		if !hasSize || !hasEntry || entry != word || size%word != 0 {
			return b, fmt.Errorf("invalid ELF RELR table metadata")
		}
		if glibc {
			setFloor(b.Floors, "GLIBC", "2.36")
		} else {
			b.Unknown = append(b.Unknown, "DT_RELR loader requirement cannot be mapped to a glibc floor")
		}
	}
	return b, nil
}

func splitVersionNode(node string) (string, string, bool) {
	i := strings.IndexByte(node, '_')
	if i <= 0 || i == len(node)-1 {
		return "", "", false
	}
	v := node[i+1:]
	for _, part := range strings.Split(v, ".") {
		if part == "" || len(part) > 9 {
			return "", "", false
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return "", "", false
			}
		}
	}
	return node[:i], v, true
}

func setFloor(floors map[string]string, prefix, value string) {
	old, ok := floors[prefix]
	if !ok || numericLess(old, value) {
		floors[prefix] = value
	}
}

func numericLess(a, b string) bool {
	x, y := strings.Split(a, "."), strings.Split(b, ".")
	n := len(x)
	if len(y) > n {
		n = len(y)
	}
	for i := 0; i < n; i++ {
		var p, q uint64
		if i < len(x) {
			p, _ = strconv.ParseUint(x[i], 10, 64)
		}
		if i < len(y) {
			q, _ = strconv.ParseUint(y[i], 10, 64)
		}
		if p != q {
			return p < q
		}
	}
	return false
}

func packedVersion(v uint32) string { return fmt.Sprintf("%d.%d.%d", v>>16, (v>>8)&255, v&255) }

func machArch(cpu, sub uint32) (string, error) {
	base := sub & 0x00ffffff
	var arch string
	var usual uint32
	switch cpu {
	case 7:
		arch, usual = "386", 3
	case 0x1000007:
		arch, usual = "amd64", 3
	case 12:
		arch, usual = "arm", 0
	case 0x100000c:
		arch, usual = "arm64", 0
	default:
		return "", fmt.Errorf("%w: Mach-O CPU %#x", ErrUnsupported, cpu)
	}
	if base != usual || sub&0xff000000 != 0 {
		arch += fmt.Sprintf("-subtype-%08x", sub)
	}
	return arch, nil
}

func inspectMach(data []byte) (Binary, error) {
	b := Binary{Format: "macho", Platform: "macho-unknown", Floors: map[string]string{}}
	if len(data) < 4 {
		return b, fmt.Errorf("truncated Mach-O magic")
	}
	var bo binary.ByteOrder = binary.BigEndian
	magic := bo.Uint32(data[:4])
	if magic == 0xcefaedfe || magic == 0xcffaedfe {
		bo = binary.LittleEndian
		magic = bo.Uint32(data[:4])
	}
	if magic != 0xfeedface && magic != 0xfeedfacf {
		return b, fmt.Errorf("invalid thin Mach-O magic")
	}
	hsize := 28
	if magic == 0xfeedfacf {
		hsize = 32
	}
	if len(data) < hsize {
		return b, fmt.Errorf("truncated Mach-O header")
	}
	cpu, sub, typ := bo.Uint32(data[4:8]), bo.Uint32(data[8:12]), bo.Uint32(data[12:16])
	if (cpu&0x1000000 != 0) != (hsize == 32) {
		return b, fmt.Errorf("Mach-O CPU width disagrees with header")
	}
	var err error
	b.Arch, err = machArch(cpu, sub)
	if err != nil {
		return b, err
	}
	switch typ {
	case 1:
		return b, ErrNotBinary
	case 2:
		b.Kind = "executable"
	case 6:
		b.Kind = "shared-library"
	case 8:
		b.Kind = "bundle"
	default:
		return b, fmt.Errorf("%w: Mach-O file type %d", ErrUnsupported, typ)
	}
	ncmd, cmdsz := bo.Uint32(data[16:20]), bo.Uint32(data[20:24])
	if ncmd > maxLoadCommands || cmdsz > 16<<20 {
		return b, fmt.Errorf("Mach-O load metadata exceeds limit")
	}
	commands, err := region(data, uint64(hsize), uint64(cmdsz))
	if err != nil {
		return b, err
	}
	hasVersion := false
	for i := uint32(0); i < ncmd; i++ {
		if len(commands) < 8 {
			return b, fmt.Errorf("truncated Mach-O load command")
		}
		cmd, size := bo.Uint32(commands[:4]), bo.Uint32(commands[4:8])
		alignment := uint32(4)
		if hsize == 32 {
			alignment = 8
		}
		if size < 8 || uint64(size) > uint64(len(commands)) || size%alignment != 0 {
			return b, fmt.Errorf("invalid Mach-O load command size")
		}
		c := commands[:size]
		commands = commands[size:]
		switch cmd {
		case 0x32, 0x24, 0x25, 0x2f, 0x30:
			if hasVersion {
				return b, fmt.Errorf("%w: multiple Mach-O deployment version commands", ErrUnsupported)
			}
			hasVersion = true
			var platform, min, sdk uint32
			if cmd == 0x32 {
				if size < 24 {
					return b, fmt.Errorf("truncated Mach-O build version")
				}
				platform, min, sdk = bo.Uint32(c[8:12]), bo.Uint32(c[12:16]), bo.Uint32(c[16:20])
				ntools := bo.Uint32(c[20:24])
				if ntools > maxLoadCommands || uint64(size) != 24+uint64(ntools)*8 {
					return b, fmt.Errorf("Mach-O build tool count mismatch")
				}
			} else {
				if size != 16 {
					return b, fmt.Errorf("invalid Mach-O legacy minimum version")
				}
				platform = map[uint32]uint32{0x24: 1, 0x25: 2, 0x2f: 3, 0x30: 4}[cmd]
				min, sdk = bo.Uint32(c[8:12]), bo.Uint32(c[12:16])
			}
			platforms := map[uint32]string{1: "macos", 2: "ios", 3: "tvos", 4: "watchos", 5: "bridgeos", 6: "maccatalyst", 7: "ios-simulator", 8: "tvos-simulator", 9: "watchos-simulator", 10: "driverkit", 11: "visionos", 12: "visionos-simulator"}
			var ok bool
			b.Platform, ok = platforms[platform]
			if !ok {
				b.Platform = fmt.Sprintf("macho-platform-%d", platform)
				b.Unknown = append(b.Unknown, "unknown Mach-O build platform")
			}
			if min == 0 {
				b.Unknown = append(b.Unknown, "Mach-O minimum OS version is zero")
			} else {
				b.MinimumOS = packedVersion(min)
			}
			if sdk != 0 {
				b.SDK = packedVersion(sdk)
			}
		case 0xc, 0x80000018, 0x8000001f, 0x20, 0x80000023:
			if size < 24 {
				return b, fmt.Errorf("truncated Mach-O dylib load command")
			}
			nameOff := bo.Uint32(c[8:12])
			if nameOff < 24 {
				return b, fmt.Errorf("Mach-O dylib name overlaps command header")
			}
			name, err := cstringAt(c, uint64(nameOff))
			if err != nil {
				return b, err
			}
			b.Libraries = append(b.Libraries, name)
			b.Requirements = append(b.Requirements, Requirement{Provider: name, Version: packedVersion(bo.Uint32(c[20:24])), Weak: cmd == 0x80000018})
		default:
			if cmd&0x80000000 != 0 {
				switch cmd {
				case 0x8000001c, // LC_RPATH
					0x80000022, // LC_DYLD_INFO_ONLY
					0x80000028, // LC_MAIN
					0x80000033, // LC_DYLD_EXPORTS_TRIE
					0x80000034, // LC_DYLD_CHAINED_FIXUPS
					0x80000035: // LC_FILESET_ENTRY
					// Known benign mandatory commands
				default:
					b.Unknown = append(b.Unknown, fmt.Sprintf("unsupported mandatory Mach-O load command %#x", cmd))
				}
			}
		}
	}
	if len(commands) != 0 {
		return b, fmt.Errorf("Mach-O command count disagrees with byte size")
	}
	if !hasVersion {
		b.Unknown = append(b.Unknown, "Mach-O deployment version command is missing")
	}
	return b, nil
}

func inspectFat(data []byte, magic uint32) ([]Binary, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("truncated universal Mach-O header")
	}
	var bo binary.ByteOrder = binary.BigEndian
	if magic == 0xbebafeca {
		bo = binary.LittleEndian
	}
	n := bo.Uint32(data[4:8])
	if n == 0 || n > maxFatSlices {
		return nil, fmt.Errorf("invalid or excessive universal slice count")
	}
	tableEnd := uint64(8) + uint64(n)*20
	table, err := region(data, 8, uint64(n)*20)
	if err != nil {
		return nil, err
	}
	type extent struct{ start, end uint64 }
	var extents []extent
	var result []Binary
	seen := map[uint64]bool{}
	kind := ""
	for i := uint32(0); i < n; i++ {
		h := table[i*20 : (i+1)*20]
		cpu, sub := bo.Uint32(h[:4]), bo.Uint32(h[4:8])
		off, size, align := uint64(bo.Uint32(h[8:12])), uint64(bo.Uint32(h[12:16])), bo.Uint32(h[16:20])
		if size == 0 || off < tableEnd || align > 31 || off%(uint64(1)<<align) != 0 {
			return nil, fmt.Errorf("invalid universal slice range or alignment")
		}
		body, err := region(data, off, size)
		if err != nil {
			return nil, err
		}
		for _, x := range extents {
			if off < x.end && x.start < off+size {
				return nil, fmt.Errorf("overlapping universal Mach-O slices")
			}
		}
		extents = append(extents, extent{off, off + size})
		key := uint64(cpu)<<32 | uint64(sub)
		if seen[key] {
			return nil, fmt.Errorf("duplicate universal Mach-O architecture")
		}
		seen[key] = true
		b, err := inspectMach(body)
		if err != nil {
			if err == ErrNotBinary {
				return nil, fmt.Errorf("%w: non-runtime universal slice %d", ErrUnsupported, i)
			}
			return nil, fmt.Errorf("universal slice %d: %w", i, err)
		}
		var innerBO binary.ByteOrder = binary.BigEndian
		m := binary.BigEndian.Uint32(body[:4])
		if m == 0xcefaedfe || m == 0xcffaedfe {
			innerBO = binary.LittleEndian
		}
		if innerBO.Uint32(body[4:8]) != cpu || innerBO.Uint32(body[8:12]) != sub {
			return nil, fmt.Errorf("universal architecture disagrees with slice header")
		}
		if kind != "" && kind != b.Kind {
			return nil, fmt.Errorf("universal slices disagree on runtime file type")
		}
		kind = b.Kind
		result = append(result, b)
	}
	return result, nil
}
