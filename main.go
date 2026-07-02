package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const pollInterval = 500 * time.Millisecond

// stateDir returns the plugin state directory, creating it. Resolution order:
//  1. $HERDR_PLUGIN_STATE_DIR (if absolute)
//  2. XDG_STATE_HOME/herdr/plugins/herdr-tab-rename (if XDG_STATE_HOME is absolute)
//  3. ~/.local/state/herdr/plugins/herdr-tab-rename
//
// os.TempDir is only a last-resort fallback when HOME is unavailable (avoiding
// the prior per-shell TMPDIR bug that split state across launch contexts).
// sessionKey derives a stable, filesystem-safe key for the session from its
// socket path. Named sessions live under .../sessions/<name>/herdr.sock; the
// default session (or any non-standard path) maps to "default".
func sessionKey(sock string) string {
	if i := strings.Index(sock, "/sessions/"); i >= 0 {
		rest := sock[i+len("/sessions/"):]
		if j := strings.LastIndex(rest, "/"); j >= 0 {
			if name := rest[:j]; name != "" {
				return name
			}
		}
	}
	return "default"
}

func stateDir() string {
	session := sessionKey(socketPath())
	if d := os.Getenv("HERDR_PLUGIN_STATE_DIR"); d != "" && filepath.IsAbs(d) {
		return mkdir(filepath.Join(d, session))
	}
	if x := os.Getenv("XDG_STATE_HOME"); x != "" && filepath.IsAbs(x) {
		return mkdir(filepath.Join(x, "herdr", "plugins", "herdr-tab-rename", session))
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return mkdir(filepath.Join(os.TempDir(), "herdr-tab-rename", session))
	}
	return mkdir(filepath.Join(home, ".local", "state", "herdr", "plugins", "herdr-tab-rename", session))
}

func mkdir(p string) string {
	_ = os.MkdirAll(p, 0o755)
	return p
}

func pidFile() string { return filepath.Join(stateDir(), "herdr-tab-rename.pid") }
func logFile() string { return filepath.Join(stateDir(), "daemon.log") }
func locksPath() string { return filepath.Join(stateDir(), "locks.json") }

func socketPath() string {
	if s := os.Getenv("HERDR_SOCKET_PATH"); s != "" {
		return s
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "herdr.sock")
	}
	return filepath.Join(home, ".config", "herdr", "herdr.sock")
}

func readPid(path string) (int, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(string(b))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

func openLog() *os.File {
	f, err := os.OpenFile(logFile(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return os.Stderr
	}
	return f
}

func main() {
	cmd := "start"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	os.Exit(run(cmd))
}

func run(cmd string) int {
	switch cmd {
	case "daemon":
		return cmdDaemon()
	case "stop":
		return cmdStop()
	case "start", "":
		return cmdStart()
	default:
		fmt.Fprintln(os.Stderr, "usage: herdr-tab-rename [start|stop|daemon]")
		return 2
	}
}

// cmdStart is the idempotent launcher. If a live process owns the pid file it
// exits 0 immediately; otherwise it re-executes the daemon as a detached
// background process. Hook commands block until exit, so start returns quickly
// after detaching.
func cmdStart() int {
	if pid, ok := readPid(pidFile()); ok && processAlive(pid) {
		return 0
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not resolve executable:", err)
		return 1
	}

	lf := openLog()
	defer lf.Close()

	c := exec.Command(exe, "daemon")
	c.Stdin = nil
	c.Stdout = lf
	c.Stderr = lf
	c.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := c.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "start failed:", err)
		return 1
	}
	_ = os.WriteFile(pidFile(), []byte(strconv.Itoa(c.Process.Pid)), 0o644)
	_ = c.Process.Release() // detach; child re-parents to init via Setsid
	return 0
}

func cmdStop() int {
	pid, ok := readPid(pidFile())
	if !ok {
		_ = os.Remove(pidFile())
		return 0
	}
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Signal(syscall.SIGTERM)
	}
	_ = os.Remove(pidFile())
	return 0
}

// cmdDaemon is the long-lived poll loop.
func cmdDaemon() int {
	lf := openLog()
	defer lf.Close()
	logger := log.New(lf, "", log.LstdFlags)

	// Authoritative pid: the daemon writes its own pid.
	_ = os.WriteFile(pidFile(), []byte(strconv.Itoa(os.Getpid())), 0o644)

	sock := socketPath()
	logger.Printf("herdr-tab-rename daemon starting (socket=%s)", sock)

	d := newDaemon(sock, logger)
	if err := d.seed(); err != nil {
		logger.Printf("seed warning: %v", err)
	}
	d.restoreLocks()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for range ticker.C {
		if err := d.poll(); err != nil {
			logger.Printf("poll error: %v", err)
		}
	}
	return 0
}
