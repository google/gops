// Copyright 2014 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.6
// +build !386 go1.8
// ... see golang.org/issue/12840

// Armspec reads the ``ARM Architecture Reference Manual''
// to collect instruction encoding details and writes those details to standard output
// in JSON format.
//
// Warning Warning Warning
//
// This program is unfinished. It is being published in this incomplete form
// for interested readers, but do not expect it to be runnable or useful.
//
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"rsc.io/pdf"
)

type Inst struct {
	Name   string
	ID     string
	Bits   string
	Arch   string
	Syntax []string
	Code   string
}

const debugPage = 0

var stdout *bufio.Writer

func main() {
	log.SetFlags(0)
	log.SetPrefix("armspec: ")

	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: armspec file.pdf\n")
		os.Exit(2)
	}

	f, err := pdf.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	// Find instruction set reference in outline, to build instruction list.
	instList := instHeadings(f.Outline())
	if len(instList) < 200 {
		log.Fatalf("only found %d instructions in table of contents", len(instList))
	}

	stdout = bufio.NewWriter(os.Stdout)
	fmt.Fprintf(stdout, "[")
	numTable := 0
	defer stdout.Flush()

	// Scan document looking for instructions.
	// Must find exactly the ones in the outline.
	n := f.NumPage()
PageLoop:
	for pageNum := 1; pageNum <= n; pageNum++ {
		if debugPage > 0 && pageNum != debugPage {
			continue
		}
		if pageNum > 1127 {
			break
		}
		p := f.Page(pageNum)
		name, table := parsePage(pageNum, p)
		if name == "" {
			continue
		}
		if len(table) < 1 {
			if false {
				fmt.Fprintf(os.Stderr, "no encodings for instruction %q (page %d)\n", name, pageNum)
			}
			continue
		}
		for _, inst := range table {
			if numTable > 0 {
				fmt.Fprintf(stdout, ",")
			}
			numTable++
			js, _ := json.Marshal(inst)
			fmt.Fprintf(stdout, "\n%s", jsFix.Replace(string(js)))
		}
		for j, headline := range instList {
			if name == headline {
				instList[j] = ""
				continue PageLoop
			}
		}
		fmt.Fprintf(os.Stderr, "unexpected instruction %q (page %d)\n", name, pageNum)
	}

	fmt.Fprintf(stdout, "\n]\n")
	stdout.Flush()

	if debugPage == 0 {
		for _, headline := range instList {
			if headline != "" {
				switch headline {
				default:
					fmt.Fprintf(os.Stderr, "missing instruction %q\n", headline)
				case "CHKA": // ThumbEE
				case "CPS": // system instruction
				case "CPY": // synonym for MOV
				case "ENTERX": // ThumbEE
				case "F* (former VFP instruction mnemonics)": // synonyms
				case "HB, HBL, HBLP, HBP": // ThumbEE
				case "LEAVEX": // ThumbEE
				case "MOV (shifted register)": // pseudo instruction for ASR, LSL, LSR, ROR, and RRX
				case "NEG": // synonym for RSB
				case "RFE": // system instruction
				case "SMC (previously SMI)": // system instruction
				case "SRS": // system instruction
				case "SUBS PC, LR and related instructions": // system instruction
				case "VAND (immediate)": // pseudo instruction
				case "VCLE (register)": // pseudo instruction
				case "VCLT (register)": // pseudo instruction
				case "VORN (immediate)": // pseudo instruction
				}
			}
		}
	}
}

func instHeadings(outline pdf.Outline) []string {
	return appendInstHeadings(outline, nil)
}

var instRE = regexp.MustCompile(`A[\d.]+ Alphabetical list of instructions`)
var childRE = regexp.MustCompile(`A[\d.]+ (.+)`)
var sectionRE = regexp.MustCompile(`^A[\d.]+$`)
var bitRE = regexp.MustCompile(`^( |[01]|\([01]\))*$`)

