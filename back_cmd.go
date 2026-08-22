package main

import (
	"context"
	"io"
	"os/exec"
	"strings"
)

type CommandBackend struct {
	io.ReadWriteCloser

	command string
}

func (b CommandBackend) Run(ctx context.Context) error {
	split := strings.Split(b.command, " ")

	cmd := exec.CommandContext(ctx, split[0], split[1:]...)
	cmd.Stdin = b
	cmd.Stdout = b

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
