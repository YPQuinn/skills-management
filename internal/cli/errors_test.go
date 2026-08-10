package cli

import (
	"errors"
	"testing"

	"skillctl/internal/app"
)

func TestExitCode(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{app.Errorf(app.CodeInvalidArgument, "bad flag"), 2},
		{app.Errorf(app.CodeInvalidConfig, "bad config"), 2},
		{app.Errorf(app.CodeAlreadyInitialized, "no"), 1},
		{app.Errorf(app.CodeStateMissing, "no"), 1},
		{app.Errorf(app.CodeLocked, "no"), 1},
		{app.Errorf(app.CodeNotInitialized, "no"), 1},
		{app.Errorf(app.CodeInternal, "no"), 1},
		{errors.New("plain error"), 1},
	}
	for _, c := range cases {
		if got := ExitCode(c.err); got != c.want {
			t.Errorf("ExitCode(%v): got %d, want %d", c.err, got, c.want)
		}
	}
}
