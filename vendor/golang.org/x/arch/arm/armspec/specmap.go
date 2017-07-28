// Copyright 2014 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build ignore

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var _ = os.Stdout
var _ = fmt.Sprintf

type Inst struct {
	Name   string
	ID     string
	Bits   string
	Arch   string
	Syntax []string
	Code   string
	Base   uint32
	Mask   uint32
	Prog   []*Stmt
}

func main() {
	data, err := ioutil.ReadFile("spec.json")
	if err != nil {
		log.Fatal(err)
	}
	var insts []Inst
	if err := json.Unmarshal(data, &insts); err != nil {
		log.Fatal(err)
	}

	var out []Inst
	for _, inst := range insts {
		inst.Prog = parse(inst.Name+" "+inst.ID, inst.Code)
		if inst.ID[0] == 'A' && !strings.HasPrefix(inst.Syntax[0], "MSR<c>") && !strings.Contains(inst.Syntax[0], "<coproc>") && !strings.Contains(inst.Syntax[0], "VLDM") && !strings.Contains(inst.Syntax[0], "VSTM") {
			out = append(out, inst)
		}
	}
	insts = out

	for i := range insts {
		dosize(&insts[i])
	}

	var cond, special []Inst
	for _, inst := range insts {
		if inst.Base>>28 == 0xF {
			special = append(special, inst)
		} else {
			cond = append(cond, inst)
		}
	}

	fmt.Printf("special:\n")
	split(special, 0xF0000000, 1)
	fmt.Printf("cond:\n")
	split(cond, 0xF0000000, 1)
}

func dosize(inst *Inst) {
	var base, mask uint32
	off := 0
	for _, f := range strings.Split(inst.Bits, "|") {
		if i := strings.Index(f, ":"); i >= 0 {
			n, _ := strconv.Atoi(f[i+1:])
			off += n
			continue
		}
		for _, bit := range strings.Fields(f) {
			switch bit {
			case "0", "(0)":
				mask |= 1 << uint(31-off)
			case "1", "(1)":
				base |= 1 << uint(31-off)
			}
			off++
		}
	}
	if off != 16 && off != 32 {
		log.Printf("incorrect bit count for %s %s: have %d", inst.Name, inst.Bits, off)
	}
	if off == 16 {
		mask >>= 16
		base >>= 16
	}
	mask |= base
	inst.Mask = mask
	inst.Base = base
}

func split(insts []Inst, used uint32, depth int) {
Again:
	if len(insts) <= 1 {
		for _, inst := range insts {
			fmt.Printf("%*s%#08x %#08x %s %s %v\n", depth*2+2, "", inst.Mask, inst.Base, inst.Syntax[0], inst.Bits, seeRE.FindAllString(inst.Code, -1))
		}
		return
	}

	m := ^used
	for _, inst := range insts {
		m &= inst.Mask
	}
	if m == 0 {
		fmt.Printf("«%*s%#08x masked out (%d)\n", depth*2, "", used, len(insts))
		for _, inst := range insts {
			fmt.Printf("%*s%#08x %#08x %s %s %v\n", depth*2+2, "", inst.Mask, inst.Base, inst.Syntax[0], inst.Bits, seeRE.FindAllString(inst.Code, -1))
		}
		updated := false
		for i := range insts {
			if updateMask(&insts[i]) {
				updated = true
			}
		}
		fmt.Printf("»\n")
		if updated {
			goto Again
		}
		fmt.Printf("%*s%#08x masked out (%d)\n", depth*2, "", used, len(insts))
		for _, inst := range insts {
			fmt.Printf("%*s%#08x %#08x %s %s %v\n", depth*2+2, "", inst.Mask, inst.Base, inst.Syntax[0], inst.Bits, seeRE.FindAllString(inst.Code, -1))
		}
		//checkOverlap(used, insts)
		return
	}
	for i := 31; i >= 0; i-- {
		if m&(1<<uint(i)) != 0 {
			m = 1 << uint(i)
			break
		}
	}
	var bit [2][]Inst
	for _, inst := range insts {
		b := (inst.Base / m) & 1
		bit[b] = append(bit[b], inst)
	}

	for b, list := range bit {
		if len(list) > 0 {
			suffix := ""
			if len(bit[1-b]) == 0 {
				suffix = " (only)"
			}
			fmt.Printf("%*sbit %#08x = %d%s\n", depth*2, "", m, b, suffix)
			split(list, used|m, depth+1)
		}
	}
}

var seeRE = regexp.MustCompile(`SEE ([^;\n]+)`)

func updateMask(inst *Inst) bool {
	defer func() {
		if err := recover(); err != nil {
			fmt.Println("PANIC:", err)
			return
		}
	}()

	print(".")
	println(inst.Name, inst.ID, inst.Bits)
	println(inst.Code)
	wiggle := ^inst.Mask &^ 0xF0000000
	n := countbits(wiggle)
	m1 := ^uint32(0)
	m2 := ^uint32(0)
	for i := uint32(0); i < 1<<uint(n); i++ {
		w := inst.Base | expand(i, wiggle)
		if !isValid(inst, w) {
			continue
		}
		m1 &= w
		m2 &= ^w
	}
	m := m1 | m2
	m &^= 0xF0000000
	m |= 0xF0000000 & inst.Mask
	if m&^inst.Mask != 0 {
		fmt.Printf("%s %s: mask=%#x but decided %#x\n", inst.Name, inst.ID, inst.Mask, m)
		inst.Mask = m
		inst.Base = m1
		return true
	}
	if inst.Mask&^m != 0 {
		fmt.Printf("%s %s: mask=%#x but got %#x\n", inst.Name, inst.ID, inst.Mask, m)
		panic("bad updateMask")
	}
	return false
}

func countbits(x uint32) int {
	n := 0
	for ; x != 0; x >>= 1 {
		n += int(x & 1)
	}
	return n
}

func expand(x, m uint32) uint32 {
	var out uint32
	for i := uint(0); i < 32; i++ {
		out >>= 1
		if m&1 != 0 {
			out |= (x & 1) << 31
			x >>= 1
		}
		m >>= 1
	}
	return out
}
