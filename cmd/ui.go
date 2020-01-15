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
	"strings"
	"sync"
	"time"

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
	rendered := bar.Render(0)
	// Only output the left and middle part of the progress
	fprintf(stdout, "%s %s %s%s", rendered[0], rendered[2], rightText, end)
}

func renderMultipleBars(isTTY, goBack bool, leftMax int, pbs []*pb.ProgressBar) string {
	lineEnd := "\n"
	if isTTY {
		//TODO: check for cross platform support
		lineEnd = "\x1b[K\n" // erase till end of line
	}

	pbsCount := len(pbs)
	result := make([]string, pbsCount+2)
	result[0] = lineEnd // start with an empty line
	// TODO: simplify this...
	var (
		maxRightFirstColLen int
		rendered            [][]string
	)
	// First pass to render all progressbars and get the maximum length of
	// the first right-side column.
	for _, pb := range pbs {
		ren := pb.Render(leftMax)
		rendered = append(rendered, ren)
		if len(ren) > 3 && len(ren[3]) > maxRightFirstColLen {
			maxRightFirstColLen = len(ren[3])
		}
	}

	// Second pass to render final output, applying padding where needed
	for i, ren := range rendered {
		var leftText, middleText, rightText string
		leftPadFmt := fmt.Sprintf("%%-%ds", leftMax)
		leftText = fmt.Sprintf(leftPadFmt, ren[0])
		if len(ren) > 1 {
			leftText = leftText + fmt.Sprintf(" %s ", ren[1])
		}
		if len(ren) > 2 {
			middleText = ren[2]
			if len(ren) > 3 {
				rightPadFmt := fmt.Sprintf(" %%-%ds", maxRightFirstColLen+2)
				rightText = fmt.Sprintf(rightPadFmt, ren[3])
				if len(ren) == 5 {
					rightText = rightText + fmt.Sprintf("%s", ren[4])
				}
			}
		}
		result[i+1] = leftText + middleText + rightText + lineEnd
	}

	if isTTY && goBack {
		// Go back to the beginning
		//TODO: check for cross platform support
		result[pbsCount+1] = fmt.Sprintf("\r\x1b[%dA", pbsCount+1)
	} else {
		result[pbsCount+1] = "\n"
	}
	return strings.Join(result, "")
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

	// Get the longest left side string length, to align progress bars
	// horizontally and trim excess text.
	var leftLen int64
	for _, pb := range pbs {
		l := pb.Left()
		leftLen = lib.Max(int64(len(l)), leftLen)
	}

	// Limit to maximum left text length
	leftMax := int(lib.Min(leftLen, maxLeftLength))

	// For flicker-free progressbars!
	progressBarsLastRender := []byte(renderMultipleBars(stdoutTTY, true, leftMax, pbs))
	progressBarsPrint := func() {
		_, _ = stdout.Writer.Write(progressBarsLastRender)
	}

	//TODO: make configurable?
	updateFreq := 1 * time.Second
	//TODO: remove !noColor after we fix how we handle colors (see the related
	//description in the TODO message in cmd/root.go)
	if stdoutTTY && !noColor {
		updateFreq = 100 * time.Millisecond
		outMutex.Lock()
		stdout.PersistentText = progressBarsPrint
		stderr.PersistentText = progressBarsPrint
		outMutex.Unlock()
		defer func() {
			outMutex.Lock()
			stdout.PersistentText = nil
			stderr.PersistentText = nil
			if ctx.Err() != nil {
				// Render a last plain-text progressbar in an error
				progressBarsLastRender = []byte(renderMultipleBars(stdoutTTY, false, leftMax, pbs))
				progressBarsPrint()
			}
			outMutex.Unlock()
		}()
	}

	ctxDone := ctx.Done()
	ticker := time.NewTicker(updateFreq)
	for {
		select {
		case <-ticker.C:
			barText := renderMultipleBars(stdoutTTY, true, leftMax, pbs)
			outMutex.Lock()
			progressBarsLastRender = []byte(barText)
			progressBarsPrint()
			outMutex.Unlock()
		case <-ctxDone:
			return
		}
	}
}
