// Copyright 2017 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build go1.6,amd64 go1.8

// arm64spec reads the ``ARMv8-A Reference Manual''
// to collect instruction encoding details and writes those
// details to standard output in JSON format.
// usage: arm64spec file.pdf

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
	Bits   string
	Arch   string
	Syntax string
	Code   string
	Alias  string
}

const debugPage = 0

var stdout *bufio.Writer

func check(e error) {
	if e != nil {
		panic(e)
	}
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("arm64spec: ")

	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "usage: arm64spec file.pdf\n")
		os.Exit(2)
	}
	f, err := pdf.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}

	// Find instruction set reference in outline, to build instruction list.
	instList := instHeadings(f.Outline())
	if debugPage == 0 {
		fmt.Println("the number of instructions:", len(instList))
	}
	if len(instList) < 200 {
		log.Fatalf("only found %d instructions in table of contents", len(instList))
	}

	file, err := os.Create("inst.json")
	check(err)
	w := bufio.NewWriter(file)
	_, err = w.WriteString("[")
	check(err)
	numTable := 0
	defer w.Flush()
	defer file.Close()

	// Scan document looking for instructions.
	// Must find exactly the ones in the outline.
	n := f.NumPage()
PageLoop:
	for pageNum := 435; pageNum <= n; pageNum++ {
		if debugPage > 0 && pageNum != debugPage {
			continue
		}
		if pageNum == 770 {
			continue
		}
		if pageNum > 1495 {
			break
		}
		p := f.Page(pageNum)
		name, table := parsePage(pageNum, p, f)
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
				_, err = w.WriteString(jsFix.Replace(","))
				check(err)
				_, err = w.WriteString("\n")
				check(err)
			}
			numTable++
			js, _ := json.Marshal(inst)
			_, err = w.WriteString(jsFix.Replace(string(js)))
			check(err)
		}
		for j, headline := range instList {
			if name == headline {
				instList[j] = ""
				continue PageLoop
			}
		}
		fmt.Fprintf(os.Stderr, "unexpected instruction %q (page %d)\n", name, pageNum)
	}

	_, err = w.WriteString("\n]\n")
	check(err)
	w.Flush()

	if debugPage == 0 {
		for _, headline := range instList {
			if headline != "" {
				fmt.Fprintf(os.Stderr, "missing instruction %q\n", headline)
			}
		}
	}
}

func instHeadings(outline pdf.Outline) []string {
	return appendInstHeadings(outline, nil)
}

var instRE = regexp.MustCompile(`C[\d.]+ Alphabetical list of A64 base instructions`)
var instRE_A = regexp.MustCompile(`C[\d.]+ Alphabetical list of A64 floating-point and Advanced SIMD instructions`)
var childRE = regexp.MustCompile(`C[\d.]+ (.+)`)
var sectionRE = regexp.MustCompile(`^C[\d.]+$`)
var bitRE = regexp.MustCompile(`^( |[01]|\([01]\))*$`)
var IMMRE = regexp.MustCompile(`^imm[\d]+$`)

