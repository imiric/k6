// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
This package is mostly a direct copy of the stdlib one, except for interface
changes and replacements for internal/ imports, so that we can reuse the local
DNS resolution logic (e.g. reading /etc/hosts, /etc/nsswitch.conf,
/etc/resolv.conf) and the cross-platform quirks it addresses, while having a
better handling of DNS caching and related scenarios which the stdlib doesn't
handle as well[1].
The suggested approach of using a custom net.DefaultResolver[2] wouldn't
easily work as the cgo resolver is forced on darwin, windows and other
platforms.

[1]: https://github.com/golang/go/issues/24796
[2]: https://github.com/golang/go/issues/24796#issuecomment-383716244
*/

package net

type AddrError struct {
	Err  string
	Addr string
}

func (e *AddrError) Error() string {
	if e == nil {
		return "<nil>"
	}
	s := e.Err
	if e.Addr != "" {
		s = "address " + e.Addr + ": " + s
	}
	return s
}

func (e *AddrError) Timeout() bool   { return false }
func (e *AddrError) Temporary() bool { return false }

// A ParseError is the error type of literal network address parsers.
type ParseError struct {
	// Type is the type of string that was expected, such as
	// "IP address", "CIDR address".
	Type string

	// Text is the malformed text string.
	Text string
}

func (e *ParseError) Error() string { return "invalid " + e.Type + ": " + e.Text }
