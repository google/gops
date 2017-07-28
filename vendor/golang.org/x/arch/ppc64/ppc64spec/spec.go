// Copyright 2014 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.6,amd64 go1.8

// Power64spec reads the ``Power ISA V2.07'' Manual
// to collect instruction encoding details and writes those details to standard output
// in CSV format.
//
// Usage:
//	ppc64spec PowerISA_V2.07_PUBLIC.pdf >ppc64.csv
//
// Each CSV line contains four fields:
//
//	instruction
//		The instruction heading, such as "AAD imm8".
//	mnemonic
//		The instruction mnemonics, separated by | symbols.
//	encoding
//		The instruction encoding, a sequence of name@startbit| describing each bit field in turn.
//	tags
//		For now, empty.
//
// For more on the exact meaning of these fields, see the Power manual.
//
package main

import (
	"bufio"
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
	Name string
	Text string
	Enc  string
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

	var all = []Inst{
		// Split across multiple columns and pages!
		{"Count Leading Zeros Word X-form", "cntlzw RA, RS (Rc=0)\ncntlzw. RA, RS (Rc=1)", "31@0|RS@6|RA@11|///@16|26@21|Rc@31|"},
	}

	for j, headline := range instList {
		for _, inst := range all {
			if headline == inst.Name {
				instList[j] = ""
				break
			}
		}
	}

	// Scan document looking for instructions.
	// Must find exactly the ones in the outline.
	n := f.NumPage()
	for pageNum := 1; pageNum <= n; pageNum++ {
		if debugPage > 0 && pageNum != debugPage {
			continue
		}
		p := f.Page(pageNum)
		table := parsePage(pageNum, p)
		if len(table) == 0 {
			continue
		}
	InstLoop:
		for _, inst := range table {
			for j, headline := range instList {
				if inst.Name == headline {
					instList[j] = ""
					continue InstLoop
				}
			}
			fmt.Fprintf(os.Stderr, "page %d: unexpected instruction %q\n", pageNum, inst.Name)
		}
		all = append(all, table...)
	}

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

	stdout = bufio.NewWriter(os.Stdout)
	for _, inst := range all {
		fmt.Fprintf(stdout, "%q,%q,%q,%q\n", inst.Name, strings.Replace(inst.Text, "\n", "|", -1), inst.Enc, "")
	}
	stdout.Flush()

}

func instHeadings(outline pdf.Outline) []string {
	return appendInstHeadings(outline, nil)
}

var instRE = regexp.MustCompile(` ([A-Z0-9]+-form|Byte|Word|Doubleword|Halfword)($| \[)`)
var sectionRE = regexp.MustCompile(`^[0-9A-Z]+\.[0-9]`)

func appendInstHeadings(outline pdf.Outline, list []string) []string {
	if strings.Contains(outline.Title, "Variable Length Encoding (VLE) Encoding") {
		for _, child := range outline.Child {
			vle = appendInstHeadings(child, vle)
		}
		return list
	}
	if instRE.MatchString(outline.Title) && !sectionRE.MatchString(outline.Title) {
		list = append(list, outline.Title)
	}
	if outline.Title == "Transaction Abort Word Conditional" {
		list = append(list, outline.Title+" X-form")
	}
	for _, child := range outline.Child {
		list = appendInstHeadings(child, list)
	}
	return list
}

const inch = 72.0