func appendInstHeadings(outline pdf.Outline, list []string) []string {
	if instRE.MatchString(outline.Title) {
		for _, child := range outline.Child {
			m := childRE.FindStringSubmatch(child.Title)
			if m == nil {
				fmt.Fprintf(os.Stderr, "cannot parse section title: %s\n", child.Title)
				continue
			}
			list = append(list, m[1])
		}
	}
	for _, child := range outline.Child {
		list = appendInstHeadings(child, list)
	}
	return list
}

const inch = 72.0

func parsePage(num int, p pdf.Page) (name string, table []Inst) {
	content := p.Content()

	var text []pdf.Text
	for _, t := range content.Text {
		if match(t, "Times-Roman", 7.2, "") {
			t.FontSize = 9
		}
		if match(t, "Times-Roman", 6.72, "") && '0' <= t.S[0] && t.S[0] <= '9' {
			t.S = string([]rune("⁰¹²³⁴⁵⁶⁷⁸⁹")[t.S[0]-'0'])
			t.FontSize = 9
			t.Y -= 2.28
		}
		if t.Font == "Gen_Arial" {
			continue
		}
		text = append(text, t)
	}

	text = findWords(text)

	for i, t := range text {
		if t.Font == "Times" {
			t.Font = "Times-Roman"
			text[i] = t
		}
	}

	if debugPage > 0 {
		for _, t := range text {
			fmt.Println(t)
		}
		for _, r := range content.Rect {
			fmt.Println(r)
		}
	}

	// Remove text we should ignore.
	out := text[:0]
	skip := false
	for _, t := range text {
		// skip page footer
		if match(t, "Helvetica", 8, "A") || match(t, "Helvetica", 8, "ARM DDI") || match(t, "Helvetica-Oblique", 8, "Copyright") {
			continue
		}
		// skip section header and body text
		if match(t, "Helvetica-Bold", 12, "") && (sectionRE.MatchString(t.S) || t.S == "Alphabetical list of instructions") {
			skip = true
			continue
		}
		if skip && match(t, "Times-Roman", 9, "") {
			continue
		}
		skip = false
		out = append(out, t)
	}
	text = out

	// Page header must say Instruction Details.
	if len(text) == 0 || !match(text[0], "Helvetica-Oblique", 8, "Instruction Details") && !match(text[0], "Times-Roman", 9, "Instruction Details") {
		return "", nil
	}
	text = text[1:]

	isSection := func(text []pdf.Text, i int) int {
		if i+2 <= len(text) && match(text[i], "Helvetica-Bold", 10, "") && sectionRE.MatchString(text[i].S) && match(text[i+1], "Helvetica-Bold", 10, "") {
			return 2
		}
		if i+1 <= len(text) && match(text[i], "Helvetica-Bold", 10, "") && childRE.MatchString(text[i].S) {
			return 1
		}
		return 0
	}

	// Skip dummy headlines and sections.
	for d := isSection(text, 0); d != 0; d = isSection(text, 0) {
		i := d
		for i < len(text) && !match(text[i], "Helvetica-Bold", 9, "Encoding") && !match(text[i], "Helvetica-Bold", 10, "") {
			i++
		}
		if isSection(text, i) == 0 {
			break
		}
		text = text[i:]
	}

	// Next line is headline. Can wrap to multiple lines.
	d := isSection(text, 0)
	if d == 0 {
		if debugPage > 0 {
			fmt.Printf("non-inst-headline: %v\n", text[0])
		}
		checkNoEncodings(num, text)
		return "", nil
	}
	if d == 2 {
		name = text[1].S
		text = text[2:]
	} else if d == 1 {
		m := childRE.FindStringSubmatch(text[0].S)
		name = m[1]
		text = text[1:]
	}
	for len(text) > 0 && match(text[0], "Helvetica-Bold", 10, "") {
		name += " " + text[0].S
		text = text[1:]
	}

	// Skip description.
	for len(text) > 0 && (match(text[0], "Times-Roman", 9, "") || match(text[0], "LucidaSansTypewriteX", 6.48, "") || match(text[0], "Times-Bold", 10, "Note")) {
		text = text[1:]
	}

	// Encodings follow.
	warned := false
	for i := 0; i < len(text); {
		if match(text[i], "Helvetica-Bold", 10, "Assembler syntax") ||
			match(text[i], "Helvetica-Bold", 9, "Modified operation in ThumbEE") ||
			match(text[i], "Helvetica-Bold", 9, "Unallocated memory hints") ||
			match(text[i], "Helvetica-Bold", 9, "Related encodings") ||
			match(text[i], "Times-Roman", 9, "Figure A") ||
			match(text[i], "Helvetica-Bold", 9, "Table A") ||
			match(text[i], "Helvetica-Bold", 9, "VFP Instructions") ||
			match(text[i], "Helvetica-Bold", 9, "VFP instructions") ||
			match(text[i], "Helvetica-Bold", 9, "VFP vectors") ||
			match(text[i], "Helvetica-Bold", 9, "FLDMX") ||
			match(text[i], "Helvetica-Bold", 9, "FSTMX") ||
			match(text[i], "Helvetica-Bold", 9, "Advanced SIMD and VFP") {
			checkNoEncodings(num, text[i:])
			break
		}
		if match(text[i], "Helvetica-Bold", 9, "Figure A") {
			y := text[i].Y
			i++
			for i < len(text) && math.Abs(text[i].Y-y) < 2 {
				i++
			}
			continue
		}
		if !match(text[i], "Helvetica-Bold", 9, "Encoding") {
			if !warned {
				warned = true
				fmt.Fprintln(os.Stderr, "page", num, ": unexpected:", text[i])
			}
			i++
			continue
		}
		inst := Inst{
			Name: name,
		}
		enc := text[i].S
		x := text[i].X
		i++
		// Possible subarchitecture notes.
		for i < len(text) && text[i].X > x+36 {
			if inst.Arch != "" {
				inst.Arch += " "
			}
			inst.Arch += text[i].S
			i++
		}
		// Encoding syntaxes.
		for i < len(text) && (match(text[i], "LucidaSansTypewriteX", 6.48, "") || text[i].X > x+36) {
			if text[i].X < x+0.25*inch {
				inst.Syntax = append(inst.Syntax, text[i].S)
			} else {
				s := inst.Syntax[len(inst.Syntax)-1]
				if !strings.Contains(s, "\t") {
					s += "\t"
				} else {
					s += " "
				}
				s += text[i].S
				inst.Syntax[len(inst.Syntax)-1] = s
			}
			i++
		}

		var bits, abits, aenc string
		bits, i = readBitBox(inst.Name, inst.Syntax, content, text, i)
		if strings.Contains(enc, " / ") {
			if i < len(text) && match(text[i], "Times-Roman", 8, "") {
				abits, i = readBitBox(inst.Name, inst.Syntax, content, text, i)
			} else {
				abits = bits
			}
			slash := strings.Index(enc, " / ")
			aenc = "Encoding " + enc[slash+len(" / "):]
			enc = enc[:slash]
		}

		// pseudocode
		y0 := -1 * inch
		tab := 0.0
		for i < len(text) && match(text[i], "LucidaSansTypewriteX", 6.48, "") {
			t := text[i]
			i++
			if math.Abs(t.Y-y0) < 3 {
				// same line as last fragment, probably just two spaces
				inst.Code += " " + t.S
				continue
			}
			if inst.Code != "" {
				inst.Code += "\n"
			}
			if t.X > x+0.1*inch {
				if tab == 0 {
					tab = t.X - x
				}
				inst.Code += strings.Repeat("\t", int((t.X-x)/tab+0.5))
			} else {
				tab = 0
			}
			inst.Code += t.S
			y0 = t.Y
		}

		inst.ID = strings.TrimPrefix(enc, "Encoding ")
		inst.Bits = bits
		table = append(table, inst)
		if abits != "" {
			inst.ID = strings.TrimPrefix(aenc, "Encoding ")
			inst.Bits = abits
			table = append(table, inst)
		}

	}
	return name, table
}

