package cmd_test

import (
	"errors"
	"fmt"
	"syscall"
	"testing"

	"github.com/blockadence/gh-archimedes/internal/cmd"
	"github.com/blockadence/gh-archimedes/internal/driver"
)

// A stopped run reaches main wrapped — internal/driver names the driver on
// its way past, and there is room for more of that between here and there —
// so the status has to be read through the wrapping rather than off the
// error that happens to be on top.
func TestExitStatus(t *testing.T) {
	stopped := &driver.Stopped{Signal: syscall.SIGTERM, Status: 143}

	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{"nothing went wrong", nil, 0},
		{"an ordinary failure", errors.New("something broke"), 1},
		{"a run a signal stopped", stopped, 143},
		{"the same, wrapped", fmt.Errorf("driver %q: %w", "spec-kit", stopped), 143},
		{"a SIGINT", &driver.Stopped{Signal: syscall.SIGINT, Status: 130}, 130},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := cmd.ExitStatus(tc.err); got != tc.want {
				t.Errorf("ExitStatus = %d, want %d", got, tc.want)
			}
		})
	}
}
