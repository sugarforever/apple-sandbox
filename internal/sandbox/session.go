// Package sandbox contains the business logic for creating and naming
// sandbox sessions on top of internal/containercli.
package sandbox

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sugarforever/apple-sandbox/internal/containercli"
)

// NamePrefix is prepended to every container this tool creates, so `ls`
// can distinguish its sandboxes from unrelated containers without relying
// on `container list`'s lack of label filtering.
const NamePrefix = "apple-sandbox-"

var invalidNameChars = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

// NewSessionID generates a short random id, or sanitizes a caller-supplied
// one (e.g. an Agents API environment id) into a valid container name.
func NewSessionID(hint string) (string, error) {
	if hint != "" {
		sanitized := invalidNameChars.ReplaceAllString(hint, "-")
		return sanitized, nil
	}
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating session id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// ContainerName returns the `container` CLI name for a session id.
func ContainerName(sessionID string) string {
	return NamePrefix + sessionID
}

// Options configures a new sandbox session.
type Options struct {
	SessionID     string
	Image         string
	WorkspaceRoot string // parent directory under which a per-session workspace dir is created
	EnvironmentID string
	RemoteURL     string
	ExecutorKey   string
	CPUs          string
	Memory        string
	Network       string
}

// Result describes a started session.
type Result struct {
	SessionID    string
	ContainerID  string
	WorkspaceDir string
}

// Start creates the workspace directory and launches the executor container.
func Start(opts Options) (*Result, error) {
	name := ContainerName(opts.SessionID)

	workspaceDir := filepath.Join(opts.WorkspaceRoot, opts.SessionID)
	if err := os.MkdirAll(workspaceDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating workspace dir %s: %w", workspaceDir, err)
	}

	env := map[string]string{
		"CODEX_API_KEY":         opts.ExecutorKey,
		"OPENAI_ENVIRONMENT_ID": opts.EnvironmentID,
		"OPENAI_REMOTE_URL":     opts.RemoteURL,
	}
	labels := map[string]string{
		"dev.apple-sandbox.session": opts.SessionID,
	}

	id, err := containercli.Run(containercli.RunOptions{
		Name:    name,
		Image:   opts.Image,
		Workdir: workspaceDir,
		Env:     env,
		Labels:  labels,
		CPUs:    opts.CPUs,
		Memory:  opts.Memory,
		Network: opts.Network,
	})
	if err != nil {
		return nil, err
	}

	return &Result{SessionID: opts.SessionID, ContainerID: id, WorkspaceDir: workspaceDir}, nil
}

// List returns the sessions (as container names, trimmed of the prefix)
// managed by this tool.
func List() ([]containercli.Container, error) {
	all, err := containercli.List()
	if err != nil {
		return nil, err
	}
	var mine []containercli.Container
	for _, c := range all {
		if strings.HasPrefix(c.Configuration.ID, NamePrefix) {
			mine = append(mine, c)
		}
	}
	return mine, nil
}
