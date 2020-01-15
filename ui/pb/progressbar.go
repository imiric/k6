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

package pb

import (
	"fmt"
	"strings"
	"sync"

	"github.com/fatih/color"
	"github.com/sirupsen/logrus"
)

const defaultWidth = 40
const defaultBarColor = color.Faint

// ProgressBar is just a simple thread-safe progressbar implementation with
// callbacks.
type ProgressBar struct {
	mutex  sync.RWMutex
	width  int
	color  *color.Color
	logger *logrus.Entry

	left     func() string
	progress func() (progress float64, right string)
	hijack   func() string
}

// ProgressBarOption is used for helper functions that modify the progressbar
// parameters, either in the constructor or via the Modify() method.
type ProgressBarOption func(*ProgressBar)

// WithLeft modifies the function that returns the left progressbar padding.
func WithLeft(left func() string) ProgressBarOption {
	return func(pb *ProgressBar) { pb.left = left }
}

// WithConstLeft sets the left progressbar padding to the supplied const.
func WithConstLeft(left string) ProgressBarOption {
	return func(pb *ProgressBar) {
		pb.left = func() string { return left }
	}
}

// WithLogger modifies the logger instance
func WithLogger(logger *logrus.Entry) ProgressBarOption {
	return func(pb *ProgressBar) { pb.logger = logger }
}

// WithProgress modifies the progress calculation function.
func WithProgress(progress func() (float64, string)) ProgressBarOption {
	return func(pb *ProgressBar) { pb.progress = progress }
}

// WithConstProgress sets the progress and right padding to the supplied consts.
func WithConstProgress(progress float64, right string) ProgressBarOption {
	return func(pb *ProgressBar) {
		pb.progress = func() (float64, string) { return progress, right }
	}
}

// WithHijack replaces the progressbar String function with the argument.
func WithHijack(hijack func() string) ProgressBarOption {
	return func(pb *ProgressBar) { pb.hijack = hijack }
}

// New creates and initializes a new ProgressBar struct, calling all of the
// supplied options
func New(options ...ProgressBarOption) *ProgressBar {
	pb := &ProgressBar{
		mutex: sync.RWMutex{},
		width: defaultWidth,
		color: color.New(defaultBarColor),
	}
	pb.Modify(options...)
	return pb
}

// Left returns the left part of the progressbar in a thread-safe way.
func (pb *ProgressBar) Left() string {
	pb.mutex.RLock()
	defer pb.mutex.RUnlock()

	return pb.renderLeft(0)
}

// renderLeft renders the left part of the progressbar, applying the
// given padding and trimming text exceeding maxLen length,
// replacing it with an ellipsis.
func (pb *ProgressBar) renderLeft(maxLen int) string {
	var left string
	if pb.left != nil {
		l := pb.left()
		if maxLen > 0 && len(l) > maxLen {
			l = l[:maxLen-3] + "..."
		}
		padFmt := fmt.Sprintf("%%-%ds", maxLen)
		left = fmt.Sprintf(padFmt, l)
	}
	return left
}

// Modify changes the progressbar options in a thread-safe way.
func (pb *ProgressBar) Modify(options ...ProgressBarOption) {
	pb.mutex.Lock()
	defer pb.mutex.Unlock()
	for _, option := range options {
		option(pb)
	}
}

// Render locks the progressbar struct for reading and calls all of its methods
// to assemble the progress bar and return it as a string.
// - isTTY writes ANSI escape sequences to clean up the terminal output if true.
// - leftMax defines the maximum character length of the left-side
//   text, as well as the padding between the text and the opening
//   square bracket. Characters exceeding this length will be replaced
//   with a single ellipsis. Passing <=0 disables this.
func (pb *ProgressBar) Render(isTTY bool, leftMax int) string {
	pb.mutex.RLock()
	defer pb.mutex.RUnlock()

	if pb.hijack != nil {
		return pb.hijack()
	}

	var (
		progress float64
		right    string
	)
	if pb.progress != nil {
		progress, right = pb.progress()
		right = " " + right
		progressClamped := Clampf(progress, 0, 1)
		if progress != progressClamped {
			progress = progressClamped
			if pb.logger != nil {
				pb.logger.Warnf("progress value %.2f exceeds valid range, clamped between 0 and 1", progress)
			}
		}
	}

	space := pb.width - 2
	filled := int(float64(space) * progress)

	filling := ""
	caret := ""
	if filled > 0 {
		if filled < space {
			filling = strings.Repeat("=", filled-1)
			caret = ">"
		} else {
			filling = strings.Repeat("=", filled)
		}
	}

	padding := ""
	if space > filled {
		padding = pb.color.Sprint(strings.Repeat("-", space-filled))
	}

	if isTTY {
		// Ensure right side is clear of garbage text that might be introduced
		// by errors or logging messages. This is needed in addition to the
		// optional clear after the right text because of the use of tabs.
		right = "\x1b[K" + right
	}

	return fmt.Sprintf("%s [%s%s%s]%s",
		pb.renderLeft(leftMax), filling, caret, padding, right)
}
