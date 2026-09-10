package cli

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"skillctl/internal/app"
	"skillctl/internal/bootstrap"
	"skillctl/internal/httpapi"
)

func NewUICmd(bm *bootstrap.Manager) *cobra.Command {
	var port int
	var noOpen bool
	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Start the local WebUI",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if port < 0 || port > 65535 {
				return app.Errorf(app.CodeInvalidArgument, "invalid --port %d: must be between 0 and 65535", port)
			}
			// The listener is loopback-only by construction.
			listener, err := listenLoopback(port)
			if err != nil {
				return app.Errorf(app.CodeInternal, "listening on 127.0.0.1:%d: %v", port, err)
			}
			actualPort := listener.Addr().(*net.TCPAddr).Port
			url := fmt.Sprintf("http://127.0.0.1:%d", actualPort)
			fmt.Printf("UI available at %s\n", url)
			if !noOpen {
				go openBrowser(url)
			}
			return httpapi.New(bm, actualPort).Serve(cmd.Context(), listener)
		},
	}
	cmd.Flags().IntVar(&port, "port", 10000, "port to listen on (occupied ports try the next; 0 picks a free port)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "do not open a browser automatically")
	return cmd
}

// listenLoopback binds 127.0.0.1 at port. Port 0 lets the kernel assign a
// free port. Any other port that is already in use tries the next port
// until 65535.
//
// Listen success is not enough: Darwin SO_REUSEADDR lets 127.0.0.1 bind
// while a wildcard listener already owns that port and steal its loopback
// traffic. A successful Dial is the occupancy signal that covers that case.
func listenLoopback(port int) (net.Listener, error) {
	if port == 0 {
		return net.Listen("tcp", "127.0.0.1:0")
	}
	var last error
	for p := port; p <= 65535; p++ {
		if loopbackReachable(p) {
			last = syscall.EADDRINUSE
			continue
		}
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			return ln, nil
		}
		if !errors.Is(err, syscall.EADDRINUSE) {
			return nil, err
		}
		last = err
	}
	return nil, last
}

func loopbackReachable(port int) bool {
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 200*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

// openBrowser opens url in the system browser. A failure is only reported:
// the URL has already been printed, so the user can open it manually.
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return
	}
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "could not open a browser; visit %s (%v)\n", url, err)
	}
}
