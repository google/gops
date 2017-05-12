// Copyright 2014 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// ppc64map constructs the ppc64 opcode map from the instruction set CSV file.
//
// Usage:
//	ppc64map [-fmt=format] ppc64.csv
//
// The known output formats are:
//
//  text (default) - print decoding tree in text form
//  decoder - print decoding tables for the ppc64asm package
package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	gofmt "go/format"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	asm "golang.org/x/arch/ppc64/ppc64asm"
)

var format = flag.String("fmt", "text", "output format: text, decoder")
var debug = flag.Bool("debug", false, "enable debugging output")

var inputFile string

func usage() {
	fmt.Fprintf(os.Stderr, "usage: ppc64map [-fmt=format] ppc64.csv\n")
	os.Exit(2)
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("ppc64map: ")

	flag.Usage = usage
	flag.Parse()
	if flag.NArg() != 1 {
		usage()
	}

	inputFile = flag.Arg(0)

	var print func(*Prog)
	switch *format {
	default:
		log.Fatalf("unknown output format %q", *format)
	case "text":
		print = printText
	case "decoder":
		print = printDecoder
	}

	p, err := readCSV(flag.Arg(0))
	log.Printf("Parsed %d instruction forms.", len(p.Insts))
	if err != nil {
		log.Fatal(err)
	}

	print(p)
}

// readCSV reads the CSV file and returns the corresponding Prog.
// It may print details about problems to standard error using the log package.
func readCSV(file string) (*Prog, error) {
	// Read input.
	// Skip leading blank and # comment lines.
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	b := bufio.NewReader(f)
	for {
		c, err := b.ReadByte()
		if err != nil {
			break
		}
		if c == '\n' {
			continue
		}
		if c == '#' {
			b.ReadBytes('\n')
			continue
		}
		b.UnreadByte()
		break
	}
	table, err := csv.NewReader(b).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %v", file, err)
	}
	if len(table) == 0 {
		return nil, fmt.Errorf("empty csv input")
	}
	if len(table[0]) < 4 {
		return nil, fmt.Errorf("csv too narrow: need at least four columns")
	}

	p := &Prog{}
	for _, row := range table {
		add(p, row[0], row[1], row[2], row[3])
	}
	return p, nil
}

type Prog struct {
	Insts    []Inst
	OpRanges map[string]string
}

type Field struct {
	Name      string
	BitFields asm.BitFields
	Type      asm.ArgType
	Shift     uint8
}

func (f Field) String() string {
	return fmt.Sprintf("%v(%s%v)", f.Type, f.Name, f.BitFields)
}

type Inst struct {
	Text     string
	Encoding string
	Op       string
	Mask     uint32
	Value    uint32
	DontCare uint32
	Fields   []Field
}

func (i Inst) String() string {
	return fmt.Sprintf("%s (%s) %08x/%08x[%08x] %v (%s)", i.Op, i.Encoding, i.Value, i.Mask, i.DontCare, i.Fields, i.Text)
}

type Arg struct {
	Name string
	Bits int8
	Offs int8
}

func (a Arg) String() string {
	return fmt.Sprintf("%s[%d:%d]", a.Name, a.Offs, a.Offs+a.Bits-1)
}

func (a Arg) Maximum() int {
	return 1<<uint8(a.Bits) - 1
}

func (a Arg) BitMask() uint32 {
	return uint32(a.Maximum()) << a.Shift()
}

func (a Arg) Shift() uint8 {
	return uint8(32 - a.Offs - a.Bits)
}

type Args []Arg

func (as Args) String() string {
	ss := make([]string, len(as))
	for i := range as {
		ss[i] = as[i].String()
	}
	return strings.Join(ss, "|")
}

func (as Args) Find(name string) int {
	for i := range as {
		if as[i].Name == name {
			return i
		}
	}
	return -1
}

func (as *Args) Append(a Arg) {
	*as = append(*as, a)
}

func (as *Args) Delete(i int) {
	*as = append((*as)[:i], (*as)[i+1:]...)
}

