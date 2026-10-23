package daemon

import (
	"bufio"
	"context"
	"os"
	"os/exec"

	"github.com/pkg/errors"
)

// Start runs exe with args as a detached daemon that writes its log to
// logPath, and returns once it reports that it listens. The daemon outlives
// ctx; ctx bounds only the wait.
func Start(ctx context.Context, exe string, args []string, logPath string) error {
	// Open the daemon's log.
	log, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer log.Close()

	// Start the daemon apart from this process, its log to the file and its
	// ready line to this process.
	cmd := exec.Command(exe, args...)
	cmd.Stderr = log
	cmd.Env = append(os.Environ(), startedEnv+"=1")
	cmd.SysProcAttr = detached()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	// Read the ready line, or how the daemon exited without it.
	readyCh := make(chan error, 1)
	go func() {
		// Report the ready line.
		line, _ := bufio.NewReader(stdout).ReadString('\n')
		if line == readyLine {
			readyCh <- nil
			return
		}

		// Report the exit instead.
		err := cmd.Wait()
		if err == nil {
			err = errors.New("it exited")
		}
		readyCh <- errors.Wrapf(err, "the modlock daemon stopped before it was ready; its log is %s", logPath)
	}()

	// Wait for the ready line, or for the daemon to exit without it.
	select {
	case <-ctx.Done():
		_ = cmd.Process.Release()
		return context.Cause(ctx)
	case err := <-readyCh:
		if err != nil {
			return err
		}
		return cmd.Process.Release()
	}
}

// Ready tells the process that started this daemon that it listens, and
// stops writing to the standard output that process reads.
func Ready() {
	_, _ = os.Stdout.WriteString(readyLine)
	_ = os.Stdout.Close()
	if null, err := os.Open(os.DevNull); err == nil {
		os.Stdout = null
	}
}
