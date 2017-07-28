// Copyright 2014 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// X86map constructs the x86 opcode map from the instruction set CSV file.
//
// Usage:
//	x86map [-fmt=format] x86.csv
//
// The known output formats are:
//
//  text (default) - print decoding tree in text form
//  decoder - print decoding tables for the x86asm package
//  scanner - print scanning tables for x86scan package
package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"
)

var format = flag.String("fmt", "text", "output format: text, decoder")

var inputFile string

func usage() {
	fmt.Fprintf(os.Stderr, "usage: x86map [-fmt=format] x86.csv\n")
	os.Exit(2)
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("x86map: ")

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
	case "scanner":
		print = printScanner
	}

	p, err := readCSV(flag.Arg(0))
	if err != nil {
		log.Fatal(err)
	}

	//p = mergeTail(p)

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
	if len(table[0]) < 6 {
		return nil, fmt.Errorf("csv too narrow: need at least six columns")
	}

	p := &Prog{}
	for _, row := range table {
		add(p, row[0], row[1], row[2], row[3], row[4], row[5])
	}

	check(p)

	return p, nil
}

// A Prog is a single node in the tree representing the instruction format.
// Collectively the tree of nodes form a kind of program for decoding.
// Each Prog has a single action, identifying the kind of node it is,
// and then children to be executed according to the action.
// For example, the Prog with Action="decode" has children named for each
// possible next byte in the input, and those children are the decoding
// tree to execute for the corresponding bytes.
type Prog struct {
	Path   string
	Action string
	Child  map[string]*Prog
	PC     int
	TailID int
}