func (as Args) Clone() Args {
	return append(Args{}, as...)
}

func (a Arg) isDontCare() bool {
	return a.Name[0] == '/' && a.Name == strings.Repeat("/", len(a.Name))
}

// add adds the entry from the CSV described by text, mnemonics, encoding, and tags
// to the program p.
func add(p *Prog, text, mnemonics, encoding, tags string) {
	if strings.HasPrefix(mnemonics, "e_") || strings.HasPrefix(mnemonics, "se_") {
		// TODO(minux): VLE instructions are ignored.
		return
	}

	// Parse encoding, building size and offset of each field.
	// The first field in the encoding is the smallest offset.
	// And note the MSB is bit 0, not bit 31.
	// Example: "31@0|RS@6|RA@11|///@16|26@21|Rc@31|"
	var args Args
	fields := strings.Split(encoding, "|")
	for i, f := range fields {
		name, off := "", -1
		if f == "" {
			off = 32
			if i == 0 || i != len(fields)-1 {
				fmt.Fprintf(os.Stderr, "%s: wrong %d-th encoding field: %q\n", text, i, f)
				return
			}
		} else {
			j := strings.Index(f, "@")
			if j < 0 {
				fmt.Fprintf(os.Stderr, "%s: wrong %d-th encoding field: %q\n", text, i, f)
				continue
			}
			off, _ = strconv.Atoi(f[j+1:])
			name = f[:j]
		}
		if len(args) > 0 {
			args[len(args)-1].Bits += int8(off)
		}
		if name != "" {
			arg := Arg{Name: name, Offs: int8(off), Bits: int8(-off)}
			args.Append(arg)
		}
	}

	var mask, value, dontCare uint32
	for i := 0; i < len(args); i++ {
		arg := args[i]
		v, err := strconv.Atoi(arg.Name)
		switch {
		case err == nil: // is a numbered field
			if v < 0 || v > arg.Maximum() {
				fmt.Fprintf(os.Stderr, "%s: field %s value (%d) is out of range (%d-bit)\n", text, arg, v, arg.Bits)
			}
			mask |= arg.BitMask()
			value |= uint32(v) << arg.Shift()
			args.Delete(i)
			i--
		case arg.Name[0] == '/': // is don't care
			if arg.Name != strings.Repeat("/", len(arg.Name)) {
				log.Fatalf("%s: arg %v named like a don't care bit, but it's not", text, arg)
			}
			dontCare |= arg.BitMask()
			args.Delete(i)
			i--
		default:
			continue
		}
	}

	// rename duplicated fields (e.g. 30@0|RS@6|RA@11|sh@16|mb@21|0@27|sh@30|Rc@31|)
	// but only support two duplicated fields
	for i := 1; i < len(args); i++ {
		if args[:i].Find(args[i].Name) >= 0 {
			args[i].Name += "2"
		}
		if args[:i].Find(args[i].Name) >= 0 {
			log.Fatalf("%s: more than one duplicated fields: %s", text, args)
		}
	}

	// sanity checks
	if mask&dontCare != 0 {
		log.Fatalf("%s: mask (%08x) and don't care (%08x) collide", text, mask, dontCare)
	}
	if value&^mask != 0 {
		log.Fatalf("%s: value (%08x) out of range of mask (%08x)", text, value, mask)
	}
	var argMask uint32
	for _, arg := range args {
		if arg.Bits <= 0 || arg.Bits > 32 || arg.Offs > 31 || arg.Offs <= 0 {
			log.Fatalf("%s: arg %v has wrong bit field spec", text, arg)
		}
		if mask&arg.BitMask() != 0 {
			log.Fatalf("%s: mask (%08x) intersect with arg %v", text, mask, arg)
		}
		if argMask&arg.BitMask() != 0 {
			log.Fatalf("%s: arg %v overlap with other args %v", text, arg, args)
		}
		argMask |= arg.BitMask()
	}
	if 1<<32-1 != mask|dontCare|argMask {
		log.Fatalf("%s: args %v fail to cover all 32 bits", text, args)
	}

	// split mnemonics into individual instructions
	// example: "b target_addr (AA=0 LK=0)|ba target_addr (AA=1 LK=0)|bl target_addr (AA=0 LK=1)|bla target_addr (AA=1 LK=1)"
	insts := strings.Split(categoryRe.ReplaceAllString(mnemonics, ""), "|")
	for _, inst := range insts {
		value, mask := value, mask
		args := args.Clone()
		if inst == "" {
			continue
		}
		// amend mask and value
		parts := instRe.FindStringSubmatch(inst)
		if parts == nil {
			log.Fatalf("%v couldn't match %s", instRe, inst)
		}
		conds := condRe.FindAllStringSubmatch(parts[2], -1)
		isPCRel := true
		for _, cond := range conds {
			i := args.Find(cond[1])
			v, _ := strconv.ParseInt(cond[2], 16, 32) // the regular expression has checked the number format
			if i < 0 {
				log.Fatalf("%s: %s don't contain arg %s used in %s", text, args, cond[1], inst)
			}
			if cond[1] == "AA" && v == 1 {
				isPCRel = false
			}
			mask |= args[i].BitMask()
			value |= uint32(v) << args[i].Shift()
			args.Delete(i)
		}
		inst := Inst{Text: text, Encoding: parts[1], Value: value, Mask: mask, DontCare: dontCare}

		// order inst.Args according to mnemonics order
		for i, opr := range operandRe.FindAllString(parts[1], -1) {
			if i == 0 { // operation
				inst.Op = opr
				continue
			}
			field := Field{Name: opr}
			typ := asm.TypeUnknown
			var shift uint8
			opr2 := ""
			switch opr {
			case "target_addr":
				shift = 2
				if isPCRel {
					typ = asm.TypePCRel
				} else {
					typ = asm.TypeLabel
				}
				if args.Find("LI") >= 0 {
					opr = "LI"
				} else {
					opr = "BD"
				}
			case "UI", "BO", "BH", "TH", "LEV", "NB", "L", "TO", "FXM", "U", "W", "FLM", "UIM", "SHB", "SHW", "ST", "SIX", "PS", "DCM", "DGM", "RMC", "R", "SP", "S", "DM", "CT", "EH", "E", "MO", "WC", "A", "IH", "OC", "DUI", "DUIS":
				typ = asm.TypeImmUnsigned
				if i := args.Find(opr); i < 0 {
					opr = "D"
				}
			case "SH":
				typ = asm.TypeImmUnsigned
				if args.Find("sh2") >= 0 { // sh2 || sh
					opr = "sh2"
					opr2 = "sh"
				}
			case "MB", "ME":
				typ = asm.TypeImmUnsigned
				if n := strings.ToLower(opr); args.Find(n) >= 0 {
					opr = n // xx[5] || xx[0:4]
				}
			case "SI", "SIM", "TE":
				typ = asm.TypeImmSigned
				if i := args.Find(opr); i < 0 {
					opr = "D"
				}
			case "DS":
				typ = asm.TypeOffset
				shift = 2
			case "DQ":
				typ = asm.TypeOffset
				shift = 4
			case "D":
				if i := args.Find(opr); i >= 0 {
					typ = asm.TypeOffset
					break
				}
				if i := args.Find("UI"); i >= 0 {
					typ = asm.TypeImmUnsigned
					opr = "UI"
					break
				}
				if i := args.Find("SI"); i >= 0 {
					typ = asm.TypeImmSigned
					opr = "SI"
					break
				}
			case "RA", "RB", "RS", "RSp", "RT", "RTp":
				typ = asm.TypeReg
			case "BT", "BA", "BB", "BC", "BI":
				typ = asm.TypeCondRegBit
			case "BF", "BFA":
				typ = asm.TypeCondRegField
			case "FRA", "FRB", "FRBp", "FRC", "FRS", "FRSp", "FRT", "FRTp":
				typ = asm.TypeFPReg
			case "XA", "XB", "XC", "XS", "XT": // 5-bit, split field
				typ = asm.TypeVecSReg
				opr2 = opr[1:]
				opr = opr[1:] + "X"
			case "VRA", "VRB", "VRC", "VRS", "VRT":
				typ = asm.TypeVecReg
			case "SPR", "DCRN", "BHRBE", "TBR", "SR", "TMR", "PMRN": // Note: if you add to this list and the register field needs special handling, add it to switch statement below
				typ = asm.TypeSpReg
				switch opr {
				case "BHRBE":
					opr = "bhrbe" // no special handling
				case "DCRN":
					opr = "DCR"
				}
				if n := strings.ToLower(opr); n != opr && args.Find(n) >= 0 {
					opr = n // spr[5:9] || spr[0:4]
				}
			}
			if typ == asm.TypeUnknown {
				log.Fatalf("%s %s unknown type for opr %s", text, inst, opr)
			}
			field.Type = typ
			field.Shift = shift
			var f1, f2 asm.BitField
			switch {
			case opr2 != "":
				ext := args.Find(opr)
				if ext < 0 {
					log.Fatalf("%s: couldn't find extended field %s in %s", text, opr, args)
				}
				f1.Offs, f1.Bits = uint8(args[ext].Offs), uint8(args[ext].Bits)
				base := args.Find(opr2)
				if base < 0 {
					log.Fatalf("%s: couldn't find base field %s in %s", text, opr2, args)
				}
				f2.Offs, f2.Bits = uint8(args[base].Offs), uint8(args[base].Bits)
			case opr == "mb", opr == "me": // xx[5] || xx[0:4]
				i := args.Find(opr)
				if i < 0 {
					log.Fatalf("%s: couldn't find special 'm[be]' field for %s in %s", text, opr, args)
				}
				f1.Offs, f1.Bits = uint8(args[i].Offs+args[i].Bits)-1, 1
				f2.Offs, f2.Bits = uint8(args[i].Offs), uint8(args[i].Bits)-1
			case opr == "spr", opr == "tbr", opr == "tmr", opr == "dcr": // spr[5:9] || spr[0:4]
				i := args.Find(opr)
				if i < 0 {
					log.Fatalf("%s: couldn't find special 'spr' field for %s in %s", text, opr, args)
				}
				if args[i].Bits != 10 {
					log.Fatalf("%s: special 'spr' field is not 10-bit: %s", text, args)
				}
				f1.Offs, f1.Bits = uint8(args[i].Offs)+5, 5
				f2.Offs, f2.Bits = uint8(args[i].Offs), 5
			default:
				i := args.Find(opr)
				if i < 0 {
					log.Fatalf("%s: couldn't find %s in %s", text, opr, args)
				}
				f1.Offs, f1.Bits = uint8(args[i].Offs), uint8(args[i].Bits)
			}
			field.BitFields.Append(f1)
			if f2.Bits > 0 {
				field.BitFields.Append(f2)
			}
			inst.Fields = append(inst.Fields, field)
		}
		if *debug {
			fmt.Printf("%v\n", inst)
		}

		p.Insts = append(p.Insts, inst)
	}
}

