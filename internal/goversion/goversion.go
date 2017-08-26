// Copyright 2017 The Go Authors. All Rights Reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package goversion

import (
	"bytes"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

type matcher [][]uint32

const (
	pWild uint32 = 0xff00
	pAddr uint32 = 0x10000
	pEnd  uint32 = 0x20000

	opMaybe = 1 + iota
	opMust
	opDone
	opAnchor = 0x100
	opSub8   = 0x200
	opFlags  = opAnchor | opSub8
)

var amd64Matcher = matcher{
	{opMaybe,
		// _rt0_amd64_linux:
		//	lea 0x8(%rsp), %rsi
		//	mov (%rsp), %rdi
		//	lea ADDR(%rip), %rax # main
		//	jmpq *%rax
		0x48, 0x8d, 0x74, 0x24, 0x08,
		0x48, 0x8b, 0x3c, 0x24, 0x48,
		0x8d, 0x05, pWild | pAddr, pWild, pWild, pWild | pEnd,
		0xff, 0xe0,
	},
	{opMaybe,
		// _rt0_amd64_linux:
		//	lea 0x8(%rsp), %rsi
		//	mov (%rsp), %rdi
		//	mov $ADDR, %eax # main
		//	jmpq *%rax
		0x48, 0x8d, 0x74, 0x24, 0x08,
		0x48, 0x8b, 0x3c, 0x24,
		0xb8, pWild | pAddr, pWild, pWild, pWild,
		0xff, 0xe0,
	},
	{opMaybe,
		// _start (toward end)
		//	lea __libc_csu_fini(%rip), %r8
		//	lea __libc_csu_init(%rip), %rcx
		//	lea ADDR(%rip), %rdi # main
		//	callq *xxx(%rip)
		0x4c, 0x8d, 0x05, pWild, pWild, pWild, pWild,
		0x48, 0x8d, 0x0d, pWild, pWild, pWild, pWild,
		0x48, 0x8d, 0x3d, pWild | pAddr, pWild, pWild, pWild | pEnd,
		0xff, 0x15,
	},
	{opMaybe,
		// _start (toward end)
		//	push %rsp (1)
		//	mov $__libc_csu_fini, %r8 (7)
		//	mov $__libc_csu_init, %rcx (7)
		//	mov $ADDR, %rdi # main (7)
		//	callq *xxx(%rip)
		0x54,
		0x49, 0xc7, 0xc0, pWild, pWild, pWild, pWild,
		0x48, 0xc7, 0xc1, pWild, pWild, pWild, pWild,
		0x48, 0xc7, 0xc7, pAddr | pWild, pWild, pWild, pWild,
	},
	{opMaybe | opAnchor,
		// main:
		//	lea ADDR(%rip), %rax # rt0_go
		//	jmpq *%rax
		0x48, 0x8d, 0x05, pWild | pAddr, pWild, pWild, pWild | pEnd,
		0xff, 0xe0,
	},
	{opMaybe | opAnchor,
		// main:
		//	mov $ADDR, %eax
		//	jmpq *%rax
		0xb8, pWild | pAddr, pWild, pWild, pWild,
		0xff, 0xe0,
	},
	{opMust | opAnchor,
		// rt0_go:
		//	mov %rdi, %rax
		//	mov %rsi, %rbx
		//	sub %0x27, %rsp
		//	and $0xfffffffffffffff0,%rsp
		//	mov %rax,0x10(%rsp)
		//	mov %rbx,0x18(%rsp)
		0x48, 0x89, 0xf8,
		0x48, 0x89, 0xf3,
		0x48, 0x83, 0xec, 0x27,
		0x48, 0x83, 0xe4, 0xf0,
		0x48, 0x89, 0x44, 0x24, 0x10,
		0x48, 0x89, 0x5c, 0x24, 0x18,
	},
	{opMust,
		// later in rt0_go:
		//	mov %eax, (%rsp)
		//	mov 0x18(%rsp), %rax
		//	mov %rax, 0x8(%rsp)
		//	callq runtime.args
		//	callq runtime.osinit
		//	callq runtime.schedinit (ADDR)
		//	lea mainPC(%rip), %rax
		0x89, 0x04, 0x24,
		0x48, 0x8b, 0x44, 0x24, 0x18,
		0x48, 0x89, 0x44, 0x24, 0x08,
		0xe8, pWild, pWild, pWild, pWild,
		0xe8, pWild, pWild, pWild, pWild,
		0xe8, pWild, pWild, pWild, pWild,
	},
	{opMaybe,
		// later in rt0_go:
		//	mov %eax, (%rsp)
		//	mov 0x18(%rsp), %rax
		//	mov %rax, 0x8(%rsp)
		//	callq runtime.args
		//	callq runtime.osinit
		//	callq runtime.schedinit (ADDR)
		//	lea other(%rip), %rdi
		0x89, 0x04, 0x24,
		0x48, 0x8b, 0x44, 0x24, 0x18,
		0x48, 0x89, 0x44, 0x24, 0x08,
		0xe8, pWild, pWild, pWild, pWild,
		0xe8, pWild, pWild, pWild, pWild,
		0xe8, pWild | pAddr, pWild, pWild, pWild | pEnd,
		0x48, 0x8d, 0x05,
	},
	{opMaybe,
		// later in rt0_go:
		//	mov %eax, (%rsp)
		//	mov 0x18(%rsp), %rax
		//	mov %rax, 0x8(%rsp)
		//	callq runtime.args
		//	callq runtime.osinit
		//	callq runtime.hashinit
		//	callq runtime.schedinit (ADDR)
		//	pushq $main.main
		0x89, 0x04, 0x24,
		0x48, 0x8b, 0x44, 0x24, 0x18,
		0x48, 0x89, 0x44, 0x24, 0x08,
		0xe8, pWild, pWild, pWild, pWild,
		0xe8, pWild, pWild, pWild, pWild,
		0xe8, pWild, pWild, pWild, pWild,
		0xe8, pWild | pAddr, pWild, pWild, pWild | pEnd,
		0x68,
	},
	{opDone | opSub8,
		// schedinit (toward end)
		//	mov ADDR(%rip), %rax
		//	test %rax, %rax
		//	jne <short>
		//	movq $0x7, ADDR(%rip)
		//
		0x48, 0x8b, 0x05, pWild, pWild, pWild, pWild,
		0x48, 0x85, 0xc0,
		0x75, pWild,
		0x48, 0xc7, 0x05, pWild | pAddr, pWild, pWild, pWild, 0x07, 0x00, 0x00, 0x00 | pEnd,
	},
	{opDone | opSub8,
		// schedinit (toward end)
		//	mov ADDR(%rip), %rbx
		//	cmp $0x0, %rbx
		//	jne <short>
		//	lea "unknown"(%rip), %rbx
		//	mov %rbx, ADDR(%rip)
		//	movq $7, (ADDR+8)(%rip)
		0x48, 0x8b, 0x1d, pWild, pWild, pWild, pWild,
		0x48, 0x83, 0xfb, 0x00,
		0x75, pWild,
		0x48, 0x8d, 0x1d, pWild, pWild, pWild, pWild,
		0x48, 0x89, 0x1d, pWild, pWild, pWild, pWild,
		0x48, 0xc7, 0x05, pWild | pAddr, pWild, pWild, pWild, 0x07, 0x00, 0x00, 0x00 | pEnd,
	},
	{opDone,
		// schedinit (toward end)
		//	cmpq $0x0, ADDR(%rip)
		//	jne <short>
		//	lea "unknown"(%rip), %rax
		//	mov %rax, ADDR(%rip)
		//	lea ADDR(%rip), %rax
		//	movq $7, 8(%rax)
		0x48, 0x83, 0x3d, pWild | pAddr, pWild, pWild, pWild, 0x00,
		0x75, pWild,
		0x48, 0x8d, 0x05, pWild, pWild, pWild, pWild,
		0x48, 0x89, 0x05, pWild, pWild, pWild, pWild,
		0x48, 0x8d, 0x05, pWild | pAddr, pWild, pWild, pWild | pEnd,
		0x48, 0xc7, 0x40, 0x08, 0x07, 0x00, 0x00, 0x00,
	},
	{opDone,
		//	test %eax, %eax
		//	jne <later>
		//	lea "unknown"(RIP), %rax
		//	mov %rax, ADDR(%rip)
		0x48, 0x85, 0xc0, 0x75, pWild, 0x48, 0x8d, 0x05, pWild, pWild, pWild, pWild, 0x48, 0x89, 0x05, pWild | pAddr, pWild, pWild, pWild | pEnd,
	},
}

func (m matcher) match(f Exe, addr uint64) (uint64, bool) {
	data, err := f.ReadData(addr, 512)
	if err != nil {
		return 0, false
	}
Matchers:
	for _, p := range m {
		op := p[0]
		p = p[1:]
	Search:
		for i := 0; i <= len(data)-len(p); i++ {
			a := -1
			e := -1
			if i > 0 && op&opAnchor != 0 {
				break
			}
			for j := 0; j < len(p); j++ {
				b := byte(p[j])
				m := byte(p[j] >> 8)
				if data[i+j]&^m != b {
					continue Search
				}
				if p[j]&pAddr != 0 {
					a = j
				}
				if p[j]&pEnd != 0 {
					e = j + 1
				}
			}
			// matched
			if a != -1 {
				val := uint64(int32(binary.LittleEndian.Uint32(data[i+a:])))
				if e == -1 {
					addr = val
				} else {
					addr += uint64(i+e) + val
				}
				if op&opSub8 != 0 {
					addr -= 8
				}
			}
			if op&^opFlags == opDone {
				return addr, true
			}
			if a != -1 {
				// changed addr, so reload
				data, err = f.ReadData(addr, 512)
				if err != nil {
					return 0, false
				}
			}
			continue Matchers
		}
		// not matched
		if op&^opFlags == opMust {
			return 0, false
		}
	}
	// ran off end of matcher
	return 0, false
}

func readBuildVersionX86Asm(f Exe) (isGo bool, buildVersion string) {
	entry := f.Entry()
	if entry == 0 {
		return
	}
	addr, ok := amd64Matcher.match(f, entry)
	if !ok {
		return
	}
	v, err := readBuildVersion(f, addr, 16)
	if err != nil {
		return
	}
	return true, v
}

type Sym struct {
	Name string
	Addr uint64
	Size uint64
}

type Exe interface {
	AddrSize() int // bytes
	ReadData(addr, size uint64) ([]byte, error)
	Symbols() ([]Sym, error)
	SectionNames() []string
	Close() error
	ByteOrder() binary.ByteOrder
	Entry() uint64
}

func openExe(file string) (Exe, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	data := make([]byte, 16)
	if _, err := io.ReadFull(f, data); err != nil {
		return nil, err
	}
	f.Seek(0, 0)
	if bytes.HasPrefix(data, []byte("\x7FELF")) {
		e, err := elf.NewFile(f)
		if err != nil {
			f.Close()
			return nil, err
		}
		return &elfExe{f, e}, nil
	}
	if bytes.HasPrefix(data, []byte("MZ")) {
		e, err := pe.NewFile(f)
		if err != nil {
			f.Close()
			return nil, err
		}
		return &peExe{f, e}, nil
	}
	if bytes.HasPrefix(data, []byte("\xFE\xED\xFA")) || bytes.HasPrefix(data[1:], []byte("\xFA\xED\xFE")) {
		e, err := macho.NewFile(f)
		if err != nil {
			f.Close()
			return nil, err
		}
		return &machoExe{f, e}, nil
	}
	return nil, fmt.Errorf("unrecognized executable format")
}

type elfExe struct {
	os *os.File
	f  *elf.File
}

func (x *elfExe) AddrSize() int { return 0 }

func (x *elfExe) ByteOrder() binary.ByteOrder { return x.f.ByteOrder }

func (x *elfExe) Close() error {
	return x.os.Close()
}

func (x *elfExe) Entry() uint64 { return x.f.Entry }

func (x *elfExe) ReadData(addr, size uint64) ([]byte, error) {
	data := make([]byte, size)
	for _, prog := range x.f.Progs {
		if prog.Vaddr <= addr && addr+size-1 <= prog.Vaddr+prog.Filesz-1 {
			_, err := prog.ReadAt(data, int64(addr-prog.Vaddr))
			if err != nil {
				return nil, err
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("address not mapped")
}

func (x *elfExe) Symbols() ([]Sym, error) {
	syms, err := x.f.Symbols()
	if err != nil {
		return nil, err
	}
	var out []Sym
	for _, sym := range syms {
		out = append(out, Sym{sym.Name, sym.Value, sym.Size})
	}
	return out, nil
}

func (x *elfExe) SectionNames() []string {
	var names []string
	for _, sect := range x.f.Sections {
		names = append(names, sect.Name)
	}
	return names
}

type peExe struct {
	os *os.File
	f  *pe.File
}

func (x *peExe) imageBase() uint64 {
	switch oh := x.f.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		return uint64(oh.ImageBase)
	case *pe.OptionalHeader64:
		return oh.ImageBase
	}
	return 0
}

func (x *peExe) AddrSize() int {
	if x.f.Machine == pe.IMAGE_FILE_MACHINE_AMD64 {
		return 8
	}
	return 4
}

func (x *peExe) ByteOrder() binary.ByteOrder { return binary.LittleEndian }

func (x *peExe) Close() error {
	return x.os.Close()
}

func (x *peExe) Entry() uint64 {
	switch oh := x.f.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		return uint64(oh.ImageBase + oh.AddressOfEntryPoint)
	case *pe.OptionalHeader64:
		return oh.ImageBase + uint64(oh.AddressOfEntryPoint)
	}
	return 0
}

func (x *peExe) ReadData(addr, size uint64) ([]byte, error) {
	addr -= x.imageBase()
	data := make([]byte, size)
	for _, sect := range x.f.Sections {
		if uint64(sect.VirtualAddress) <= addr && addr+size-1 <= uint64(sect.VirtualAddress+sect.Size-1) {
			_, err := sect.ReadAt(data, int64(addr-uint64(sect.VirtualAddress)))
			if err != nil {
				return nil, err
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("address not mapped")
}

func (x *peExe) Symbols() ([]Sym, error) {
	base := x.imageBase()
	var out []Sym
	for _, sym := range x.f.Symbols {
		if sym.SectionNumber <= 0 || int(sym.SectionNumber) > len(x.f.Sections) {
			continue
		}
		sect := x.f.Sections[sym.SectionNumber-1]
		out = append(out, Sym{sym.Name, uint64(sym.Value) + base + uint64(sect.VirtualAddress), 0})
	}
	return out, nil
}

func (x *peExe) SectionNames() []string {
	var names []string
	for _, sect := range x.f.Sections {
		names = append(names, sect.Name)
	}
	return names
}

type machoExe struct {
	os *os.File
	f  *macho.File
}

func (x *machoExe) AddrSize() int {
	if x.f.Cpu&0x01000000 != 0 {
		return 8
	}
	return 4
}

func (x *machoExe) ByteOrder() binary.ByteOrder { return x.f.ByteOrder }

func (x *machoExe) Close() error {
	return x.os.Close()
}

func (x *machoExe) Entry() uint64 {
	return 0
}

func (x *machoExe) ReadData(addr, size uint64) ([]byte, error) {
	data := make([]byte, size)
	for _, load := range x.f.Loads {
		seg, ok := load.(*macho.Segment)
		if !ok {
			continue
		}
		if seg.Addr <= addr && addr+size-1 <= seg.Addr+seg.Filesz-1 {
			if seg.Name == "__PAGEZERO" {
				continue
			}
			_, err := seg.ReadAt(data, int64(addr-seg.Addr))
			if err != nil {
				return nil, err
			}
			return data, nil
		}
	}
	return nil, fmt.Errorf("address not mapped")
}

func (x *machoExe) Symbols() ([]Sym, error) {
	var out []Sym
	for _, sym := range x.f.Symtab.Syms {
		out = append(out, Sym{sym.Name, sym.Value, 0})
	}
	return out, nil
}

func (x *machoExe) SectionNames() []string {
	var names []string
	for _, sect := range x.f.Sections {
		names = append(names, sect.Name)
	}
	return names
}

var standardCryptoNames = []string{
	"crypto/sha1.(*digest)",
	"crypto/sha256.(*digest)",
	"crypto/rand.(*devReader)",
	"crypto/rsa.encrypt",
	"crypto/rsa.decrypt",
}

func readBuildVersion(f Exe, addr, size uint64) (string, error) {
	if size == 0 {
		size = uint64(f.AddrSize() * 2)
	}
	if size != 8 && size != 16 {
		return "", fmt.Errorf("invalid size for runtime.buildVersion")
	}
	data, err := f.ReadData(addr, size)
	if err != nil {
		return "", fmt.Errorf("reading runtime.buildVersion: %v", err)
	}

	if size == 8 {
		addr = uint64(f.ByteOrder().Uint32(data))
		size = uint64(f.ByteOrder().Uint32(data[4:]))
	} else {
		addr = f.ByteOrder().Uint64(data)
		size = f.ByteOrder().Uint64(data[8:])
	}
	if size > 1000 {
		return "", fmt.Errorf("implausible string size %d for runtime.buildVersion", size)
	}

	data, err = f.ReadData(addr, size)
	if err != nil {
		return "", fmt.Errorf("reading runtime.buildVersion string data: %v", err)
	}
	return string(data), nil
}

func Report(file, diskFile string, info os.FileInfo) (buildVersion string, isGo bool) {
	if info.Mode()&os.ModeSymlink != 0 {
		return
	}
	if file == diskFile && info.Mode()&0111 == 0 {
		return
	}
	f, err := openExe(diskFile)
	if err != nil {
		return
	}
	defer f.Close()
	syms, symsErr := f.Symbols()
	var isGccgo = false
	for _, name := range f.SectionNames() {
		if name == ".note.go.buildid" {
			isGo = true
		}
	}
	for _, sym := range syms {
		name := sym.Name
		if name == "runtime.main" || name == "main.main" {
			isGo = true
		}
		if strings.HasPrefix(name, "runtime.") && strings.HasSuffix(name, "$descriptor") {
			isGccgo = true
		}
		if name == "runtime.buildVersion" {
			isGo = true
			v, err := readBuildVersion(f, sym.Addr, sym.Size)
			if err != nil {
				return
			}
			buildVersion = v
		}
	}

	if buildVersion == "" {
		g, v := readBuildVersionX86Asm(f)
		if g {
			isGo = true
			buildVersion = v
		}
	}
	if isGccgo && buildVersion == "" {
		isGo = true
		buildVersion = "gccgo (unknown)"
	}
	if !isGo && symsErr != nil {
		return
	}
	if !isGo {
		return "", false
	}
	if buildVersion == "" {
		buildVersion = "unknown"
	}
	return buildVersion, isGo
}