func appendInstHeadings(outline pdf.Outline, list []string) []string {
	if instRE.MatchString(outline.Title) || instRE_A.MatchString(outline.Title) {
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

func parsePage(num int, p pdf.Page, f *pdf.Reader) (name string, table []Inst) {
	content := p.Content()
	var text []pdf.Text
	CrossTwoPage := true
	for _, t := range content.Text {
		text = append(text, t)
	}
	text = findWords(text)
	if !(instRE.MatchString(text[1].S) || instRE_A.MatchString(text[1].S)) || len(text) == 0 || !sectionRE.MatchString(text[2].S) {
		return "", nil
	}
	// Check whether the content crosses the page.
	for _, t := range text {
		if match(t, "Arial,Bold", 10, "Assembler symbols") {
			CrossTwoPage = false
			break
		}
	}
	// Deal with cross page issue. To the next page content.
	var Ncontent pdf.Content
	Npagebox := false
	CrossThreePage := false
	Noffset := ""
	if CrossTwoPage == true {
		Np := f.Page(num + 1)
		Ncontent = Np.Content()
		var Ntext []pdf.Text
		for _, t := range Ncontent.Text {
			Ntext = append(Ntext, t)
		}
		Ntext = findWords(Ntext)
		if len(Ntext) == 0 || sectionRE.MatchString(Ntext[2].S) {
			Ntext = text[:0]
		} else {
			for _, t := range Ntext {
				if match(t, "Arial,Bold", 10, "offset") {
					Noffset = t.S
					Npagebox = true
				}
				// This istruction cross three pages.
				if match(t, "Arial,Bold", 10, "Assembler symbols") {
					CrossThreePage = false
				} else {
					CrossThreePage = true
				}
				text = append(text, t)
			}
		}
	}
	if CrossThreePage == true {
		NNp := f.Page(num + 2)
		NNcontent := NNp.Content()
		var NNtext []pdf.Text
		for _, t := range NNcontent.Text {
			NNtext = append(NNtext, t)
		}
		NNtext = findWords(NNtext)
		if len(NNtext) == 0 || sectionRE.MatchString(NNtext[2].S) {
			NNtext = text[:0]
		} else {
			for _, t := range NNtext {
				text = append(text, t)
			}
		}
	}
	// Get alias and remove text we should ignore.
	out := text[:0]
	alias := ""
	for _, t := range text {
		if strings.Contains(t.S, "instruction is used by the alias") || strings.Contains(t.S, "instruction is an alias of") {
			alias_t := strings.SplitAfter(t.S, ".")
			alias = alias_t[0]
		}
		// Skip page footer
		if match(t, "Arial-ItalicMT", 8, "") || match(t, "ArialMT", 8, "") {
			if debugPage > 0 {
				fmt.Println("==the skip page footer is:==", t)
			}
			continue
		}
		// Skip the body text
		if match(t, "TimesNewRoman", 9, "") || match(t, "TimesNewRomanPS-ItalicMT", 9, "") {
			if debugPage > 0 {
				fmt.Println("==the skip body text is:==", t)
			}
			continue
		}
		out = append(out, t)
	}
	text = out
	// Page header must be child title.
	if len(text) == 0 || !sectionRE.MatchString(text[0].S) {
		return "", nil
	}

	name = text[1].S
	inst := Inst{
		Name:  name,
		Alias: alias,
	}
	text = text[2:]
	// Skip body text before bits.
	OffsetMark := false
	k := 0
	for k = 0; k < len(text); {
		if !match(text[k], "Arial", 8, "31") {
			k++
		} else {
			break
		}
	}
	// Check offset.
	if k > 0 && match(text[k-1], "Arial,Bold", 10, "") {
		OffsetMark = true
		text = text[k-1:]
	} else {
		text = text[k:]
	}
	// Encodings follow.
	BitMark := false
	bits := ""
	// Find bits.
	for i := 0; i < len(text); {
		inst.Bits = ""
		offset := ""
		abits := ""
		// Read bits only one time.
		if OffsetMark == true {
			for i < len(text) && !match(text[i], "Arial", 8, "") {
				i++
			}
			if i < len(text) {
				offset = text[i-1].S
				BitMark = false
				bits = ""
			} else {
				break
			}
		}
		if BitMark == false {
			if Npagebox == true && Noffset == offset {
				bits, i = readBitBox(name, Ncontent, text, i)
			} else {
				bits, i = readBitBox(name, content, text, i)
			}
			BitMark = true
			// Every time, get "then SEE" after get bits.
			enc := false
			if i < len(text)-1 {
				m := i
				for m < len(text)-1 && !match(text[m], "Arial-BoldItalicMT", 9, "encoding") {
					m++
				}
				if match(text[m], "Arial-BoldItalicMT", 9, "encoding") && m < len(text) {
					enc = true
					m = m + 1
				}
				if enc == true {
					for m < len(text) && !match(text[m], "Arial,Bold", 10, "") && match(text[m], "LucidaSansTypewriteX", 6.48, "") {
						if strings.Contains(text[m].S, "then SEE") {
							inst.Code = text[m].S
							break
						} else {
							m++
						}
					}
				}
			}
		}

		// Possible subarchitecture notes.
	ArchLoop:
		for i < len(text) {
			if !match(text[i], "Arial-BoldItalicMT", 9, "variant") || match(text[i], "Arial-BoldItalicMT", 9, "encoding") {
				i++
				continue
			}
			inst.Arch = ""
			inst.Arch += offset
			inst.Arch += " "
			inst.Arch += text[i].S
			inst.Arch = strings.TrimSpace(inst.Arch)
			i++
			// Encoding syntaxes.
			sign := ""
			SynMark := false
			for i < len(text) && match(text[i], "LucidaSansTypewriteX", 6.48, "") && SynMark == false {
				if (strings.Contains(text[i].S, "==") || strings.Contains(text[i].S, "!=")) && SynMark == false {
					sign = text[i].S
					i++
					continue
				}
				// Avoid "equivalent to" another syntax.
				if SynMark == false {
					SynMark = true
					inst.Syntax = ""
					inst.Syntax = text[i].S
					i++
				}
			}
			abits = bits
			// Analyse and replace some bits value.eg, sf==1
			if strings.Contains(sign, "&&") {
				split := strings.Split(sign, "&&")
				for k := 0; k < len(split); {
					if strings.Contains(split[k], "==") && !strings.Contains(split[k], "!") {
						tmp := strings.Split(split[k], "==")
						prefix := strings.TrimSpace(tmp[0])
						value := strings.TrimSpace(tmp[1])
						if strings.Contains(bits, prefix) && !strings.Contains(value, "x") {
							abits = strings.Replace(abits, prefix, value, -1)
						}
					}
					k++
				}
			} else if strings.Contains(sign, "==") && !strings.Contains(sign, "!") {
				split := strings.Split(sign, "==")
				prefix := strings.TrimSpace(split[0])
				value := strings.TrimSpace(split[1])
				if strings.Contains(bits, prefix) && !strings.Contains(value, "x") {
					abits = strings.Replace(abits, prefix, value, -1)
				}
			}
			// Deal with syntax contains {2}
			if strings.Contains(inst.Syntax, "{2}") {
				if !strings.Contains(abits, "Q") {
					fmt.Fprintf(os.Stderr, "instruction%s - syntax%s: is wrong!!\n", name, inst.Syntax)
				}
				syn := inst.Syntax
				bits := abits
				for i := 0; i < 2; {
					if i == 0 {
						inst.Bits = strings.Replace(bits, "Q", "0", -1)
						inst.Syntax = strings.Replace(syn, "{2}", "", -1)
						table = append(table, inst)
					}
					if i == 1 {
						inst.Bits = strings.Replace(bits, "Q", "1", -1)
						inst.Syntax = strings.Replace(syn, "{2}", "2", -1)
						table = append(table, inst)
					}
					i++
				}
			} else {
				inst.Bits = abits
				table = append(table, inst)
			}

			if OffsetMark == true && i < len(text) && match(text[i], "Arial-BoldItalicMT", 9, "variant") && !match(text[i], "Arial-BoldItalicMT", 9, "encoding") {
				continue ArchLoop
			} else {
				break
			}
		}
	}
	return name, table
}

func readBitBox(name string, content pdf.Content, text []pdf.Text, i int) (string, int) {
	// Bits headings
	y3 := 0.0
	x1 := 0.0
	for i < len(text) && match(text[i], "Arial", 8, "") {
		if y3 == 0 {
			y3 = text[i].Y
		}
		if x1 == 0 {
			x1 = text[i].X
		}
		if text[i].Y != y3 {
			break
		}
		i++
	}
	// Bits fields in box
	x2 := 0.0
	y2 := 0.0
	dy1 := 0.0
	for i < len(text) && match(text[i], "Arial", 8, "") {
		if x2 < text[i].X+text[i].W {
			x2 = text[i].X + text[i].W
		}
		if y2 == 0 {
			y2 = text[i].Y
		}
		if text[i].Y != y2 {
			break
		}
		dy1 = text[i].FontSize
		i++
	}
	// Bits fields below box
	x3 := 0.0
	y1 := 0.0
	for i < len(text) && match(text[i], "Arial", 8, "") {
		if x3 < text[i].X+text[i].W {
			x3 = text[i].X + text[i].W
		}
		y1 = text[i].Y
		if text[i].Y != y1 {
			break
		}
		i++
	}
	//no bits fields below box
	below_flag := true
	if y1 == 0.0 {
		below_flag = false
		y1 = y2
	}
	// Encoding box
	if debugPage > 0 {
		fmt.Println("encoding box", x1, y3, x2, y1)
	}

	// Find lines (thin rectangles) separating bit fields.
	var bottom, top pdf.Rect
	const (
		yMargin = 0.25 * 72
		xMargin = 2 * 72
	)
	cont := 0
	if below_flag == true {
		for _, r := range content.Rect {
			cont = cont + 1
			if x1-xMargin < r.Min.X && r.Min.X < x1 && x2 < r.Max.X && r.Max.X < x2+xMargin {
				if y1-yMargin < r.Min.Y && r.Min.Y < y2-dy1 {
					bottom = r
				}
				if y2+dy1 < r.Min.Y && r.Min.Y < y3+yMargin {
					top = r
				}
			}
		}
	} else {
		for _, r := range content.Rect {
			cont = cont + 1
			if x1-xMargin < r.Min.X && r.Min.X < x1 && x2 < r.Max.X && r.Max.X < x2+xMargin {
				if y1-yMargin-dy1 < r.Min.Y && r.Min.Y < y3-dy1 {
					bottom = r
				}
				if y2+dy1 < r.Min.Y && r.Min.Y < y3+yMargin {
					top = r
				}
			}
		}
	}

	if debugPage > 0 {
		fmt.Println("top", top, "bottom", bottom, "content.Rect number", cont)
	}

	const ε = 0.5 * 72
	cont_1 := 0
	var bars []pdf.Rect
	for _, r := range content.Rect {
		if math.Abs(r.Min.X-r.Max.X) < bottom.Max.X-bottom.Min.X-(ε/2) && math.Abs(r.Min.Y-bottom.Min.Y) < ε && math.Abs(r.Max.Y-top.Min.Y) < ε {
			cont_1 = cont_1 + 1
			bars = append(bars, r)
		}
	}
	sort.Sort(RectHorizontal(bars))
	if debugPage > 0 {
		fmt.Println("==bars number==", cont_1)
	}

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
	for i := 0; i < len(bars); i++ {
		if i > 0 {
			fmt.Fprintf(&buf, "|")
		}
		var sub []pdf.Text
		x1, x2 := bars[i].Min.X, bars[i].Max.X
		for _, t := range content.Text {
			tx := t.X + t.W/2
			ty := t.Y
			if x1 < tx && tx < x2 && y2-dy1 < ty && ty < y2+dy1 {
				sub = append(sub, t)
			}
		}
		var str []string
		for _, t := range findWords(sub) {
			str = append(str, t.S)
		}
		s := strings.Join(str, " ")
		s = strings.Replace(s, ")(", ") (", -1)

		// If bits contain "!" or "x", be replaced by the bits below it.
		if strings.Contains(s, "!") || strings.Contains(s, "x") {
			var sub1 []pdf.Text
			for _, t := range content.Text {
				tx := t.X + t.W/2
				ty := t.Y
				if x1 < tx && tx < x2 && y1-dy1 < ty && ty < y1+dy1 {
					sub1 = append(sub1, t)
				}

			}
			var str1 []string
			for _, t := range findWords(sub1) {
				str1 = append(str1, t.S)
			}
			s = strings.Join(str1, " ")
			s = strings.Replace(s, ")(", ") (", -1)
		}

		n := len(strings.Fields(s))

		var b int
		if IMMRE.MatchString(s) {
			bitNum := strings.TrimPrefix(s, "imm")
			b, _ = strconv.Atoi(bitNum)
		} else if s == "immhi" {
			b = 19
		} else {
			b = int(float64(nbit)*(x2-x1)/dx + 0.5)
		}
		if n == b {
			for k, f := range strings.Fields(s) {
				if k > 0 {
					fmt.Fprintf(&buf, "|")
				}
				fmt.Fprintf(&buf, "%s", f)
			}
		} else {
			if n != 1 {
				fmt.Fprintf(os.Stderr, "%s - multi-field %d-bit encoding: %s\n", name, n, s)
			}
			fmt.Fprintf(&buf, "%s:%d", s, b)
		}
		total += b
	}

	if total != nbit || total == 0 {
		fmt.Fprintf(os.Stderr, "%s - %d-bit encoding\n", name, total)
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
	`\u003c`, `<`,
	`\u003e`, `>`,
	`\u0026`, `&`,
	`\u0009`, `\t`,
)

func printTable(name string, table []Inst) {
	_ = strconv.Atoi
}
