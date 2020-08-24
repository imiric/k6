// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build aix darwin dragonfly freebsd linux netbsd openbsd solaris

package net

import (
	bytealg "bytes"
	"errors"
	"io"
	"os"
)

// NSSConf represents the state of the machine's /etc/nsswitch.conf file.
type NSSConf struct {
	Err     error                  // any error encountered opening or parsing the file
	Sources map[string][]NSSSource // keyed by database (e.g. "hosts")
}

type NSSSource struct {
	Source   string // e.g. "compat", "files", "mdns4_minimal"
	Criteria []NSSCriterion
}

// standardCriteria reports all specified criteria have the default
// status actions.
func (s NSSSource) standardCriteria() bool {
	for i, crit := range s.Criteria {
		if !crit.standardStatusAction(i == len(s.Criteria)-1) {
			return false
		}
	}
	return true
}

// NSSCriterion is the parsed structure of one of the criteria in brackets
// after an NSS source name.
type NSSCriterion struct {
	negate bool   // if "!" was present
	status string // e.g. "success", "unavail" (lowercase)
	action string // e.g. "return", "continue" (lowercase)
}

// standardStatusAction reports whether c is equivalent to not
// specifying the criterion at all. last is whether this criteria is the
// last in the list.
func (c NSSCriterion) standardStatusAction(last bool) bool {
	if c.negate {
		return false
	}
	var def string
	switch c.status {
	case "success":
		def = "return"
	case "notfound", "unavail", "tryagain":
		def = "continue"
	default:
		// Unknown status
		return false
	}
	if last && c.action == "return" {
		return true
	}
	return c.action == def
}

func parseNSSConfFile(file string) *NSSConf {
	f, err := os.Open(file)
	if err != nil {
		return &NSSConf{Err: err}
	}
	defer f.Close()
	return parseNSSConf(f)
}

func parseNSSConf(r io.Reader) *NSSConf {
	slurp, err := readFull(r)
	if err != nil {
		return &NSSConf{Err: err}
	}
	conf := new(NSSConf)
	conf.Err = foreachLine(slurp, func(line []byte) error {
		line = trimSpace(removeComment(line))
		if len(line) == 0 {
			return nil
		}
		colon := bytealg.IndexByte(line, ':')
		if colon == -1 {
			return errors.New("no colon on line")
		}
		db := string(trimSpace(line[:colon]))
		srcs := line[colon+1:]
		for {
			srcs = trimSpace(srcs)
			if len(srcs) == 0 {
				break
			}
			sp := bytealg.IndexByte(srcs, ' ')
			var src string
			if sp == -1 {
				src = string(srcs)
				srcs = nil // done
			} else {
				src = string(srcs[:sp])
				srcs = trimSpace(srcs[sp+1:])
			}
			var criteria []NSSCriterion
			// See if there's a criteria block in brackets.
			if len(srcs) > 0 && srcs[0] == '[' {
				bclose := bytealg.IndexByte(srcs, ']')
				if bclose == -1 {
					return errors.New("unclosed criterion bracket")
				}
				var err error
				criteria, err = parseCriteria(srcs[1:bclose])
				if err != nil {
					return errors.New("invalid criteria: " + string(srcs[1:bclose]))
				}
				srcs = srcs[bclose+1:]
			}
			if conf.Sources == nil {
				conf.Sources = make(map[string][]NSSSource)
			}
			conf.Sources[db] = append(conf.Sources[db], NSSSource{
				Source:   src,
				Criteria: criteria,
			})
		}
		return nil
	})
	return conf
}

// parses "foo=bar !foo=bar"
func parseCriteria(x []byte) (c []NSSCriterion, err error) {
	err = foreachField(x, func(f []byte) error {
		not := false
		if len(f) > 0 && f[0] == '!' {
			not = true
			f = f[1:]
		}
		if len(f) < 3 {
			return errors.New("criterion too short")
		}
		eq := bytealg.IndexByte(f, '=')
		if eq == -1 {
			return errors.New("criterion lacks equal sign")
		}
		lowerASCIIBytes(f)
		c = append(c, NSSCriterion{
			negate: not,
			status: string(f[:eq]),
			action: string(f[eq+1:]),
		})
		return nil
	})
	return
}