func readBitBox(name string, syntax []string, content pdf.Content, text []pdf.Text, i int) (string, int) {
	// bit headings
	y2 := 0.0
	x1 := 0.0
	x2 := 0.0
	for i < len(text) && match(text[i], "Times-Roman", 8, "") {
		if y2 == 0 {
			y2 = text[i].Y
		}
		if x1 == 0 {
			x1 = text[i].X
		}
		i++
	}
	// bit fields in box
	y1 := 0.0
	dy1 := 0.0
	for i < len(text) && match(text[i], "Times-Roman", 9, "") {
		if x2 < text[i].X+text[i].W {
			x2 = text[i].X + text[i].W
		}
		y1 = text[i].Y
		dy1 = text[i].FontSize
		i++
	}

	if debugPage > 0 {
		fmt.Println("encoding box", x1, y1, x2, y2)
	}

	// Find lines (thin rectangles) separating bit fields.
	var bottom, top pdf.Rect
	const (
		yMargin = 0.25 * 72
		xMargin = 2 * 72
	)
	for _, r := range content.Rect {
		if r.Max.Y-r.Min.Y < 2 && x1-xMargin < r.Min.X && r.Min.X < x1 && x2 < r.Max.X && r.Max.X < x2+xMargin {
			if y1-yMargin < r.Min.Y && r.Min.Y < y1 {
				bottom = r
			}
			if y1+dy1 < r.Min.Y && r.Min.Y < y2 {
				top = r
			}
		}
	}

	if debugPage > 0 {
		fmt.Println("top", top, "bottom", bottom)
	}

	const ε = 0.1 * 72
	var bars []pdf.Rect
	for _, r := range content.Rect {
		if r.Max.X-r.Min.X < 2 && math.Abs(r.Min.Y-bottom.Min.Y) < ε && math.Abs(r.Max.Y-top.Min.Y) < ε {
			bars = append(bars, r)
		}
	}
	sort.Sort(RectHorizontal(bars))

	// There are 16-bit and 32-bit encodings.
	// In practice, they are about 2.65 and 5.3 inches wide, respectively.
	// Use 4 inches as a cutoff.
	nbit := 32
	dx := top.Max.X - top.Min.X
	if top.Max.X-top.Min.X < 4*72 {
		nbit = 16
	}

	total := 0
	var buf bytes.Buffer
	for i := 0; i < len(bars)-1; i++ {
		if i > 0 {
			fmt.Fprintf(&buf, "|")
		}
		var sub []pdf.Text
		x1, x2 := bars[i].Min.X, bars[i+1].Min.X
		for _, t := range content.Text {
			tx := t.X + t.W/2
			ty := t.Y + t.FontSize/2
			if x1 < tx && tx < x2 && y1 < ty && ty < y2 {
				sub = append(sub, t)
			}
		}
		var str []string
		for _, t := range findWords(sub) {
			str = append(str, t.S)
		}
		s := strings.Join(str, " ")
		s = strings.Replace(s, ")(", ") (", -1)
		n := len(strings.Fields(s))
		b := int(float64(nbit)*(x2-x1)/dx + 0.5)
		if n == b {
			for j, f := range strings.Fields(s) {
				if j > 0 {
					fmt.Fprintf(&buf, "|")
				}
				fmt.Fprintf(&buf, "%s", f)
			}
		} else {
			if n != 1 {
				fmt.Fprintf(os.Stderr, "%s - %s - multi-field %d-bit encoding: %s\n", name, syntax, n, s)
			}
			fmt.Fprintf(&buf, "%s:%d", s, b)
		}
		total += b
	}

	if total != nbit || total == 0 {
		fmt.Fprintf(os.Stderr, "%s - %s - %d-bit encoding\n", name, syntax, total)
	}
	return buf.String(), i
}

