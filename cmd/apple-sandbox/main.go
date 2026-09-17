// Command apple-sandbox is a no-daemon CLI that launches OpenAI Agents API
// self-hosted executors (`codex exec-server`) inside Apple `container` VMs.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/sugarforever/apple-sandbox/internal/containercli"
	"github.com/sugarforever/apple-sandbox/internal/doctor"
	"github.com/sugarforever/apple-sandbox/internal/sandbox"
)

const defaultImage = "apple-sandbox-executor:latest"

// Set via -ldflags at release build time (see .goreleaser.yml).
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "doctor":
		err = runDoctor()
	case "build":
		err = runBuild(os.Args[2:])
	case "run":
		err = runRun(os.Args[2:])
	case "logs":
		err = runLogs(os.Args[2:])
	case "ls", "list":
		err = runList()
	case "stop":
		err = runStop(os.Args[2:])
	case "rm":
		err = runRm(os.Args[2:])
	case "prune":
		err = runPrune(os.Args[2:])
	case "version", "-v", "--version":
		fmt.Printf("apple-sandbox %s (commit %s, built %s)\n", version, commit, date)
		return
	case "-h", "--help", "help":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		usage()
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `apple-sandbox: run OpenAI Agents API self-hosted executors on Apple container

Usage:
  apple-sandbox doctor                     Check environment readiness
  apple-sandbox build [--tag <tag>]        Build the executor image
  apple-sandbox run [flags]                Start a sandbox session
  apple-sandbox logs [-f] <session-id>     Show/follow logs for a session
  apple-sandbox ls                         List sandbox sessions
  apple-sandbox stop <session-id>          Stop a session
  apple-sandbox rm <session-id>            Force-remove a session
  apple-sandbox prune [flags]              Remove stopped sessions older than a TTL
  apple-sandbox version                    Print version info
`)
}

func runDoctor() error {
	checks := doctor.Run()
	failed := false
	for _, c := range checks {
		fmt.Printf("[%s] %-24s %s\n", c.Status, c.Name, c.Detail)
		if c.Status == doctor.Fail {
			failed = true
		}
	}
	if failed {
		return fmt.Errorf("one or more checks failed")
	}
	return nil
}

func runBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	tag := fs.String("tag", defaultImage, "image tag to build")
	dir := fs.String("context", "image/executor", "build context directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "building %s from %s ...\n", *tag, *dir)
	return containercli.Build(*tag, "", *dir)
}

func runRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	sessionHint := fs.String("session-id", "", "session id (default: random); sanitized into a container name")
	image := fs.String("image", defaultImage, "executor image")
	workspaceRoot := fs.String("workspace-root", "./sandboxes", "parent directory for per-session workspace dirs")
	environmentID := fs.String("environment-id", "", "Agents API session.environment.id (required)")
	remoteURL := fs.String("remote-url", "", "Agents API session.environment.remote_url (required)")
	executorKey := fs.String("executor-key", "", "restricted, environment-scoped executor key; defaults to $"+sandbox.ExecutorKeyEnvVar)
	cpus := fs.String("cpus", "2", "CPUs allocated to the sandbox VM")
	memory := fs.String("memory", "2G", "memory allocated to the sandbox VM")
	network := fs.String("network", "", "container network name (optional)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *executorKey == "" {
		*executorKey = os.Getenv(sandbox.ExecutorKeyEnvVar)
	}
	if *environmentID == "" || *remoteURL == "" || *executorKey == "" {
		return fmt.Errorf("--environment-id and --remote-url are required, and --executor-key or $%s must be set", sandbox.ExecutorKeyEnvVar)
	}

	sessionID, err := sandbox.NewSessionID(*sessionHint)
	if err != nil {
		return err
	}

	res, err := sandbox.Start(sandbox.Options{
		SessionID:     sessionID,
		Image:         *image,
		WorkspaceRoot: *workspaceRoot,
		EnvironmentID: *environmentID,
		RemoteURL:     *remoteURL,
		ExecutorKey:   *executorKey,
		CPUs:          *cpus,
		Memory:        *memory,
		Network:       *network,
	})
	if err != nil {
		return err
	}

	fmt.Printf("session:    %s\n", res.SessionID)
	fmt.Printf("container:  %s\n", res.ContainerID)
	fmt.Printf("workspace:  %s\n", res.WorkspaceDir)
	fmt.Printf("logs:       apple-sandbox logs -f %s\n", res.SessionID)
	return nil
}

func runLogs(args []string) error {
	fs := flag.NewFlagSet("logs", flag.ExitOnError)
	follow := fs.Bool("f", false, "follow log output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: apple-sandbox logs [-f] <session-id>")
	}
	return containercli.Logs(sandbox.ContainerName(fs.Arg(0)), *follow, os.Stdout, os.Stderr)
}

func runList() error {
	sessions, err := sandbox.List()
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		fmt.Println("no sandbox sessions")
		return nil
	}
	fmt.Printf("%-40s %-10s %s\n", "SESSION", "STATUS", "IMAGE")
	for _, s := range sessions {
		id := s.Configuration.ID[len(sandbox.NamePrefix):]
		fmt.Printf("%-40s %-10s %s\n", id, s.Status, s.Configuration.Image.Reference)
	}
	return nil
}

func runStop(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: apple-sandbox stop <session-id>")
	}
	return containercli.Stop(sandbox.ContainerName(args[0]))
}

func runRm(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: apple-sandbox rm <session-id>")
	}
	return containercli.Delete(sandbox.ContainerName(args[0]), true)
}

func runPrune(args []string) error {
	fs := flag.NewFlagSet("prune", flag.ExitOnError)
	olderThan := fs.Duration("older-than", time.Hour, "remove stopped sessions started more than this long ago")
	dryRun := fs.Bool("dry-run", false, "show what would be removed without removing it")
	if err := fs.Parse(args); err != nil {
		return err
	}

	results, err := sandbox.Prune(sandbox.PruneOptions{OlderThan: *olderThan, DryRun: *dryRun})
	if err != nil {
		return err
	}
	if len(results) == 0 {
		fmt.Println("no stopped sandbox sessions")
		return nil
	}
	for _, r := range results {
		switch {
		case r.Removed && *dryRun:
			fmt.Printf("would remove %-20s started %s ago\n", r.SessionID, r.Age.Round(time.Second))
		case r.Removed:
			fmt.Printf("removed      %-20s started %s ago\n", r.SessionID, r.Age.Round(time.Second))
		default:
			fmt.Printf("kept         %-20s %s\n", r.SessionID, r.Kept)
		}
	}
	return nil
}
