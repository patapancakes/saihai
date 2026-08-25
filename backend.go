package main

import (
	"bytes"
	"context"
	"errors"
	"io"

	"go.bug.st/serial"
)

type Backend interface {
	Run(context.Context, io.ReadWriteCloser) error
}

type ModemSession struct {
	context.CancelFunc
	serial.Port
}

var (
	ErrNoCarrier = errors.New("no carrier")
	noCarrier    = []byte("\r\nNO CARRIER\r\n")
)

func (s *ModemSession) Read(p []byte) (int, error) {
	n, err := s.Port.Read(p)
	if bytes.HasSuffix(p[:n], noCarrier) {
		s.Close()
		return n, ErrNoCarrier
	}
	if err != nil {
		return n, err
	}

	return n, nil
}

func (s *ModemSession) Close() error {
	s.CancelFunc()
	return nil
}
