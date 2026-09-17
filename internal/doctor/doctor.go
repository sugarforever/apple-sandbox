// Package doctor implements environment checks for running Apple `container`
// based sandboxes: the `container` CLI itself, its background services,
// hardware virtualization, and free disk space.
package doctor

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/sugarforever/apple-sandbox/internal/containercli"
)

// Status is the outcome of a single check.
type Status int

const (
	Pass Status = iota
	Warn
	Fail
)

func (s Status) String() string {
	switch s {
	case Pass:
		return "PASS"
	case Warn:
		return "WARN"
	default:
		return "FAIL"
	}
}

// Check is one diagnostic result.
type Check struct {
	Name   string
	Status Status
	Detail string
}

// minFreeGiB is the free-space threshold below which we warn: pulling/building
// a Node-based executor image comfortably needs a few GiB of headroom.
const minFreeGiB = 5

// Run executes all checks and returns them in a fixed order.
func Run() []Check {
	var checks []Check

	checks = append(checks, checkArch())
	checks = append(checks, checkHypervisor())
	checks = append(checks, checkCLI())
	checks = append(checks, checkSystemStatus())
	checks = append(checks, checkDiskSpace())

	return checks
}

func checkArch() Check {
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		return Check{"Apple Silicon", Pass, "darwin/arm64"}
	}
	return Check{"Apple Silicon", Fail, fmt.Sprintf("%s/%s is not supported; Apple `container` requires Apple Silicon macOS", runtime.GOOS, runtime.GOARCH)}
}

func checkHypervisor() Check {
	out, err := exec.Command("sysctl", "-n", "kern.hv_support").Output()
	if err != nil {
		return Check{"Hypervisor.framework", Warn, fmt.Sprintf("could not query kern.hv_support: %v", err)}
	}
	if strings.TrimSpace(string(out)) == "1" {
		return Check{"Hypervisor.framework", Pass, "supported"}
	}
	return Check{"Hypervisor.framework", Fail, "kern.hv_support=0; virtualization unavailable (nested VM or unsupported hardware)"}
}

func checkCLI() Check {
	if err := containercli.Available(); err != nil {
		return Check{"`container` CLI", Fail, "not found in PATH; install from https://github.com/apple/container"}
	}
	return Check{"`container` CLI", Pass, "found in PATH"}
}

func checkSystemStatus() Check {
	out, err := containercli.SystemStatus()
	if err != nil || !strings.Contains(out, "apiserver is running") {
		return Check{"container system services", Fail, "apiserver not running; run `container system start`"}
	}
	return Check{"container system services", Pass, "apiserver is running"}
}

func checkDiskSpace() Check {
	out, err := exec.Command("df", "-g", "/System/Volumes/Data").Output()
	if err != nil {
		return Check{"disk space", Warn, fmt.Sprintf("could not check disk space: %v", err)}
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) < 2 {
		return Check{"disk space", Warn, "unexpected `df` output"}
	}
	fields := strings.Fields(lines[1])
	if len(fields) < 4 {
		return Check{"disk space", Warn, "unexpected `df` output"}
	}
	availGiB, err := strconv.Atoi(fields[3])
	if err != nil {
		return Check{"disk space", Warn, "could not parse available space"}
	}
	if availGiB < minFreeGiB {
		return Check{"disk space", Warn, fmt.Sprintf("%dGiB free on /System/Volumes/Data; image pulls/builds may fail below ~%dGiB", availGiB, minFreeGiB)}
	}
	return Check{"disk space", Pass, fmt.Sprintf("%dGiB free on /System/Volumes/Data", availGiB)}
}