// condRegexp is a regular expression that matches condition in mnemonics (e.g. "AA=1")
const condRegexp = `\s*([[:alpha:]]+)=([0-9a-f]+)\s*`

// condRe matches condition in mnemonics (e.g. "AA=1")
var condRe = regexp.MustCompile(condRegexp)

// instRe matches instruction with potentially multiple conditions in mnemonics
var instRe = regexp.MustCompile(`^(.*?)\s?(\((` + condRegexp + `)+\))?$`)

// categoryRe matches intruction category notices in mnemonics
var categoryRe = regexp.MustCompile(`(\s*\[Category:[^]]*\]\s*)|(\s*\[Co-requisite[^]]*\]\s*)|(\s*\(\s*0[Xx][[0-9A-Fa-f_]{9}\s*\)\s*)`)

// operandRe matches each operand (including opcode) in instruction mnemonics
var operandRe = regexp.MustCompile(`([[:alpha:]][[:alnum:]_]*\.?)`)

// printText implements the -fmt=text mode, which is not implemented (yet?).
func printText(p *Prog) {
	log.Fatal("-fmt=text not implemented")
}

// opName translate an opcode to a valid Go identifier all-cap op name.
func opName(op string) string {
	return strings.ToUpper(strings.Replace(op, ".", "_", 1))
}

