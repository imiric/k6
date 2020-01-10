/*
 *
 * k6 - a next-generation load testing tool
 * Copyright (C) 2019 Load Impact
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

package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/apoorvam/goterminal"

	"github.com/loadimpact/k6/core/local"
	"github.com/loadimpact/k6/lib"
	"github.com/loadimpact/k6/ui/pb"
)

// TODO: Make configurable
const maxLeftLength = 30

// A writer that syncs writes with a mutex and, if the output is a TTY, clears before newlines.
type consoleWriter struct {
	Writer io.Writer
	IsTTY  bool
	Mutex  *sync.Mutex

	// Used for flicker-free persistent objects like the progressbars
	PersistentText func()
}

func (w *consoleWriter) Write(p []byte) (n int, err error) {
	origLen := len(p)
	if w.IsTTY {
		// Add a TTY code to erase till the end of line with each new line
		// TODO: check how cross-platform this is...
		p = bytes.Replace(p, []byte{'\n'}, []byte{'\x1b', '[', '0', 'K', '\n'}, -1)
	}

	w.Mutex.Lock()
	n, err = w.Writer.Write(p)
	if w.PersistentText != nil {
		w.PersistentText()
	}
	w.Mutex.Unlock()

	if err != nil && n < origLen {
		return n, err
	}
	return origLen, err
}

func printBar(bar *pb.ProgressBar, rightText string) {
	end := "\n"
	if stdout.IsTTY {
		// If we're in a TTY, instead of printing the bar and going to the next
		// line, erase everything till the end of the line and return to the
		// start, so that the next print will overwrite the same line.
		//
		// TODO: check for cross platform support
		end = "\x1b[0K\r"
	}
	fprintf(stdout, "%s %s%s", bar.Render(0, 0), rightText, end)
}

func renderMultipleBars(pbs []*pb.ProgressBar) []string {
	var leftPad int
	for _, pb := range pbs {
		l := pb.Left()
		if len(l) > leftPad {
			leftPad = len(l)
		}
	}
	// Floor padding to maximum left text length
	leftPad = int(lib.Min(int64(leftPad), maxLeftLength))

	result := make([]string, len(pbs))
	for i, pb := range pbs {
		result[i] = pb.Render(leftPad, maxLeftLength)
	}

	return result
}

//TODO: show other information here?
//TODO: add a no-progress option that will disable these
//TODO: don't use global variables...
func showProgress(ctx context.Context, conf Config, execScheduler *local.ExecutionScheduler) {
	if quiet || conf.HTTPDebug.Valid && conf.HTTPDebug.String != "" {
		return
	}

	pbs := []*pb.ProgressBar{execScheduler.GetInitProgressBar()}
	for _, s := range execScheduler.GetExecutors() {
		pbs = append(pbs, s.GetProgress())
	}

	writer := goterminal.New(stdout.Writer)

	printProgressBars := func() {
		for _, line := range renderMultipleBars(pbs) {
			fmt.Fprintf(writer, "%s\n", line)
		}
		writer.Print()
	}

	//TODO: make configurable?
	updateFreq := 1 * time.Second
	//TODO: remove !noColor after we fix how we handle colors (see the related
	//description in the TODO message in cmd/root.go)
	if stdoutTTY && !noColor {
		updateFreq = 100 * time.Millisecond
		outMutex.Lock()
		stdout.PersistentText = printProgressBars
		stderr.PersistentText = printProgressBars
		outMutex.Unlock()
		defer func() {
			outMutex.Lock()
			stdout.PersistentText = nil
			stderr.PersistentText = nil
			if ctx.Err() != nil {
				// Render a last plain-text progressbar in an error
				printProgressBars()
			}
			outMutex.Unlock()
			writer.Reset()
		}()
	}

	ctxDone := ctx.Done()
	ticker := time.NewTicker(updateFreq)
	for {
		select {
		case <-ticker.C:
			printProgressBars()
			writer.Clear()
		case <-ctxDone:
			writer.Reset()
			return
		}
	}
}
