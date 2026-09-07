package source

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func gitTraceEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SKILLCTL_GIT_TRACE"))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func tracef(format string, args ...any) {
	if !gitTraceEnabled() {
		return
	}
	fmt.Fprintf(os.Stderr, "skillctl-git: "+format+"\n", args...)
}

func traceGit(args []string, d time.Duration, err error) {
	if !gitTraceEnabled() {
		return
	}
	status := "ok"
	if err != nil {
		status = "err"
	}
	tracef("git %s %s %s", status, d.Round(time.Millisecond), strings.Join(redactGitArgs(args), " "))
}

// redactGitArgs drops the inline GitHub credential helper text so a trace
// never prints the helper script. Token values are not in argv.
func shortSHA(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

func redactGitArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "-c" && i+1 < len(args) && strings.HasPrefix(args[i+1], "credential.helper=") {
			out = append(out, "-c", "credential.helper=<redacted>")
			i++
			continue
		}
		out = append(out, args[i])
	}
	return out
}
