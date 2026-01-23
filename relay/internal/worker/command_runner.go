package worker

import (
	"context"
	"os"
	"os/exec"
)

type Command struct {
	Name string
	Args []string
	Dir  string
	Env  []string
}

type CommandRunner interface {
	Run(ctx context.Context, cmd Command) ([]byte, error)
}

type ExecCommandRunner struct{}

func (r ExecCommandRunner) Run(ctx context.Context, cmd Command) ([]byte, error) {
	command := exec.CommandContext(ctx, cmd.Name, cmd.Args...)
	if cmd.Dir != "" {
		command.Dir = cmd.Dir
	}
	if len(cmd.Env) > 0 {
		command.Env = append(os.Environ(), cmd.Env...)
	}
	return command.CombinedOutput()
}
