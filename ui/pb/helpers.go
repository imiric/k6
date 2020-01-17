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
	"math"
	"strconv"
	"time"
)

// GetFixedLengthIntFormat returns "%0__d" format argument for fmt functions
// that will produce a base-10 right-aligned zero-padded string representation
// of the supplied integer value. The number of characters (i.e. the actual
// number + how many zeros it will be padded on the left with) in the returned
// string corresponds to the number of digits in the supplied maxValue.
func GetFixedLengthIntFormat(maxValue int64) (formatStr string) {
	resLen := 1
	if maxValue < 0 {
		resLen++
	}
	for maxValue /= 10; maxValue != 0; maxValue /= 10 {
		resLen++
	}
	return "%0" + strconv.Itoa(resLen) + "d"
}

// GetFixedLengthFloatFormat returns "%0__.__f" format argument for fmt
// functions that will produce a base-10 right-aligned zero-padded string
// representation of the supplied float value, with the specified decimal
// precision. The number of characters (i.e. the actual number + maybe dot and
// precision + how many zeros it will be padded on the left with) in the
// returned string corresponds to the number of digits in the supplied maxValue
// and the desired precision.
func GetFixedLengthFloatFormat(maxValue float64, precision uint) (formatStr string) {
	resLen := 1
	if maxValue < 0 {
		maxValue = -maxValue
		resLen++
	}
	if maxValue >= 10 {
		resLen += int(math.Log10(maxValue))
	}
	if precision > 0 {
		resLen += int(precision + 1)
	}
	return "%0" + strconv.Itoa(resLen) + "." + strconv.Itoa(int(precision)) + "f"
}

// GetFixedLengthDuration takes a *positive* duration and its max value and
// returns a string with a fixed width so we can prevent UI elements jumping
// around. The format is "___d__h__m__s.s", but leading values can be omitted
// based on the maxDuration value, the results can be: "___h__m__s.s".
//
// This is code was inspired by the Go stdlib's time.Duration.String() code.
// TODO: more flexibility - negative values or variable precision?
func GetFixedLengthDuration(d, maxDuration time.Duration) (result string) {
	const rounding = 100 * time.Millisecond
	if d < 0 {
		d = -d
	}
	if maxDuration < 0 {
		maxDuration = -maxDuration
	}
	if maxDuration < d {
		maxDuration = d
	}
	maxDuration = maxDuration.Round(rounding)

	// Largest time is "106751d23h47m16.9s", i.e. time.Duration(math.MaxInt64)
	// Positions:    0    1    2    3    4    5    6    7    8    9    10   11   12   13   14   15   16   17
	buf := [18]byte{'0', '0', '0', '0', '0', '0', 'd', '0', '0', 'h', '0', '0', 'm', '0', '0', '.', '0', 's'}

	u := uint64(d.Round(rounding) / (rounding))
	u, buf[16] = u/10, byte(u%10)+'0'
	u, buf[14] = u/10, byte(u%10)+'0'
	if maxDuration < 10*time.Second {
		return string(buf[14:])
	}

	u, buf[13] = u/6, byte(u%6)+'0'
	if maxDuration < time.Minute {
		return string(buf[13:])
	}

	u, buf[11] = u/10, byte(u%10)+'0'
	if maxDuration < 10*time.Minute {
		return string(buf[11:])
	}

	u, buf[10] = u/6, byte(u%6)+'0'
	if maxDuration < time.Hour {
		return string(buf[10:])
	}

	u, h := u/24, u%24
	buf[7], buf[8] = byte(h/10)+'0', byte(h%10)+'0'
	if maxDuration < 10*time.Hour {
		return string(buf[8:])
	} else if maxDuration < 24*time.Hour {
		return string(buf[7:])
	}

	u, buf[5] = u/10, byte(u%10)+'0'
	remDayPowers := maxDuration / (240 * time.Hour)
	i := 5
	for remDayPowers > 0 {
		i--
		u, buf[i] = u/10, byte(u%10)+'0'
		remDayPowers /= 10
	}

	return string(buf[i:])
}

// Clampf returns the given value, "clamped" to the range [min, max].
// This is copied from lib/util.go to avoid circular imports.
func Clampf(val, min, max float64) float64 {
	switch {
	case val < min:
		return min
	case val > max:
		return max
	default:
		return val
	}
}

// Min returns the minimum value of a and b.
// This is copied from lib/util.go to avoid circular imports.
func Min(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