// argFieldName constructs a name for the argField
func argFieldName(f Field) string {
	ns := []string{"ap", f.Type.String()}
	for _, b := range f.BitFields {
		ns = append(ns, fmt.Sprintf("%d_%d", b.Offs, b.Offs+b.Bits-1))
	}
	if f.Shift > 0 {
		ns = append(ns, fmt.Sprintf("shift%d", f.Shift))
	}
	return strings.Join(ns, "_")
}

var funcBodyTmpl = template.Must(template.New("funcBody").Parse(``))

// printDecoder implements the -fmt=decoder mode.
// It emits the tables.go for package armasm's decoder.
func printDecoder(p *Prog) {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "// DO NOT EDIT\n")
	fmt.Fprintf(&buf, "// generated by: ppc64map -fmt=decoder %s\n", inputFile)
	fmt.Fprintf(&buf, "\n")

	fmt.Fprintf(&buf, "package ppc64asm\n\n")

	// Build list of opcodes, using the csv order (which corresponds to ISA docs order)
	m := map[string]bool{}
	fmt.Fprintf(&buf, "const (\n\t_ Op = iota\n")
	for _, inst := range p.Insts {
		name := opName(inst.Op)
		if ok := m[name]; ok {
			continue
		}
		m[name] = true
		fmt.Fprintf(&buf, "\t%s\n", name)
	}
	fmt.Fprintln(&buf, ")\n\n")

	// Emit slice mapping opcode number to name string.
	m = map[string]bool{}
	fmt.Fprintf(&buf, "var opstr = [...]string{\n")
	for _, inst := range p.Insts {
		name := opName(inst.Op)
		if ok := m[name]; ok {
			continue
		}
		m[name] = true
		fmt.Fprintf(&buf, "\t%s: %q,\n", opName(inst.Op), inst.Op)
	}
	fmt.Fprintln(&buf, "}\n\n")

	// print out argFields
	fmt.Fprintf(&buf, "var (\n")
	m = map[string]bool{}
	for _, inst := range p.Insts {
		for _, f := range inst.Fields {
			name := argFieldName(f)
			if ok := m[name]; ok {
				continue
			}
			m[name] = true
			fmt.Fprintf(&buf, "\t%s = &argField{Type: %#v, Shift: %d, BitFields: BitFields{", name, f.Type, f.Shift)
			for _, b := range f.BitFields {
				fmt.Fprintf(&buf, "{%d, %d},", b.Offs, b.Bits)
			}
			fmt.Fprintf(&buf, "}}\n")
		}
	}
	fmt.Fprintln(&buf, ")\n\n")

	// Emit decoding table.
	fmt.Fprintf(&buf, "var instFormats = [...]instFormat{\n")
	for _, inst := range p.Insts {
		fmt.Fprintf(&buf, "\t{ %s, %#x, %#x, %#x,", opName(inst.Op), inst.Mask, inst.Value, inst.DontCare)
		fmt.Fprintf(&buf, " // %s (%s)\n\t\t[5]*argField{", inst.Text, inst.Encoding)
		for _, f := range inst.Fields {
			fmt.Fprintf(&buf, "%s, ", argFieldName(f))
		}
		fmt.Fprintf(&buf, "}},\n")
	}
	fmt.Fprintln(&buf, "}\n")

	out, err := gofmt.Source(buf.Bytes())
	if err != nil {
		log.Fatalf("gofmt error: %v", err)
		fmt.Printf("%s", buf.Bytes())
	} else {
		fmt.Printf("%s", out)
	}
}
