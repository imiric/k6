// +build darwin dragonfly freebsd linux netbsd openbsd

package cmd

import (
	"os"
	"os/signal"
	"syscall"
)

// GetTermSize returns the current terminal window size on Windows,
// but is a no-op on all other platforms. This is used to continuously
// "poll" terminal size before each UI redraw, instead of the more
// efficient event-based SIGWINCH method on *nix.
func GetTermSize(fd, termWidth int) (width, height int, err error) {
	return termWidth, 0, nil
}

// NotifyWindowResize listens for SIGWINCH (terminal window size changes)
// on *nix platforms, and is a no-op on Windows.
func NotifyWindowResize() <-chan os.Signal {
	sch := make(chan os.Signal, 1)
	signal.Notify(sch, os.Signal(syscall.SIGWINCH))
	return sch
}
