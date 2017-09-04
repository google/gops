// Copyright 2017 The Go Authors. All Rights Reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package version reports the Go version used to build program executables.
package version

import (
	"errors"
	"fmt"
	"strings"
)

// Version is the information reported by ReadExe.
type Version struct {
	Release        string // Go version (runtime.Version in the program)
	BoringCrypto   bool   // program uses BoringCrypto
	StandardCrypto bool   // program uses standard crypto (replaced by BoringCrypto)
}

// ReadExe reports information about the Go version used to build
// the program executable named by file.
func ReadExe(file string) (Version, error) {
	var v Version
	f, err := openExe(file)
	if err != nil {
		return v, err
	}
	defer f.Close()
	isGo := false
	for _, name := range f.SectionNames() {
		if name == ".note.go.buildid" {
			isGo = true
		}
	}
	syms, symsErr := f.Symbols()
	isGccgo := false
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
			release, err := readBuildVersion(f, sym.Addr, sym.Size)
			if err != nil {
				return v, err
			}
			v.Release = release

		}
		if strings.Contains(name, "_Cfunc__goboringcrypto_") {
			v.BoringCrypto = true
		}
		for _, s := range standardCryptoNames {
			if strings.Contains(name, s) {
				v.StandardCrypto = true
			}
		}
	}

	if *debugMatch {
		v.Release = ""
	}
	if v.Release == "" {
		g, release := readBuildVersionX86Asm(f)
		if g {
			isGo = true
			v.Release = release
		}
	}
	if isGccgo && v.Release == "" {
		isGo = true
		v.Release = "gccgo (version unknown)"
	}
	if !isGo && symsErr != nil {
		return v, symsErr
	}

	if !isGo {
		return v, errors.New("not a Go executable")
	}
	if v.Release == "" {
		v.Release = "unknown Go version"
	}
	return v, nil
}

var standardCryptoNames = []string{
	"crypto/sha1.(*digest)",
	"crypto/sha256.(*digest)",
	"crypto/rand.(*devReader)",
	"crypto/rsa.encrypt",
	"crypto/rsa.decrypt",
}

func readBuildVersion(f exe, addr, size uint64) (string, error) {
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