type RectHorizontal []pdf.Rect

func (x RectHorizontal) Swap(i, j int)      { x[i], x[j] = x[j], x[i] }
func (x RectHorizontal) Less(i, j int) bool { return x[i].Min.X < x[j].Min.X }
func (x RectHorizontal) Len() int           { return len(x) }

func checkNoEncodings(num int, text []pdf.Text) {
	for _, t := range text {
		if match(t, "Helvetica-Bold", 9, "Encoding") {
			fmt.Fprintf(os.Stderr, "page %d: unexpected encoding: %s\n", num, t.S)
		}
	}
}

func match(t pdf.Text, font string, size float64, substr string) bool {
	return t.Font == font && math.Abs(t.FontSize-size) < 0.1 && strings.Contains(t.S, substr)
}

func findWords(chars []pdf.Text) (words []pdf.Text) {
	// Sort by Y coordinate and normalize.
	const nudge = 1
	sort.Sort(pdf.TextVertical(chars))
	old := -100000.0
	for i, c := range chars {
		if c.Y != old && math.Abs(old-c.Y) < nudge {
			chars[i].Y = old
		} else {
			old = c.Y
		}
	}

	// Sort by Y coordinate, breaking ties with X.
	// This will bring letters in a single word together.
	sort.Sort(pdf.TextVertical(chars))

	// Loop over chars.
	for i := 0; i < len(chars); {
		// Find all chars on line.
		j := i + 1
		for j < len(chars) && chars[j].Y == chars[i].Y {
			j++
		}
		var end float64
		// Split line into words (really, phrases).
		for k := i; k < j; {
			ck := &chars[k]
			s := ck.S
			end = ck.X + ck.W
			charSpace := ck.FontSize / 6
			wordSpace := ck.FontSize * 2 / 3
			l := k + 1
			for l < j {
				// Grow word.
				cl := &chars[l]
				if sameFont(cl.Font, ck.Font) && math.Abs(cl.FontSize-ck.FontSize) < 0.1 && cl.X <= end+charSpace {
					s += cl.S
					end = cl.X + cl.W
					l++
					continue
				}
				// Add space to phrase before next word.
				if sameFont(cl.Font, ck.Font) && math.Abs(cl.FontSize-ck.FontSize) < 0.1 && cl.X <= end+wordSpace {
					s += " " + cl.S
					end = cl.X + cl.W
					l++
					continue
				}
				break
			}
			f := ck.Font
			f = strings.TrimSuffix(f, ",Italic")
			f = strings.TrimSuffix(f, "-Italic")
			words = append(words, pdf.Text{f, ck.FontSize, ck.X, ck.Y, end - ck.X, s})
			k = l
		}
		i = j
	}

	return words
}

func sameFont(f1, f2 string) bool {
	f1 = strings.TrimSuffix(f1, ",Italic")
	f1 = strings.TrimSuffix(f1, "-Italic")
	f2 = strings.TrimSuffix(f1, ",Italic")
	f2 = strings.TrimSuffix(f1, "-Italic")
	return strings.TrimSuffix(f1, ",Italic") == strings.TrimSuffix(f2, ",Italic") || f1 == "Symbol" || f2 == "Symbol" || f1 == "TimesNewRoman" || f2 == "TimesNewRoman"
}

var jsFix = strings.NewReplacer(
//	`\u003c`, `<`,
//	`\u003e`, `>`,
//	`\u0026`, `&`,
//	`\u0009`, `\t`,
)

func printTable(name string, table []Inst) {
	_ = strconv.Atoi
}