func parsePage(num int, p pdf.Page) []Inst {
	content := p.Content()

	var text []pdf.Text
	for _, t := range content.Text {
		text = append(text, t)
	}

	text = findWords(text)

	if debugPage > 0 {
		for _, t := range text {
			fmt.Println(t)
		}
		for _, r := range content.Rect {
			fmt.Println(r)
		}
	}

	// Look for instruction encodings.
	// Some begin with a Helvetica-BoldOblique size 11 headline like "AND   X-Form",
	// is followed by Helvetica 9 mnemonic, and then a bit box with
	// Helvetica 9 fields and Helvetica 7 bit offsets.
	// Others use Arial,BoldItalic 11 for the headline,
	// Arial 8 for the mnemonic, and Arial 4.2 for the bit offsets.

	var insts []Inst
	for {
		// Heading
		for len(text) > 0 && !match(text[0], "Helvetica-BoldOblique", 11, "") && !match(text[0], "Arial,BoldItalic", 11, "") && !match(text[0], "Arial,BoldItalic", 10, "") {
			text = text[1:]
		}
		if len(text) == 0 {
			break
		}
		heading := text[0].S
		text = text[1:]
		for len(text) > 0 && (match(text[0], "Helvetica-BoldOblique", 11, "") || match(text[0], "Arial,BoldItalic", 11, "") || match(text[0], "Arial,BoldItalic", 10, "")) {
			heading += " " + text[0].S
			text = text[1:]
		}
		heading = strings.Replace(heading, "]", "] ", -1)
		heading = strings.Replace(heading, "  ", " ", -1)
		heading = strings.Replace(heading, "rEVX-form", "r EVX-form", -1)
		heading = strings.Replace(heading, "eX-form", "e X-form", -1)
		heading = strings.Replace(heading, "mSD4-form", "m SD4-form", -1)
		heading = strings.Replace(heading, "eSCI8-form", "e SCI8-form", -1)
		heading = strings.TrimSpace(heading)
		if isVLE(heading) {
			continue
		}

		// Mnemonic
		if len(text) == 0 || (!match(text[0], "Helvetica", 9, "") && !match(text[0], "Helvetica-BoldOblique", 9, "") && !match(text[0], "Arial", 9, "") && !match(text[0], "Arial", 10, "")) {
			continue
		}
		mnemonic := ""
		y := text[0].Y
		x0 := text[0].X
		for len(text) > 0 && (match(text[0], "Helvetica", 9, "") || match(text[0], "Helvetica-BoldOblique", 9, "") || match(text[0], "Arial", 9, "") || match(text[0], "Courier", 8, "") || match(text[0], "LucidaConsole", 7.17, "") || text[0].Y == y) {
			if text[0].Y != y {
				if math.Abs(text[0].X-x0) > 4 {
					break
				}
				mnemonic += "\n"
				y = text[0].Y
			} else if mnemonic != "" {
				mnemonic += " "
			}
			mnemonic += text[0].S
			text = text[1:]
		}

		// Encoding
		bits, i := readBitBox(heading, content, text, num)
		if i == 0 {
			continue
		}

		insts = append(insts, Inst{heading, mnemonic, bits})
	}
	return insts
}

var vle = []string{
	"System Call C-form,ESC-form",
}

func isVLE(s string) bool {
	for _, v := range vle {
		if s == v {
			return true
		}
	}
	return false
}

