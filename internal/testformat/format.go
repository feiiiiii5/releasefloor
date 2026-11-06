// Package testformat builds original synthetic object-format fixtures.
package testformat

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
)

// These fixtures are generated here from public object-format structures.
// No external executable, linker output, or third-party source is included.
type Node struct {
	Provider, Version, Symbol string
	WeakNode, WeakSymbol      bool
}

func ELF(nodes []Node, relr bool) []byte {
	bo := binary.LittleEndian
	str := []byte{0}
	addString := func(s string) uint32 {
		off := uint32(len(str))
		str = append(str, s...)
		str = append(str, 0)
		return off
	}
	type node struct {
		Node
		versionOff, symbolOff uint32
		index                 uint16
	}
	type provider struct {
		nameOff uint32
		nodes   []node
	}
	var groups []provider
	for i, n := range nodes {
		g := -1
		for j := range groups {
			if groups[j].nodes[0].Provider == n.Provider {
				g = j
			}
		}
		if g == -1 {
			groups = append(groups, provider{nameOff: addString(n.Provider)})
			g = len(groups) - 1
		}
		groups[g].nodes = append(groups[g].nodes, node{Node: n, versionOff: addString(n.Version), symbolOff: addString(n.Symbol), index: uint16(i + 2)})
	}
	var need, syms bytes.Buffer
	versions := make([]byte, (len(nodes)+1)*2)
	syms.Write(make([]byte, 24))
	for gi, g := range groups {
		next := uint32(16 + 16*len(g.nodes))
		if gi == len(groups)-1 {
			next = 0
		}
		binary.Write(&need, bo, uint16(1))
		binary.Write(&need, bo, uint16(len(g.nodes)))
		binary.Write(&need, bo, g.nameOff)
		binary.Write(&need, bo, uint32(16))
		binary.Write(&need, bo, next)
		for j, n := range g.nodes {
			flags := uint16(0)
			if n.WeakNode {
				flags = 2
			}
			anext := uint32(16)
			if j == len(g.nodes)-1 {
				anext = 0
			}
			binary.Write(&need, bo, uint32(0))
			binary.Write(&need, bo, flags)
			binary.Write(&need, bo, n.index)
			binary.Write(&need, bo, n.versionOff)
			binary.Write(&need, bo, anext)
			info := byte(elf.STB_GLOBAL) << 4
			if n.WeakSymbol {
				info = byte(elf.STB_WEAK) << 4
			}
			binary.Write(&syms, bo, n.symbolOff)
			syms.WriteByte(info)
			syms.WriteByte(0)
			binary.Write(&syms, bo, uint16(0))
			binary.Write(&syms, bo, uint64(0))
			binary.Write(&syms, bo, uint64(0))
			// Symbol order is group order; version indices are deliberately independent.
			bo.PutUint16(versions[(syms.Len()/24-1)*2:], n.index)
		}
	}
	const base = uint64(0x400000)
	var data bytes.Buffer
	data.Write(make([]byte, 64+2*56))
	type section struct {
		typ            uint32
		off, size, ent uint64
		link, info     uint32
	}
	sections := []section{{}}
	add := func(typ uint32, body []byte, link, info uint32, ent uint64) int {
		for data.Len()%8 != 0 {
			data.WriteByte(0)
		}
		sections = append(sections, section{typ: typ, off: uint64(data.Len()), size: uint64(len(body)), link: link, info: info, ent: ent})
		data.Write(body)
		return len(sections) - 1
	}
	add(uint32(elf.SHT_STRTAB), str, 0, 0, 0)
	add(uint32(elf.SHT_DYNSYM), syms.Bytes(), 1, 1, 24)
	add(uint32(elf.SHT_GNU_VERNEED), need.Bytes(), 1, uint32(len(groups)), 0)
	add(uint32(elf.SHT_GNU_VERSYM), versions, 2, 0, 2)
	relrIndex := 0
	if relr {
		relrIndex = add(19, make([]byte, 8), 0, 0, 8)
	}
	var dynamic bytes.Buffer
	entry := func(tag, value uint64) { binary.Write(&dynamic, bo, tag); binary.Write(&dynamic, bo, value) }
	for _, g := range groups {
		entry(uint64(elf.DT_NEEDED), uint64(g.nameOff))
	}
	entry(uint64(elf.DT_STRTAB), base+sections[1].off)
	entry(uint64(elf.DT_STRSZ), sections[1].size)
	entry(uint64(elf.DT_SYMTAB), base+sections[2].off)
	entry(uint64(elf.DT_SYMENT), 24)
	entry(uint64(elf.DT_VERNEED), base+sections[3].off)
	entry(uint64(elf.DT_VERNEEDNUM), uint64(len(groups)))
	entry(uint64(elf.DT_VERSYM), base+sections[4].off)
	if relr {
		entry(36, base+sections[relrIndex].off)
		entry(35, 8)
		entry(37, 8)
	}
	entry(0, 0)
	dynamicIndex := add(uint32(elf.SHT_DYNAMIC), dynamic.Bytes(), 1, 0, 16)
	for data.Len()%8 != 0 {
		data.WriteByte(0)
	}
	shoff := uint64(data.Len())
	for i, s := range sections {
		flags := uint64(2)
		if i == 0 {
			flags = 0
		}
		binary.Write(&data, bo, uint32(0))
		binary.Write(&data, bo, s.typ)
		binary.Write(&data, bo, flags)
		addr := uint64(0)
		if i != 0 {
			addr = base + s.off
		}
		binary.Write(&data, bo, addr)
		binary.Write(&data, bo, s.off)
		binary.Write(&data, bo, s.size)
		binary.Write(&data, bo, s.link)
		binary.Write(&data, bo, s.info)
		binary.Write(&data, bo, uint64(8))
		binary.Write(&data, bo, s.ent)
	}
	out := data.Bytes()
	copy(out[:16], []byte{0x7f, 'E', 'L', 'F', 2, 1, 1})
	bo.PutUint16(out[16:], uint16(elf.ET_DYN))
	bo.PutUint16(out[18:], uint16(elf.EM_X86_64))
	bo.PutUint32(out[20:], 1)
	bo.PutUint64(out[32:], 64)
	bo.PutUint64(out[40:], shoff)
	bo.PutUint16(out[52:], 64)
	bo.PutUint16(out[54:], 56)
	bo.PutUint16(out[56:], 2)
	bo.PutUint16(out[58:], 64)
	bo.PutUint16(out[60:], uint16(len(sections)))
	putProgram := func(pos int, typ uint32, off, size uint64) {
		bo.PutUint32(out[pos:], typ)
		bo.PutUint32(out[pos+4:], 4)
		bo.PutUint64(out[pos+8:], off)
		bo.PutUint64(out[pos+16:], base+off)
		bo.PutUint64(out[pos+24:], base+off)
		bo.PutUint64(out[pos+32:], size)
		bo.PutUint64(out[pos+40:], size)
		bo.PutUint64(out[pos+48:], 8)
	}
	putProgram(64, uint32(elf.PT_LOAD), 0, uint64(len(out)))
	s := sections[dynamicIndex]
	putProgram(120, uint32(elf.PT_DYNAMIC), s.off, s.size)
	return out
}

