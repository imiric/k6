// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build aix darwin dragonfly freebsd linux netbsd openbsd solaris

package net

import (
	"os"
	"runtime"
	"sync"
)

// conf represents a system's network configuration.
type conf struct {
	// machine has an /etc/mdns.allow file
	hasMDNSAllow bool

	goos string // the runtime.GOOS, to ease testing

	nss    *nssConf
	resolv *dnsConfig
}

var (
	confOnce sync.Once // guards init of confVal via initConfVal
	confVal  = &conf{goos: runtime.GOOS}
)

// systemConf returns the machine's network configuration.
func systemConf() *conf {
	confOnce.Do(initConfVal)
	return confVal
}

func initConfVal() {
	if runtime.GOOS != "openbsd" {
		confVal.nss = parseNSSConfFile("/etc/nsswitch.conf")
	}

	confVal.resolv = dnsReadConfig("/etc/resolv.conf")
	// TODO: Handle any errors
	// if confVal.resolv.err != nil && !os.IsNotExist(confVal.resolv.err) &&
	// 	!os.IsPermission(confVal.resolv.err) {
	// 	// If we can't read the resolv.conf file, assume it
	// 	// had something important in it and defer to cgo.
	// 	// libc's resolver might then fail too, but at least
	// 	// it wasn't our fault.
	// 	confVal.forceCgoLookupHost = true
	// }

	if _, err := os.Stat("/etc/mdns.allow"); err == nil {
		confVal.hasMDNSAllow = true
	}
}
