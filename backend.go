package main

import (
	"bytes"
	"context"
	"errors"
	"io"
)

type Backend interface {
	io.ReadWriter

	Run(context.Context) error
}

type ModemSession struct {
	context.CancelFunc
	io.ReadWriter
}

var (
	ErrNoCarrier = errors.New("no carrier")
	noCarrier    = []byte("\r\nNO CARRIER\r\n")
)

func (s *ModemSession) Read(p []byte) (int, error) {
	n, err := s.ReadWriter.Read(p)
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