func Mach(cpu, subtype, min, sdk uint32, legacy bool, commands ...[]byte) []byte {
	bo := binary.LittleEndian
	var version bytes.Buffer
	if legacy {
		binary.Write(&version, bo, uint32(0x24))
		binary.Write(&version, bo, uint32(16))
		binary.Write(&version, bo, min)
		binary.Write(&version, bo, sdk)
	} else {
		for _, n := range []uint32{0x32, 24, 1, min, sdk, 0} {
			binary.Write(&version, bo, n)
		}
	}
	all := append([]byte(nil), version.Bytes()...)
	for _, cmd := range commands {
		all = append(all, cmd...)
	}
	var out bytes.Buffer
	for _, n := range []uint32{0xfeedfacf, cpu, subtype, 2, uint32(len(commands) + 1), uint32(len(all)), 0, 0} {
		binary.Write(&out, bo, n)
	}
	out.Write(all)
	return out.Bytes()
}

func Dylib(cmd uint32, name string) []byte {
	bo := binary.LittleEndian
	size := (24 + len(name) + 1 + 7) / 8 * 8
	out := make([]byte, size)
	for i, n := range []uint32{cmd, uint32(size), 24, 0, 0x30200, 0x20100} {
		bo.PutUint32(out[i*4:], n)
	}
	copy(out[24:], name)
	return out
}

func Fat(slices ...[]byte) []byte {
	bo := binary.BigEndian
	out := make([]byte, 8+len(slices)*20)
	bo.PutUint32(out, 0xcafebabe)
	bo.PutUint32(out[4:], uint32(len(slices)))
	for i, s := range slices {
		for len(out)%8 != 0 {
			out = append(out, 0)
		}
		h := out[8+i*20 : 8+(i+1)*20]
		bo.PutUint32(h, binary.LittleEndian.Uint32(s[4:8]))
		bo.PutUint32(h[4:], binary.LittleEndian.Uint32(s[8:12]))
		bo.PutUint32(h[8:], uint32(len(out)))
		bo.PutUint32(h[12:], uint32(len(s)))
		bo.PutUint32(h[16:], 3)
		out = append(out, s...)
	}
	return out
}