// keys returns the child keys in sorted order.
func (p *Prog) keys() []string {
	var keys []string
	for key := range p.Child {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// findChildLeaf finds a leaf node in the subtree rooted at p
// and returns that node's full path. The path is useful in error
// messages as an example of where a particular subtree is headed.
func (p *Prog) findChildLeaf() string {
	for {
		if len(p.Child) == 0 {
			return p.Path
		}
		p = p.Child[p.keys()[0]]
	}
}

// walk advances from p to apply the given action and key.
// If p has no action yet, the action is recorded as p.Action.
// Otherwise the action must match p's action: every node in the
// tree can have at most one action, although possibly with many
// alternative keys.
// If p already has an alternative with the given key, walk returns
// that preexisting subtree. Otherwise walk allocates a new Prog
// representing that subtree and returns that node.
func (p *Prog) walk(action, key, text, opcode string) *Prog {
	if p.Action == "" {
		p.Action = action
	} else if p.Action != action {
		log.Printf("%s; %s: conflicting paths %s and %s|%s %s\n", text, opcode, p.findChildLeaf(), p.Path, action, key)
		return new(Prog)
	}
	q := p.Child[key]
	if q == nil {
		if p.Child == nil {
			p.Child = make(map[string]*Prog)
		}
		q = new(Prog)
		q.Path = fmt.Sprintf("%s|%s %s", p.Path, action, key)
		p.Child[key] = q
	}
	return q
}

// add adds a single instructions to the tree rooted at root.
// The string arguments match the CSV: instruction mnemonic,
// opcode encoding, validity in 32- and 64-bit modes, CPUID
// feature set (ignored), and additional tags.
//
// In effect, add adds a new path through the tree leading to
// the given instruction, but it reuses as much of the existing
// tree structure as possible. For example if there have already
// been instructions added starting with 0F and this instruction
// also starts with 0F, that 0F subtree node is reused instead of
// allocating a parallel one. To maximize the reuse, the check action
// sequence along the path being added is the same for every instruction:
// encoding pieces needed to make a decision, 64-bit mode check,
// rex check, prefix check, address size check, data size check,
// register vs memory argument check. Once all those checks have
// been applied, the assumption is that we have uniquely identified
// an instruction, and at that point it is okay to diverge from the
// uniform pattern to set the opcode and read the specific arguments
// corresponding to the instruction at hand.
//
// The maximimal reuse of the existing tree means that the tree
// resulting from all adds have been done amounts to a decision tree.
// There is one detail that makes it non-deterministic: some checks
// do not matter to some instructions and those are recorded as "any" keys.
// If you are decoding and there is a key for the specific thing you are
// seeing as well as the "any" key, both must be considered. To avoid
// adding complexity to the decoder execution, the 'check' function
// removes this case by merging "any" trees into specific keys when
// present.
func add(root *Prog, text, opcode, valid32, valid64, cpuid, tags string) {
	// These are not real instructions: they are either
	// prefixes for other instructions, composite instructions
	// built from multiple individual instructions, or alternate
	// mnemonics of other encodings.
	// Discard for disassembly, because we want a unique decoding.
	if strings.Contains(tags, "pseudo") {
		return
	}

	// Treat REX.W + opcode as being like having an "operand64" tag.
	// The REX.W flag sets the operand size to 64 bits; in this way it is
	// not much different than the 66 prefix that inverts 32 vs 16 bits.
	if strings.Contains(opcode, "REX.W") {
		if !strings.Contains(tags, "operand64") {
			if tags != "" {
				tags += ","
			}
			tags += "operand64"
		}
	}

	// If there is more than one operand size given, we need to do
	// a separate add for each size, because we need multiple
	// keys to be added in the operand size branch, and the code makes
	// a linear pass through the tree adding just one key to each node.
	// We would need to do the same for any other possible repeated tag
	// (for example, if an instruction could have multiple address sizes)
	// but so far operand size is the only tag we have needed to repeat.
	if strings.Count(tags, "operand") > 1 {
		f := strings.Split(tags, ",")
		var ops []string
		w := 0
		for _, tag := range f {
			if strings.HasPrefix(tag, "operand") {
				ops = append(ops, tag)
			} else {
				if strings.Contains(tag, "operand") {
					log.Fatal("unknown tag %q", tag)
				}
				f[w] = tag
				w++
			}
		}
		f = f[:w]
		for _, op := range ops {
			add(root, text, opcode, valid32, valid64, cpuid, strings.Join(append(f, op), ","))
		}
		return
	}

	p := root
	walk := func(action, item string) {
		p = p.walk(action, item, text, opcode)
	}

	// Ignore VEX instructions for now.
	if strings.HasPrefix(opcode, "VEX") {
		if !strings.HasPrefix(text, "VMOVNTDQ") &&
			!strings.HasPrefix(text, "VMOVDQA") &&
			!strings.HasPrefix(text, "VMOVDQU") &&
			!strings.HasPrefix(text, "VZEROUPPER") {
			return
		}
		if !strings.HasPrefix(opcode, "VEX.256") && !strings.HasPrefix(text, "VZEROUPPER") {
			return
		}
		if !strings.Contains(tags, "VEXC4") {
			add(root, text, opcode, valid32, valid64, cpuid, tags+",VEXC4")
		}
		encoding := strings.Fields(opcode)
		walk("decode", encoding[1])
		walk("is64", "any")
		if strings.Contains(tags, "VEXC4") {
			walk("prefix", "C4")
		} else {
			walk("prefix", "C5")
		}
		for _, pref := range strings.Split(encoding[0], ".") {
			if isVexEncodablePrefix[pref] {
				walk("prefix", pref)
			}
		}
	}

	var rex, prefix string
	encoding := strings.Fields(opcode)
	if len(encoding) > 0 && strings.HasPrefix(encoding[0], "REX") {
		rex = encoding[0]
		encoding = encoding[1:]
		if len(encoding) > 0 && encoding[0] == "+" {
			encoding = encoding[1:]
		}
	}
	if len(encoding) > 0 && isPrefix[encoding[0]] {
		prefix = encoding[0]
		encoding = encoding[1:]
	}
	if rex == "" && len(encoding) > 0 && strings.HasPrefix(encoding[0], "REX") {
		rex = encoding[0]
		if rex == "REX" {
			log.Printf("REX without REX.W: %s %s", text, opcode)
		}
		encoding = encoding[1:]
		if len(encoding) > 0 && encoding[0] == "+" {
			encoding = encoding[1:]
		}
	}
	if len(encoding) > 0 && isPrefix[encoding[0]] {
		log.Printf("%s %s: too many prefixes", text, opcode)
		return
	}

	var haveModRM, havePlus bool
	var usedReg string
	for len(encoding) > 0 && (isHex(encoding[0]) || isSlashNum(encoding[0])) {
		key := encoding[0]
		if isSlashNum(key) {
			if usedReg != "" {
				log.Printf("%s %s: multiple modrm checks", text, opcode)
			}
			haveModRM = true
			usedReg = key
		}
		if i := strings.Index(key, "+"); i >= 0 {
			key = key[:i+1]
			havePlus = true
		}
		walk("decode", key)
		encoding = encoding[1:]
	}

	if valid32 != "V" {
		walk("is64", "1")
	} else if valid64 != "V" {
		walk("is64", "0")
	} else {
		walk("is64", "any")
	}

	if prefix == "" {
		prefix = "0"
	}
	walk("prefix", prefix)

	if strings.Contains(tags, "address16") {
		walk("addrsize", "16")
	} else if strings.Contains(tags, "address32") {
		walk("addrsize", "32")
	} else if strings.Contains(tags, "address64") {
		walk("addrsize", "64")
	} else {
		walk("addrsize", "any")
	}

	if strings.Contains(tags, "operand16") {
		walk("datasize", "16")
	} else if strings.Contains(tags, "operand32") {
		walk("datasize", "32")
	} else if strings.Contains(tags, "operand64") {
		walk("datasize", "64")
	} else {
		walk("datasize", "any")
	}

	if len(encoding) > 0 && encoding[0] == "/r" {
		haveModRM = true
	}
	if haveModRM {
		if strings.Contains(tags, "modrm_regonly") {
			walk("ismem", "0")
		} else if strings.Contains(tags, "modrm_memonly") {
			walk("ismem", "1")
		} else {
			walk("ismem", "any")
		}
	}

	walk("op", strings.Fields(text)[0])

	if len(encoding) > 0 && strings.HasPrefix(encoding[0], "VEX") {
		for _, field := range encoding[2:] {
			walk("read", field)
		}
	} else {
		for _, field := range encoding {
			walk("read", field)
		}
	}

	var usedRM string
	for _, arg := range strings.Fields(text)[1:] {
		arg = strings.TrimRight(arg, ",")
		if usesReg[arg] && !haveModRM && !havePlus {
			log.Printf("%s %s: no modrm field to use for %s", text, opcode, arg)
			continue
		}
		if usesRM[arg] && !haveModRM {
			log.Printf("%s %s: no modrm field to use for %s", text, opcode, arg)
			continue
		}
		if usesReg[arg] {
			if usedReg != "" {
				log.Printf("%s %s: modrm reg field used by both %s and %s", text, opcode, usedReg, arg)
				continue
			}
			usedReg = arg
		}
		if usesRM[arg] {
			if usedRM != "" {
				log.Printf("%s %s: modrm r/m field used by both %s and %s", text, opcode, usedRM, arg)
				continue
			}
			usedRM = arg
		}
		walk("arg", arg)
	}

	walk("match", "!")
}

// allKeys records the list of all possible child keys for actions that support "any".
var allKeys = map[string][]string{
	"is64":     {"0", "1"},
	"ismem":    {"0", "1"},
	"addrsize": {"16", "32", "64"},
	"datasize": {"16", "32", "64"},
}

// check checks that the program tree is well-formed.
// It also merges "any" keys into specific decoding keys in order to
// create an invariant that a particular check node either has a
// single "any" child - making it a no-op - or has no "any" children.
// See the discussion of "any" in the comment for add above.
func check(p *Prog) {
	if p.Child["any"] != nil && len(p.Child) > 1 {
		for _, key := range p.keys() {
			if key != "any" {
				mergeCopy(p.Child[key], p.Child["any"])
			}
		}
		if allKeys[p.Action] == nil {
			log.Printf("%s: unknown key space for %s=any", p.Path, p.Action)
		}
		for _, key := range allKeys[p.Action] {
			if p.Child[key] == nil {
				p.Child[key] = p.Child["any"]
			}
		}
		delete(p.Child, "any")
	}

	for _, q := range p.Child {
		check(q)
	}

	switch p.Action {
	case "op", "read", "arg":
		if len(p.Child) > 1 {
			log.Printf("%s: multiple children for action=%s: %v", p.Path, p.Action, p.keys())
		}
	}
}

// mergeCopy merges a copy of the tree rooted at src into dst.
// It is only used once no more paths will be added to the tree,
// so it is safe to introduce cross-links that make the program
// a dag rather than a tree.
func mergeCopy(dst, src *Prog) {
	//log.Printf("merge %s|%s and %s|%s\n", dst.Path, dst.Action, src.Path, src.Action)
	if dst.Action != src.Action {
		log.Printf("cannot merge %s|%s and %s|%s", dst.Path, dst.Action, src.Path, src.Action)
		return
	}

	for _, key := range src.keys() {
		if dst.Child[key] == nil {
			// Create new subtree by creating cross-link.
			dst.Child[key] = src.Child[key]
		} else {
			// Merge src subtree into existing dst subtree.
			mergeCopy(dst.Child[key], src.Child[key])
		}
	}
}

// set returns a map mapping each of the words in all to true.
func set(all string) map[string]bool {
	m := map[string]bool{}
	for _, f := range strings.Fields(all) {
		m[f] = true
	}
	return m
}

// isPrefix records the x86 opcode prefix bytes.
var isPrefix = set(`
	26
	2E
	36
	3E
	64
	65
	66
	67
	F0
	F2
	F3
`)

// usesReg records the argument codes that use the modrm reg field.
var usesReg = set(`
	r8
	r16
	r32
	r64
`)

// usesRM records the argument codes that use the modrm r/m field.
var usesRM = set(`
	r/m8
	r/m16
	r/m32
	r/m64
`)

var isVexEncodablePrefix = set(`
	0F
	0F38
	0F3A
	66
	F3
	F2
`)

// isHex reports whether the argument is a two digit hex number
// possibly followed by a +foo suffix.
func isHex(s string) bool {
	if i := strings.Index(s, "+"); i >= 0 {
		s = s[:i]
	}
	if len(s) != 2 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if '0' <= c && c <= '9' || 'A' <= c && c <= 'F' {
			continue
		}
		return false
	}
	return true
}

// isSlashNum reports whether the argument is /n for some number n in [0,7].
func isSlashNum(s string) bool {
	return len(s) == 2 && s[0] == '/' && '0' <= s[1] && s[1] <= '7'
}

// mergeTail is supposed to merge common subtrees (program tails),
// reducing the size of the final program code.
// It identifies the subtrees using a bottom-up canonicalization.
//
// THIS CODE DOES NOT WORK. IT NEEDS TO BE DEBUGGED.
func mergeTail(p *Prog, emitted map[string]*Prog) *Prog {
	if emitted == nil {
		emitted = make(map[string]*Prog)
	}

	if p.Action == "match" {
		return p
	}

	for _, key := range p.keys() {
		p.Child[key] = mergeTail(p.Child[key], emitted)
	}

	op := ""
	for _, key := range p.keys() {
		q := p.Child[key]
		if q.Action != "op" || len(q.Child) > 1 {
			op = ""
			break
		}
		qop := q.keys()[0]
		if op == "" {
			op = qop
		} else if op != qop {
			op = ""
			break
		}
	}

	if op != "" {
		// Pull 'op x' up above the discriminator.
		p1 := new(Prog)
		*p1 = *p
		for _, key := range p.keys() {
			p1.Child[key] = p.Child[key].Child[op]
		}
		p.Action = "op"
		p.Child = map[string]*Prog{op: p1}
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s\n", p.Action)
	for _, key := range p.keys() {
		fmt.Fprintf(&buf, "%s %d\n", key, p.Child[key].TailID)
	}
	key := buf.String()

	if q := emitted[key]; q != nil {
		return q
	}
	emitted[key] = p
	p.TailID = len(emitted)
	return p
}

// printText prints the tree in textual form.
func printText(p *Prog) {
	printTree(os.Stdout, p, 0, false)
}

var tabs = strings.Repeat("    ", 100)

func printTree(w io.Writer, p *Prog, depth int, compact bool) {
	if compact && len(p.Child) == 1 {
		fmt.Fprintf(w, "%.*s%s", 4*depth, tabs, p.Action)
		for len(p.Child) == 1 {
			key := p.keys()[0]
			child := p.Child[key]
			fmt.Fprintf(w, " %s %s", key, child.Action)
			p = child
		}
		fmt.Fprintf(w, "\n")
	} else {
		fmt.Fprintf(w, "%.*s%s\n", 4*depth, tabs, p.Action)
	}
	for _, key := range p.keys() {
		fmt.Fprintf(w, "%.*s%s\n", 4*(depth+1), tabs, key)
		printTree(w, p.Child[key], depth+2, compact)
	}
}

// printDecoder prints a Go array containing the decoder program.
// It runs in two passes, both of which traverse and could generate
// the entire program. The first pass records the PC for each Prog node,
// and the second pass emits the actual program, using the PCs as jump
// targets in the places where the program is a dag rather than a tree.
func printDecoder(p *Prog) {
	opMap := map[string]bool{
		"PAUSE": true,
	}
	printDecoderPass(p, 1, false, opMap)
	fmt.Printf("// DO NOT EDIT\n")
	fmt.Printf("// generated by: x86map -fmt=decoder %s\n", inputFile)
	fmt.Printf("\n")
	fmt.Printf("package x86asm\n\n")
	fmt.Printf("var decoder = [...]uint16{\n\tuint16(xFail),\n")
	printDecoderPass(p, 1, true, opMap)
	fmt.Printf("}\n\n")

	var ops []string
	for op := range opMap {
		ops = append(ops, op)
	}
	sort.Strings(ops)

	fmt.Printf("const (\n")
	fmt.Printf("\t_ Op = iota\n\n")
	last := ""
	for _, op := range ops {
		fmt.Printf("\t%s\n", op)
		last = op
	}
	fmt.Printf(")\n\n")
	fmt.Printf("const maxOp = %s\n\n", last)

	fmt.Printf("var opNames = [...]string{\n")
	for _, op := range ops {
		fmt.Printf("\t%s: \"%s\",\n", op, op)
	}
	fmt.Printf("}\n")
}

// printScanner prints the decoding table for a scanner.
// The scanner can identify instruction boundaries but does not do
// full decoding. It is meant to be lighter weight than the x86asm
// decoder tables.
func printScanner(p *Prog) {
	walkScanTree(p, -1)
	var out []uint16
	out = append(out, 0)
	emitScanFunc(p, &out)
	fmt.Printf("var scanProg = []uint16{\n")
	fmt.Printf("\t/*0*/ 0, // dead\n")
	for i := 1; i < len(out); i++ {
		fmt.Printf("\t/*%d*/ ", i)
		switch out[i] {
		default:
			log.Fatalf("malformed program %#x", out[i])
		case scanMatch:
			fmt.Printf("scanMatch,\n")
			continue
		case scanJump:
			fmt.Printf("scanJump, %d,\n", out[i+1])
			i++
			continue
		case scanSwitchByte:
			fmt.Printf("scanSwitchByte,\n")
			for j := 0; j < 256/8; j++ {
				fmt.Printf("\t")
				fmt.Printf("/* %#02x-%#02x */", j*8, j*8+7)
				for k := 0; k < 8; k++ {
					fmt.Printf(" %d,", out[i+1+j*8+k])
				}
				fmt.Printf("\n")
			}
			i += 256
			continue
		case scanSwitchSlash:
			fmt.Printf("scanSwitchSlash, %d,\n", out[i+1])
			n := int(out[i+1])
			for j := 0; j < n; j++ {
				fmt.Printf("\t/* byte */ %#x, %d,\n", out[i+2+2*j], out[i+2+2*j+1])
			}
			for j := 0; j < 8; j++ {
				fmt.Printf("\t/* /%d */ %d,\n", j, out[i+2+2*n+j])
			}
			i += 1 + 2*n + 8
			continue
		case scanSwitchPrefix:
			fmt.Printf("scanSwitchPrefix, %d,\n", out[i+1])
			n := int(out[i+1])
			for j := 0; j < n; j++ {
				fmt.Printf("\t/* prefix */ %#x, %d,\n", out[i+2+2*j], out[i+2+2*j+1])
			}
			i += 1 + 2*n
			continue
		case scanSwitchIs64:
			fmt.Printf("scanSwitchIs64, %d, %d\n", out[i+1], out[i+2])
			i += 2
			continue
		case scanSwitchDatasize:
			fmt.Printf("scanSwitchDatasize, %d, %d, %d\n", out[i+1], out[i+2], out[i+3])
			i += 3
			continue
		case scanSwitchIsMem:
			fmt.Printf("scanSwitchIsMem, %d, %d\n", out[i+1], out[i+2])
			i += 2
			continue
		case scanReadModRM:
			fmt.Printf("scanReadModRM,\n")
			continue
		case scanReadIB:
			fmt.Printf("scanReadIB,\n")
			continue
		case scanReadIW:
			fmt.Printf("scanReadIW,\n")
			continue
		case scanReadIWD:
			fmt.Printf("scanReadIWD,\n")
			continue
		case scanReadIWDO:
			fmt.Printf("scanReadIWDO,\n")
			continue
		case scanReadCWD:
			fmt.Printf("scanReadCWD,\n")
			continue
		case scanReadCB:
			fmt.Printf("scanReadCB,\n")
			continue
		case scanReadCDP:
			fmt.Printf("scanReadCDP,\n")
			continue
		case scanReadCM:
			fmt.Printf("scanReadCM,\n")
			continue
		}
	}
	fmt.Printf("}\n")
}

func walkScanTree(p *Prog, is64 int) {
	keys := p.keys()
	for _, key := range keys {
		if p.Action == "is64" {
			switch key {
			case "0":
				is64 = 0
			case "1":
				is64 = 1
			}
		}
		walkScanTree(p.Child[key], is64)
	}

	switch p.Action {
	case "read", "match":
		// keep
		return
	case "decode":
		if len(keys) >= 8 && keys[0] == "/0" && keys[7] == "/7" && allSame(p, keys) {
			p.Action = "read"
			p.Child = map[string]*Prog{"/r": p.Child[keys[0]]}
			return
		}
	case "op", "arg":
		// drop
		*p = *p.Child[keys[0]]
		return
	case "prefix":
		if len(keys) >= 1 && keys[0] == "0" && allSame(p, keys) {
			*p = *p.Child[keys[0]]
			return
		}
	case "is64", "addrsize", "datasize", "ismem":
		if len(keys) == 1 && keys[0] == "any" {
			*p = *p.Child[keys[0]]
			return
		}
		nkey := len(allKeys[p.Action])
		if p.Action == "addrsize" {
			nkey = 2
		}
		if p.Action == "datasize" && is64 == 0 {
			nkey = 2
		}
		if len(keys) == nkey && allSame(p, keys) {
			*p = *p.Child[keys[0]]
			return
		}
	}

	switch p.Action {
	case "datasize":
		if len(keys) == 2 && is64 == 0 || len(keys) == 3 {
			if treeText(p.Child["16"]) == "read iw match ! \n" && treeText(p.Child["32"]) == "read id match ! \n" && (len(keys) == 2 || treeText(p.Child["64"]) == "read id match ! \n") {
				p.Action = "read"
				p.Child = map[string]*Prog{"iwd/d": p.Child["16"].Child["iw"]}
				return
			}
			if len(keys) == 3 && treeText(p.Child["16"]) == "read iw match ! \n" && treeText(p.Child["32"]) == "read id match ! \n" && treeText(p.Child["64"]) == "read io match ! \n" {
				p.Action = "read"
				p.Child = map[string]*Prog{"iwdo/d": p.Child["16"].Child["iw"]}
				return
			}
			if treeText(p.Child["16"]) == "read /r read iw match ! \n" && treeText(p.Child["32"]) == "read /r read id match ! \n" && (len(keys) == 2 || treeText(p.Child["64"]) == "read /r read id match ! \n") {
				p.Action = "read"
				p.Child = map[string]*Prog{"/r": {Action: "read", Child: map[string]*Prog{"iwd/d": p.Child["16"].Child["/r"].Child["iw"]}}}
				return
			}
			if treeText(p.Child["16"]) == "read cw match ! \n" && treeText(p.Child["32"]) == "read cd match ! \n" && (len(keys) == 2 || treeText(p.Child["64"]) == "read cd match ! \n") {
				p.Action = "read"
				p.Child = map[string]*Prog{"cwd/d": p.Child["16"].Child["cw"]}
				return
			}
			if treeText(p.Child["16"]) == "read cd match ! \n" && treeText(p.Child["32"]) == "read cp match ! \n" && (len(keys) == 2 || treeText(p.Child["64"]) == "read cp match ! \n") {
				p.Action = "read"
				p.Child = map[string]*Prog{"cdp/d": p.Child["16"].Child["cd"]}
				return
			}
			fmt.Printf("!! %q\n", treeText(p.Child["16"]))
		}

	case "is64":
		if len(keys) == 2 && treeText(p.Child["0"]) == "read cwd/d match ! \n" && treeText(p.Child["1"]) == "read cd match ! \n" {
			*p = *p.Child["0"]
			return
		}
		if len(keys) == 2 && treeText(p.Child["0"]) == "read iwd/d match ! \n" && treeText(p.Child["1"]) == "read iwdo/d match ! \n" {
			*p = *p.Child["1"]
			return
		}
	}

	/*
		match := make(map[string][]string)
		for _, key := range keys {
			text := treeText(p.Child[key])
			match[text] = append(match[text], key)
		}
		child := make(map[string]*Prog)
		for _, keys := range match {
			child[strings.Join(keys, ",")] = p.Child[keys[0]]
		}
		p.Child = child
	*/
}

func treeText(p *Prog) string {
	var buf bytes.Buffer
	printTree(&buf, p, 0, true)
	return buf.String()
}

func allSame(p *Prog, keys []string) bool {
	var tree string
	for i, key := range keys {
		if i == 0 {
			tree = treeText(p.Child[key])
			continue
		}
		if treeText(p.Child[key]) != tree {
			return false
		}
	}
	return true
}

var scanCache = map[string]uint16{}

const (
	_ uint16 = iota
	scanMatch
	scanJump
	scanSwitchByte
	scanSwitchSlash
	scanSwitchIs64
	scanSwitchDatasize
	scanSwitchIsMem
	scanSwitchPrefix
	scanReadModRM
	scanReadIB
	scanReadIW
	scanReadIWD
	scanReadIWDO
	scanReadCWD
	scanReadCB
	scanReadCDP
	scanReadCM
)

func decodeKeyPlus(key string) (val, n int) {
	n = 1
	if strings.HasSuffix(key, "+") {
		n = 8
		key = key[:len(key)-1]
	}
	v, err := strconv.ParseUint(key, 16, 8)
	if err != nil {
		log.Fatalf("unexpected decode key %q", key)
	}
	return int(v), n
}

func decodeKey(key string) int {
	val, n := decodeKeyPlus(key)
	if n != 1 {
		log.Panicf("unexpected decode key+ %q", key)
	}
	return val
}

func emitScanFunc(p *Prog, out *[]uint16) uint16 {
	keys := p.keys()
	text := treeText(p)
	if off, ok := scanCache[text]; ok {
		return off
	}
	start := uint16(len(*out))
	scanCache[text] = start
	switch p.Action {
	case "decode":
		if keys[0][0] != '/' {
			*out = append(*out, scanSwitchByte)
			off := len(*out)
			for i := 0; i < 256; i++ {
				*out = append(*out, 0)
			}
			for _, key := range keys {
				val, n := decodeKeyPlus(key)
				dst := emitScanFunc(p.Child[key], out)
				for j := 0; j < n; j++ {
					(*out)[off+val+j] = dst
				}
			}
			return start
		}

		n := len(keys)
		for n > 0 && keys[n-1][0] != '/' {
			n--
		}
		total := 0
		for i := n; i < len(keys); i++ {
			key := keys[i]
			_, n := decodeKeyPlus(key)
			total += n
		}
		*out = append(*out, scanSwitchSlash, uint16(total))
		off := len(*out)
		for i := 0; i < total; i++ {
			*out = append(*out, 0, 0)
		}
		for i := 0; i < 8; i++ {
			*out = append(*out, 0)
		}
		for i := n; i < len(keys); i++ {
			key := keys[i]
			val, valn := decodeKeyPlus(key)
			targ := emitScanFunc(p.Child[key], out)
			for j := 0; j < valn; j++ {
				(*out)[off] = uint16(val + j)
				off++
				(*out)[off] = targ
				off++
			}
		}
		for i := 0; i < n; i++ {
			key := keys[i]
			if len(key) != 2 || key[0] != '/' || key[1] < '0' || '8' <= key[1] {
				log.Fatalf("unexpected decode key %q", key)
			}
			(*out)[off+int(key[1]-'0')] = emitScanFunc(p.Child[key], out)
		}
		return start

	case "read":
		switch keys[0] {
		default:
			log.Fatalf("unexpected read %q", keys[0])
		case "/r":
			*out = append(*out, scanReadModRM)
		case "ib":
			*out = append(*out, scanReadIB)
		case "iw":
			*out = append(*out, scanReadIW)
		case "cb":
			*out = append(*out, scanReadCB)
		case "cm":
			*out = append(*out, scanReadCM)
		case "iwd/d":
			*out = append(*out, scanReadIWD)
		case "iwdo/d":
			*out = append(*out, scanReadIWDO)
		case "cwd/d":
			*out = append(*out, scanReadCWD)
		case "cdp/d":
			*out = append(*out, scanReadCDP)
		}
		next := p.Child[keys[0]]
		if next.Action == "match" {
			*out = append(*out, scanMatch)
		} else {
			*out = append(*out, scanJump, 0)
			off := len(*out)
			(*out)[off-1] = emitScanFunc(next, out)
		}
		return start

	case "match":
		*out = append(*out, scanMatch)
		return start

	case "is64":
		*out = append(*out, scanSwitchIs64, 0, 0)
		if next := p.Child["0"]; next != nil {
			(*out)[start+1] = emitScanFunc(next, out)
		}
		if next := p.Child["1"]; next != nil {
			(*out)[start+2] = emitScanFunc(next, out)
		}
		return start

	case "ismem":
		*out = append(*out, scanSwitchIsMem, 0, 0)
		if next := p.Child["0"]; next != nil {
			(*out)[start+1] = emitScanFunc(next, out)
		}
		if next := p.Child["1"]; next != nil {
			(*out)[start+2] = emitScanFunc(next, out)
		}
		return start

	case "datasize":
		*out = append(*out, scanSwitchDatasize, 0, 0, 0)
		if next := p.Child["16"]; next != nil {
			(*out)[start+1] = emitScanFunc(next, out)
		}
		if next := p.Child["32"]; next != nil {
			(*out)[start+2] = emitScanFunc(next, out)
		}
		if next := p.Child["64"]; next != nil {
			(*out)[start+3] = emitScanFunc(next, out)
		}
		return start
	case "prefix":
		*out = append(*out, scanSwitchPrefix, uint16(len(keys)))
		n := len(keys)
		for i := 0; i < n; i++ {
			*out = append(*out, uint16(decodeKey(keys[i])), 0)
		}
		for i := 0; i < n; i++ {
			(*out)[int(start)+2+2*i+1] = emitScanFunc(p.Child[keys[i]], out)
		}
		return start

	}

	log.Fatalf("unexpected action %q", p.Action)
	return start
}

// printDecoderPass prints the decoding table program for p,
// assuming that we are emitting code at the given program counter.
// It returns the new current program counter, that is, the program
// counter after the printed instructions.
// If printing==false, printDecoderPass does not print the actual
// code words but still does the PC computation.
func printDecoderPass(p *Prog, pc int, printing bool, ops map[string]bool) int {
	// Record PC on first pass.
	if p.PC == 0 {
		p.PC = pc
	}

	// If PC doesn't match, we've already printed this code
	// because it was reached some other way. Jump to that copy.
	if p.PC != pc {
		if printing {
			fmt.Printf("/*%d*/\tuint16(xJump), %d,\n", pc, p.PC)
		}
		return pc + 2
	}

	// Otherwise, emit the code for the given action.

	// At the bottom, if next is non-nil, emit code for next.
	// Then emit the code for the children named by the keys.
	keys := p.keys()
	var next *Prog

	switch p.Action {
	default:
		log.Printf("printDecoderPass: unknown action %q: %s", p.Action, p.Path)

	case "decode":
		// Decode hex bytes or /n modrm op checks.
		// Hex bytes take priority, so do them first.
		// Hex bytes of the form "40+" indicate an
		// 8 entry-wide swath of codes: 40, 41, ..., 47.
		hex := 0
		slash := 0
		for _, key := range keys {
			if isHex(key) {
				if strings.Contains(key, "+") {
					hex += 8
				} else {
					hex++
				}
			}
			if isSlashNum(key) {
				slash++
			}
		}
		if hex > 0 {
			// TODO(rsc): Introduce an xCondByte256 that has 256 child entries
			// and no explicit keys. That will cut the size in half for large
			// tables.
			if printing {
				fmt.Printf("/*%d*/\tuint16(xCondByte), %d,\n", pc, hex)
				for _, key := range keys {
					if !isHex(key) {
						continue
					}
					if i := strings.Index(key, "+"); i >= 0 {
						nextPC := p.Child[key].PC
						n, _ := strconv.ParseUint(key[:i], 16, 0)
						for j := 0; j < 8; j++ {
							fmt.Printf("\t%#02x, %d,\n", int(n)+j, nextPC)
						}
						continue
					}
					fmt.Printf("\t0x%s, %d,\n", key, p.Child[key].PC)
				}
			}
			pc += 2 + 2*hex

			// All other condition checks fail the decoding if nothing is found,
			// but this one falls through so that we can then do /n checks.
			// If there are no upcoming /n checks, insert an explicit failure.
			if slash == 0 {
				if printing {
					fmt.Printf("\tuint16(xFail),\n")
				}
				pc++
			}
		}
		if slash > 0 {
			if printing {
				fmt.Printf("/*%d*/\tuint16(xCondSlashR),\n", pc)
				for i := 0; i < 8; i++ {
					fmt.Printf("\t%d, // %d\n", p.childPC(fmt.Sprintf("/%d", i)), i)
				}
			}
			pc += 1 + 8
		}

	case "is64":
		// Decode based on processor mode: 64-bit or not.
		if len(keys) == 1 && keys[0] == "any" {
			next = p.Child["any"]
			break
		}
		if p.Child["any"] != nil {
			log.Printf("%s: mixed is64 keys: %v", p.Path, keys)
		}

		if printing {
			fmt.Printf("/*%d*/\tuint16(xCondIs64), %d, %d,\n", pc, p.childPC("0"), p.childPC("1"))
		}
		pc += 3

	case "prefix":
		// Decode based on presence of prefix.
		// The "0" prefix means "none of the above", so if there's
		// nothing else, it's the same as "any".
		if len(keys) == 1 && (keys[0] == "any" || keys[0] == "0") {
			next = p.Child["any"]
			break
		}
		if p.Child["any"] != nil {
			log.Printf("%s: mixed prefix keys: %v", p.Path, keys)
		}

		// Emit the prefixes in reverse sorted order, so that F3 and F2 are
		// considered before 66, and the fallback 0 is considered last.
		if printing {
			fmt.Printf("/*%d*/\tuint16(xCondPrefix), %d,\n", pc, len(keys))
			for i := len(keys) - 1; i >= 0; i-- {
				key := keys[i]
				nextPC := p.Child[key].PC
				fmt.Printf("\t0x%s, %d,\n", key, nextPC)
			}
		}
		pc += 2 + 2*len(keys)

	case "addrsize":
		// Decode based on address size attribute.
		if len(keys) == 1 && keys[0] == "any" {
			next = p.Child["any"]
			break
		}
		if p.Child["any"] != nil {
			log.Printf("%s: mixed addrsize keys: %v", p.Path, keys)
		}

		if printing {
			fmt.Printf("/*%d*/\tuint16(xCondAddrSize), %d, %d, %d,\n", pc, p.childPC("16"), p.childPC("32"), p.childPC("64"))
		}
		pc += 4

	case "datasize":
		// Decode based on operand size attribute.
		if len(keys) == 1 && keys[0] == "any" {
			next = p.Child["any"]
			break
		}
		if p.Child["any"] != nil {
			log.Printf("%s: mixed datasize keys: %v", p.Path, keys)
		}

		if printing {
			fmt.Printf("/*%d*/\tuint16(xCondDataSize), %d, %d, %d,\n", pc, p.childPC("16"), p.childPC("32"), p.childPC("64"))
		}
		pc += 4

	case "ismem":
		// Decode based on modrm form: memory or register reference.
		if len(keys) == 1 && keys[0] == "any" {
			next = p.Child["any"]
			break
		}
		if p.Child["any"] != nil {
			log.Printf("%s: mixed ismem keys: %v", p.Path, keys)
		}

		if printing {
			fmt.Printf("/*%d*/\tuint16(xCondIsMem), %d, %d,\n", pc, p.childPC("0"), p.childPC("1"))
		}
		pc += 3

	case "op":
		// Set opcode.
		ops[keys[0]] = true
		if printing {
			fmt.Printf("/*%d*/\tuint16(xSetOp), uint16(%s),\n", pc, keys[0])
		}
		next = p.Child[keys[0]]
		pc += 2

	case "read":
		// Read argument bytes.
		if printing {
			fmt.Printf("/*%d*/\tuint16(xRead%s),\n", pc, xOp(keys[0]))
		}
		next = p.Child[keys[0]]
		pc++

	case "arg":
		// Record instruction argument (interpret bytes loaded with read).
		if printing {
			fmt.Printf("/*%d*/\tuint16(xArg%s),\n", pc, xOp(keys[0]))
		}
		next = p.Child[keys[0]]
		pc++

	case "match":
		// Finish match.
		if printing {
			fmt.Printf("/*%d*/\tuint16(xMatch),\n", pc)
		}
		pc++
		return pc
	}

	if next != nil {
		pc = printDecoderPass(next, pc, printing, ops)
	}

	for _, key := range keys {
		q := p.Child[key]
		if q.PC == 0 || q.PC == pc {
			pc = printDecoderPass(q, pc, printing, ops)
		}
	}

	return pc
}

// childPC returns the PC for the given child key.
// If the key is not present, it returns PC 0,
// which is known to be an xFail instruction.
func (p *Prog) childPC(key string) int {
	q := p.Child[key]
	if q == nil {
		return 0
	}
	return q.PC
}

// isLower reports whether c is an ASCII lower case letter.
func isLower(c byte) bool {
	return 'a' <= c && c <= 'z'
}

// isLetterDigit reports whether c is an ASCII letter or digit.
func isLetterDigit(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
}

// xOp converts arg, an Intel manual shorthand, into a decoder opcode suffix.
// The standard form is LeadingUpperLetter with a few punctuation symbols
// turned into purely lower case words: M16and32, M16colon32, CR0dashCR7.
func xOp(arg string) string {
	var buf []byte
	for i := 0; i < len(arg); i++ {
		c := arg[i]
		if isLower(c) && (i == 0 || !isLetterDigit(arg[i-1])) {
			c -= 'a' - 'A'
		}
		buf = append(buf, c)
	}
	return argFix.Replace(string(buf))
}

var argFix = strings.NewReplacer(
	"/R", "SlashR",
	"/", "",
	"<", "",
	">", "",
	"+", "plus",
	"-", "dash",
	":", "colon",
	"&", "and",
	"ST(0)", "ST",
	"ST(I)", "STi",
	"ST(I)+Op", "STi",
)
