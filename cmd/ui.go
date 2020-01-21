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
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/ssh/terminal"

	"github.com/sirupsen/logrus"

	"github.com/loadimpact/k6/core/local"
	"github.com/loadimpact/k6/lib"
	"github.com/loadimpact/k6/ui/pb"
)

// TODO: Make configurable
const (
	maxLeftLength = 30
)

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
	rendered := bar.Render(0, 0)
	// Only output the left and middle part of the progress bar
	fprintf(stdout, "%s %s %s%s", rendered.Left, rendered.Progress(), rightText, end)
}

func renderMultipleBars(
	isTTY, goBack bool, maxLeft, widthDelta int, pbs []*pb.ProgressBar,
) (string, int) {
	lineEnd := "\n"
	if isTTY {
		//TODO: check for cross platform support
		lineEnd = "\x1b[K\n" // erase till end of line
	}

	var (
		longestLine int
		// Maximum length of each right side column except last,
		// used to calculate the padding between columns.
		maxRColumnLen = make([]int, 1)
		pbsCount      = len(pbs)
		rendered      = make([]pb.ProgressBarRender, pbsCount)
		result        = make([]string, pbsCount+2)
	)

	result[0] = lineEnd // start with an empty line

	// First pass to render all progressbars and get the maximum
	// lengths of right-side columns.
	for i, pb := range pbs {
		rend := pb.Render(maxLeft, widthDelta)
		for i := range rend.Right {
			// Skip last column, since there's nothing to align after it (yet?).
			if i == len(rend.Right)-1 {
				break
			}
			if len(rend.Right[i]) > maxRColumnLen[i] {
				maxRColumnLen[i] = len(rend.Right[i])
			}
		}
		rendered[i] = rend
	}

	// Second pass to render final output, applying padding where needed
	for i := range rendered {
		rend := rendered[i]
		if rend.Hijack != "" {
			result[i+1] = rend.Hijack + lineEnd
			continue
		}
		var leftText, rightText string
		leftPadFmt := fmt.Sprintf("%%-%ds", maxLeft)
		leftText = fmt.Sprintf(leftPadFmt, rend.Left)
		for i := range rend.Right {
			rpad := 0
			if len(maxRColumnLen) > i {
				rpad = maxRColumnLen[i]
			}
			rightPadFmt := fmt.Sprintf(" %%-%ds", rpad+1)
			rightText += fmt.Sprintf(rightPadFmt, rend.Right[i])
		}
		// Get "visible" line length, without ANSI escape sequences (color)
		status := fmt.Sprintf(" %s ", rend.Status())
		lineNoAnsi := leftText + status + rend.Progress() + rightText
		if len(lineNoAnsi) > longestLine {
			longestLine = len(lineNoAnsi)
		}
		rend.Color = true
		status = fmt.Sprintf(" %s ", rend.Status())
		result[i+1] = fmt.Sprintf(leftPadFmt+"%s%s%s%s", rend.Left, status,
			rend.Progress(), rightText, lineEnd)
	}

	if isTTY && goBack {
		// Go back to the beginning
		//TODO: check for cross platform support
		result[pbsCount+1] = fmt.Sprintf("\r\x1b[%dA", pbsCount+1)
	} else {
		result[pbsCount+1] = "\n"
	}

	return strings.Join(result, ""), longestLine
}

//TODO: show other information here?
//TODO: add a no-progress option that will disable these
//TODO: don't use global variables...
func showProgress(
	ctx context.Context, conf Config, execScheduler *local.ExecutionScheduler,
	logger *logrus.Logger,
) {
	if quiet || conf.HTTPDebug.Valid && conf.HTTPDebug.String != "" {
		return
	}

	// Listen for terminal window size changes, *nix only
	sigwinch := make(chan os.Signal, 1)
	signal.Notify(sigwinch, os.Signal(syscall.SIGWINCH))

	pbs := []*pb.ProgressBar{execScheduler.GetInitProgressBar()}
	for _, s := range execScheduler.GetExecutors() {
		pbs = append(pbs, s.GetProgress())
	}

	termWidth, _, err := terminal.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		logger.WithError(err).Warn("error getting terminal size")
		termWidth = 80 // TODO: something safer, return error?
	}

	// Get the longest left side string length, to align progress bars
	// horizontally and trim excess text.
	var leftLen int64
	for _, pb := range pbs {
		l := pb.Left()
		leftLen = lib.Max(int64(len(l)), leftLen)
	}
	// Limit to maximum left text length
	maxLeft := int(lib.Min(leftLen, maxLeftLength))

	var longestLine int

	var progressBarsLastRender []byte
	renderProgressBars := func(goBack bool, widthDelta int) {
		var barText string
		barText, longestLine = renderMultipleBars(stdoutTTY, goBack, maxLeft, widthDelta, pbs)
		wd := termWidth - longestLine
		if longestLine > termWidth {
			fmt.Printf("longestLine %d > termWidth %d, delta: %d\n", longestLine, termWidth, wd)
			// The UI would be clipped or split into several lines, so
			// re-render to fit the available space.
			barText, _ = renderMultipleBars(
				stdoutTTY, true, maxLeft, wd, pbs)
		} else {
			fmt.Printf("longestLine %d < termWidth %d, delta: %d\n", longestLine, termWidth, wd)
		}

		progressBarsLastRender = []byte(barText)
	}

	printProgressBars := func() {
		_, _ = stdout.Writer.Write(progressBarsLastRender)
	}

	//TODO: make configurable?
	updateFreq := 1 * time.Second
	if stdoutTTY {
		updateFreq = 100 * time.Millisecond
	}

	ctxDone := ctx.Done()
	ticker := time.NewTicker(updateFreq)
	var widthDelta int
	for {
		select {
		case <-ctxDone:
			// FIXME: Remove this...
			// Add a small delay to allow executors time to process
			// the done context, so that the correct status symbol is
			// outputted for each progress bar.
			time.Sleep(50 * time.Millisecond)
			renderProgressBars(false, 0)
			printProgressBars()
			return
		case <-ticker.C:
		case <-sigwinch:
			fmt.Printf("received SIGWINCH\n")
			newTermWidth, _, _ := terminal.GetSize(int(os.Stdout.Fd()))
			widthDelta = termWidth - longestLine
			termWidth = newTermWidth
			// wd := newTermWidth - termWidth
			// if math.Abs(float64(wd)) >= 5 {
			// 	widthDelta = wd
			// }
			// termWidth = newTermWidth
		}
		renderProgressBars(true, widthDelta)
		outMutex.Lock()
		printProgressBars()
		outMutex.Unlock()
	}
}
