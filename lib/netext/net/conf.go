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
type Conf struct {
	// machine has an /etc/mdns.allow file
	HasMDNSAllow bool

	goos string // the runtime.GOOS, to ease testing

	NSS    *NSSConf
	Resolv *DNSConfig
}

var (
	confOnce sync.Once // guards init of confVal via initConfVal
	confVal  = &Conf{goos: runtime.GOOS}
)

// SystemConf returns the machine's network configuration.
func SystemConf() *Conf {
	confOnce.Do(initConfVal)
	return confVal
}

func initConfVal() {
	if runtime.GOOS != "openbsd" {
		confVal.NSS = parseNSSConfFile("/etc/nsswitch.conf")
	}

	confVal.Resolv = dnsReadConfig("/etc/resolv.conf")

	if _, err := os.Stat("/etc/mdns.allow"); err == nil {
		confVal.HasMDNSAllow = true
	}
}
