/*
 *
 * k6 - a next-generation load testing tool
 * Copyright (C) 2020 Load Impact
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as
 * published by the Free Software Foundation, either version 3 of the
 * License, or (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */

// Output a JSON document from the current k6 version and the versioninfo.json
// template. The output is used to generate the resource.syso file embedded
// in Windows binaries for additional metadata in the file Properties, adding an
// icon, etc.
// See https://github.com/josephspurrier/goversioninfo

// Usage:
//   go run ./packaging/gen-versioninfo.go > packaging/versioninfo.json

package main

import (
	"os"
	"strings"
	"text/template"

	"github.com/loadimpact/k6/lib/consts"
)

type version struct {
	Major, Minor, Patch string
}

func main() {
	ver := strings.SplitN(consts.Version, ".", 3)
	v := version{ver[0], ver[1], ver[2]}
	tmpl, err := template.ParseFiles("./packaging/versioninfo.json.template")
	if err != nil {
		panic(err)
	}
	err = tmpl.Execute(os.Stdout, v)
	if err != nil {
		panic(err)
	}
}
