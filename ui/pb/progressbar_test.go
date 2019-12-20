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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProgressBarRender(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		options  []ProgressBarOption
		expected string
	}{
		{[]ProgressBarOption{WithLeft(func() string { return "left" })},
			"left [--------------------------------------]"},
		{[]ProgressBarOption{WithConstLeft("constLeft")},
			"constLeft [--------------------------------------]"},
		{[]ProgressBarOption{
			WithLeft(func() string { return "left" }),
			WithProgress(func() (float64, string) { return 0, "right" }),
		},
			"left [--------------------------------------] right"},
		{[]ProgressBarOption{
			WithLeft(func() string { return "left" }),
			WithProgress(func() (float64, string) { return 0.5, "right" }),
		},
			"left [==================>-------------------] right"},
		{[]ProgressBarOption{
			WithLeft(func() string { return "left" }),
			WithProgress(func() (float64, string) { return 1.0, "right" }),
		},
			"left [======================================] right"},
		{[]ProgressBarOption{
			WithLeft(func() string { return "left" }),
			WithProgress(func() (float64, string) { return -1, "right" }),
		},
			"left [" + strings.Repeat("-", 76) + "] right"},
		{[]ProgressBarOption{
			WithLeft(func() string { return "left" }),
			WithProgress(func() (float64, string) { return 2, "right" }),
		},
			"left [" + strings.Repeat("=", 76) + "] right"},
		{[]ProgressBarOption{
			WithLeft(func() string { return "left" }),
			WithConstProgress(0.2, "constProgress"),
		},
			"left [======>-------------------------------] constProgress"},
		{[]ProgressBarOption{
			WithHijack(func() string { return "progressbar hijack!" }),
		},
			"progressbar hijack!"},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.expected, func(t *testing.T) {
			pbar := New(tc.options...)
			assert.NotNil(t, pbar)
			assert.Equal(t, tc.expected, pbar.Render(0, 0))
		})
	}
}

func TestProgressBarRenderPaddingMaxLeft(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		padding  int
		maxLen   int
		left     string
		expected string
	}{
		{-1, 0, "left", "left [--------------------------------------]"},
		{0, 0, "left", "left [--------------------------------------]"},
		{10, 0, "left", "left       [--------------------------------------]"},
		{0, 10, "left_truncated",
			"left_tr... [--------------------------------------]"},
		{20, 10, "left_truncated_padding",
			"left_tr...           [--------------------------------------]"},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.left, func(t *testing.T) {
			pbar := New(WithLeft(func() string { return tc.left }))
			assert.NotNil(t, pbar)
			assert.Equal(t, tc.expected, pbar.Render(tc.padding, tc.maxLen))
		})
	}
}

func TestProgressBarLeft(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		left     func() string
		expected string
	}{
		{nil, ""},
		{func() string { return " left " }, " left "},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.expected, func(t *testing.T) {
			pbar := New(WithLeft(tc.left))
			assert.NotNil(t, pbar)
			assert.Equal(t, tc.expected, pbar.Left())
		})
	}
}
