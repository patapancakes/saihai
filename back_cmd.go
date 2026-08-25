package main

import (
	"context"
	"io"
	"os/exec"
	"strings"
)

type CommandBackend struct {
	command string
}

func (b CommandBackend) Run(ctx context.Context, s io.ReadWriteCloser) error {
	split := strings.Split(b.command, " ")

	cmd := exec.CommandContext(ctx, split[0], split[1:]...)
	cmd.Stdin = s
	cmd.Stdout = s

	err := cmd.Start()
	if err != nil {
		return err
	}

	_, err = cmd.Process.Wait()
	if err != nil {
		return err
	}

	return nil
}
