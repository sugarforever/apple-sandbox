// Package containercli wraps invocations of Apple's `container` CLI.
// It shells out rather than linking Containerization directly because the
// CLI is the stable, documented surface; the Swift framework underneath is
// still evolving.
package containercli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
)

const binary = "container"

// Available reports whether the `container` CLI is on PATH.
func Available() error {
	if _, err := exec.LookPath(binary); err != nil {
		return fmt.Errorf("`%s` CLI not found in PATH: %w", binary, err)
	}
	return nil
}

// SystemStatus returns the raw output of `container system status`.
func SystemStatus() (string, error) {
	out, err := exec.Command(binary, "system", "status").CombinedOutput()
	return string(bytes.TrimSpace(out)), err
}

// RunOptions describes a sandbox container to launch.
type RunOptions struct {
	Name    string // container name, also used as the session id
	Image   string
	Workdir string // bind-mounted host directory -> /workspace
	Env     map[string]string
	Labels  map[string]string
	CPUs    string
	Memory  string
	Network string // optional network name
}

// Run starts a detached container and returns its id (== Name).
func Run(opts RunOptions) (string, error) {
	// Deliberately no --rm: a crashed executor's logs are the primary
	// debugging tool, so the container must survive until an explicit
	// `apple-sandbox rm`. Durable state (the workspace) lives on the host
	// bind mount regardless.
	args := []string{"run", "-d", "--name", opts.Name}

	if opts.CPUs != "" {
		args = append(args, "-c", opts.CPUs)
	}
	if opts.Memory != "" {
		args = append(args, "-m", opts.Memory)
	}
	if opts.Network != "" {
		args = append(args, "--network", opts.Network)
	}
	if opts.Workdir != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/workspace", opts.Workdir))
	}
	for k, v := range opts.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}
	for k, v := range opts.Labels {
		args = append(args, "-l", fmt.Sprintf("%s=%s", k, v))
	}
	args = append(args, opts.Image)

	cmd := exec.Command(binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("container run failed: %w\n%s", err, stderr.String())
	}
	return opts.Name, nil
}

// Logs streams (or dumps) logs for a container to the given writers.
func Logs(name string, follow bool, out, errOut io.Writer) error {
	args := []string{"logs"}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, name)
	cmd := exec.Command(binary, args...)
	cmd.Stdout = out
	cmd.Stderr = errOut
	return cmd.Run()
}

// Stop stops a running container.
func Stop(name string) error {
	out, err := exec.Command(binary, "stop", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("container stop failed: %w\n%s", err, bytes.TrimSpace(out))
	}
	return nil
}

// Delete removes a container (force to also remove if still running).
func Delete(name string, force bool) error {
	args := []string{"delete"}
	if force {
		args = append(args, "-f")
	}
	args = append(args, name)
	out, err := exec.Command(binary, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("container delete failed: %w\n%s", err, bytes.TrimSpace(out))
	}
	return nil
}

// Container is the subset of `container list --format json` fields we use.
type Container struct {
	Status        string `json:"status"`
	Configuration struct {
		ID    string `json:"id"`
		Image struct {
			Reference string `json:"reference"`
		} `json:"image"`
	} `json:"configuration"`
}

// List returns all containers (running and stopped).
func List() ([]Container, error) {
	out, err := exec.Command(binary, "list", "--all", "--format", "json").Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("container list failed: %w\n%s", err, ee.Stderr)
		}
		return nil, fmt.Errorf("container list failed: %w", err)
	}
	var containers []Container
	if err := json.Unmarshal(out, &containers); err != nil {
		return nil, fmt.Errorf("parsing `container list` output: %w", err)
	}
	return containers, nil
}

// Build runs `container build -t tag -f dockerfile contextDir`, streaming
// progress to stderr.
func Build(tag, dockerfile, contextDir string) error {
	args := []string{"build", "-t", tag}
	if dockerfile != "" {
		args = append(args, "-f", dockerfile)
	}
	args = append(args, contextDir)
	cmd := exec.Command(binary, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