func readBitBox(headline string, content pdf.Content, text []pdf.Text, pageNum int) (string, int) {
	// fields
	i := 0
	if len(text) == 0 || (!match(text[i], "Helvetica", 9, "") && !match(text[i], "Helvetica", 7.26, "") && !match(text[i], "Arial", 9, "") && !match(text[i], "Arial", 7.98, "") && !match(text[i], "Arial", 7.2, "")) {
		fmt.Fprintf(os.Stderr, "page %d: no bit fields for %q\n", pageNum, headline)
		if len(text) > 0 {
			fmt.Fprintf(os.Stderr, "\tlast text: %v\n", text[0])
		}
		return "", 0
	}
	sz := text[i].FontSize
	y2 := text[i].Y
	x2 := 0.0
	for i < len(text) && text[i].Y == y2 {
		if x2 < text[i].X+text[i].W {
			x2 = text[i].X + text[i].W
		}
		i++
	}
	y2 += sz / 2

	// bit numbers
	if i >= len(text) || text[i].S != "0" {
		if headline == "Transaction Abort Doubleword Conditional X-form" {
			// Split across the next page.
			return "31@0|TO@6|RA@11|RB@16|814@21|1@31|", i
		}
		if headline == "Add Scaled Immediate SCI8-form" {
			// Very strange fonts.
			return "06@0|RT@6|RA@11|8@16|Rc@20|F@21|SCL@22|UI8@24|", i
		}
		fmt.Fprintf(os.Stderr, "page %d: no bit numbers for %s\n", pageNum, headline)
		if i < len(text) {
			fmt.Fprintf(os.Stderr, "\tlast text: %v\n", text[i])
		}
		return "", 0
	}
	sz = text[i].FontSize
	y1 := text[i].Y
	x1 := text[i].X
	for i < len(text) && text[i].Y == y1 {
		if x2 < text[i].X+text[i].W {
			x2 = text[i].X + text[i].W
		}
		i++
	}

	if debugPage > 0 {
		fmt.Println("encoding box", x1, y1, x2, y2, i, text[0], text[i])
	}

	// Find lines (thin rectangles) separating bit fields.
	var bottom, top pdf.Rect
	const (
		yMargin = 0.25 * 72
		xMargin = 1 * 72
	)
	for _, r := range content.Rect {
		// Only consider lines in the same column.
		if (x1 < 306) != (r.Max.X < 306) {
			continue
		}
		if r.Max.Y-r.Min.Y < 2 && x1-xMargin < r.Min.X && r.Min.X < x1 && x2 < r.Max.X && r.Max.X < x2+xMargin {
			if y1-yMargin < r.Min.Y && r.Min.Y < y1 {
				bottom = r
			}
			if y2 < r.Min.Y && r.Min.Y < y2+8 {
				top = r
			}
		}
	}

	if bottom.Min.X == 0 {
		// maybe bit numbers are outside box; see doze, nap, sleep, rvwinkle.
		for _, r := range content.Rect {
			// Only consider lines in the same column.
			if (x1 < 306) != (r.Max.X < 306) {
				continue
			}
			if r.Max.Y-r.Min.Y < 2 && x1-xMargin < r.Min.X && r.Min.X < x1 && x2 < r.Max.X && r.Max.X < x2+xMargin {
				if y1+sz/2 < r.Min.Y && r.Min.Y < y2 {
					bottom = r
				}
			}
		}
	}

	if debugPage > 0 {
		fmt.Println("top", top, "bottom", bottom)
	}

	const ε = 0.1 * 72
	var bars []pdf.Rect
	for _, r := range content.Rect {
		if r.Max.X-r.Min.X < 2 && math.Abs(r.Min.Y-bottom.Min.Y) < ε && math.Abs(r.Max.Y-top.Min.Y) < ε && (bottom.Min.X < 306) == (r.Max.X < 306) {
			bars = append(bars, r)
		}
	}
	sort.Sort(RectHorizontal(bars))

	out := ""
	for i := 0; i < len(bars)-1; i++ {
		var sub []pdf.Text
		x1, x2 := bars[i].Min.X, bars[i+1].Min.X
		for _, t := range content.Text {
			tx := t.X + t.W/2
			ty := t.Y + t.FontSize/4
			if x1 < tx && tx < x2 && y1 < ty && ty < y2 {
				sub = append(sub, t)
			}
		}
		var str []string
		for _, t := range findWords(sub) {
			str = append(str, t.S)
		}
		s := strings.Join(str, "@")
		out += s + "|"
	}

	if out == "" {
		fmt.Fprintf(os.Stderr, "page %d: no bit encodings for %s\n", pageNum, headline)
	}
	return out, i
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
	return t.Font == font && (size == 0 || math.Abs(t.FontSize-size) < 0.1) && strings.Contains(t.S, substr)
}

func findWords(chars []pdf.Text) (words []pdf.Text) {
	// Sort by Y coordinate and normalize.
	const nudge = 1.5
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

	// Split into two columns.
	var col1, col2 []pdf.Text
	for _, w := range words {
		if w.X > 306 {
			col2 = append(col2, w)
		} else {
			col1 = append(col1, w)
		}
	}
	return append(col1, col2...)
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
