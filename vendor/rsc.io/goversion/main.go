// Copyright 2017 The Go Authors. All Rights Reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Goversion scans a directory tree and, for every executable it finds,
// prints the Go version used to build that executable.
//
// Usage:
//
//	goversion [-crypto] [-v] path...
//
// The list of paths can be individual files or directories; if the latter,
// goversion scans all files in the directory tree, not following symlinks.
//
// Goversion scans inside of tar or gzipped tar archives that it finds
// (named *.tar, *.tar.gz, or *.tgz), but not recursively.
//
// The -crypto flag causes goversion to print additional information
// about the crypto libraries linked into each executable.
//
// The -v flag causes goversion to print information about every
// file it considers.
//
// Example
//
// Scan /usr/bin for Go binaries and print their versions:
//
//	$ goversion /usr/bin
//	/usr/bin/containerd go1.7.4
//	/usr/bin/containerd-shim go1.7.4
//	/usr/bin/ctr go1.7.4
//	/usr/bin/docker go1.7.4
//	/usr/bin/docker-proxy go1.7.4
//	/usr/bin/dockerd go1.7.4
//	/usr/bin/kbfsfuse go1.8.3
//	/usr/bin/kbnm go1.8.3
//	/usr/bin/keybase go1.8.3
//	/usr/bin/snap go1.7.4
//	/usr/bin/snapctl go1.7.4
//
package main // import "rsc.io/goversion"

import (
	"archive/tar"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"rsc.io/goversion/version"
)

var (
	crypto  = flag.Bool("crypto", false, "check kind of crypto library")
	verbose = flag.Bool("v", false, "print verbose information")
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: goversion [-crypto] [-v] path...\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("goversion: ")
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() == 0 {
		usage()
	}

	for _, file := range flag.Args() {
		info, err := os.Stat(file)
		if err != nil {
			log.Print(err)
			continue
		}
		if info.IsDir() {
			scandir(file)
		} else {
			scanfile(file, file, info, true)
		}
	}
}

func scandir(dir string) {
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if info.Mode().IsRegular() {
			scanfile(path, path, info, *verbose)
		}
		return nil
	})
}

func scanfile(file, diskFile string, info os.FileInfo, mustPrint bool) {
	if strings.HasSuffix(file, ".tar") {
		if file != diskFile {
			fmt.Fprintf(os.Stderr, "%s: not scanning tar recursively\n", file)
			return
		}
		if mustPrint {
			fmt.Fprintf(os.Stderr, "%s: scanning tar archive\n", file)
		}
		scantar(file, info)
		return
	}
	if strings.HasSuffix(file, ".tar.gz") || strings.HasSuffix(file, ".tgz") {
		if file != diskFile {
			fmt.Fprintf(os.Stderr, "%s: not scanning tgz recursively\n", file)
			return
		}
		if mustPrint {
			fmt.Fprintf(os.Stderr, "%s: scanning tgz archive\n", file)
		}
		scantar(file, info)
		return
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if mustPrint {
			fmt.Fprintf(os.Stderr, "%s: symlink\n", file)
		}
		return
	}
	if file == diskFile && info.Mode()&0111 == 0 {
		if mustPrint {
			fmt.Fprintf(os.Stderr, "%s: not executable\n", file)
		}
		return
	}
	v, err := version.ReadExe(diskFile)
	if err != nil {
		if mustPrint {
			fmt.Fprintf(os.Stderr, "%s: %v\n", file, err)
		}
		return
	}

	buildVersion := v.Release
	if *crypto {
		switch {
		case v.BoringCrypto && v.StandardCrypto:
			buildVersion += " (boring AND standard crypto!!!)"
		case v.BoringCrypto:
			buildVersion += " (boring crypto)"
		case v.StandardCrypto:
			buildVersion += " (standard crypto)"
		}
	}
	fmt.Printf("%s %s\n", file, buildVersion)
}

type Version struct {
	Release        string
	BoringCrypto   bool
	StandardCrypto bool
}

func scantar(file string, info os.FileInfo) {
	f, err := os.Open(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", file, err)
		return
	}
	defer f.Close()
	var r io.Reader = f
	if strings.HasSuffix(file, "z") {
		z, err := gzip.NewReader(r)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", file, err)
			return
		}
		defer z.Close()
		r = z
	}
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		if hdr.Typeflag != tar.TypeReg {
			if *verbose {
				fmt.Fprintf(os.Stderr, "%s/%s: not regular file\n", file, hdr.Name)
			}
			continue
		}
		if hdr.Mode&0111 == 0 {
			if *verbose {
				fmt.Fprintf(os.Stderr, "%s/%s: not executable\n", file, hdr.Name)
			}
			continue
		}

		// executable but not special
		tmp, err := ioutil.TempFile("", "goversion-")
		if err != nil {
			log.Fatal(err)
		}
		io.Copy(tmp, tr)
		tmpName := tmp.Name()
		info, err := tmp.Stat()
		if err != nil {
			log.Fatal(err)
		}
		tmp.Close()
		scanfile(file+"/"+hdr.Name, tmpName, info, *verbose)
		os.Remove(tmpName)
	}
}
