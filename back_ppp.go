package main

import (
	"context"
	"io"
	"net"
)

type PPPBackend struct {
	address string
}

func (b PPPBackend) Run(ctx context.Context, s io.ReadWriteCloser) error {
	conn, err := net.Dial("tcp", b.address)
	if err != nil {
		return err
	}

	// TODO: maybe redundant
	defer conn.Close()

	// closes conn after b closes
	go func() {
		<-ctx.Done()
		conn.Close() // force conn.Read to unblock
	}()

	// writes to the local ppp client
	go func() {
		io.Copy(s, conn)
		s.Close() // force b.Read to unblock
	}()

	// writes to the remote ppp server
	return copyPPP(conn, s)
}

// copyPPP writes each individual PPP frame from src to dst.
// io.Copy sends the first byte read off the port immediately,
// which is wasteful and effectively doubles the amount of sent packets.
// (remote to local doesn't matter that much though)
func copyPPP(dst io.Writer, src io.Reader) error {
	buf := make([]byte, 16*1024)
	frame := make([]byte, 0, 4096)

	for {
		n, err := src.Read(buf)
		for _, b := range buf[:n] {
			// write to frame buffer
			frame = append(frame, b)

			// continue if frame buffer empty
			if len(frame) <= 1 {
				continue
			}

			// continue if not beginning of frame
			if b != 0x7E {
				continue
			}

			// write frame buffer
			_, err := dst.Write(frame)
			if err != nil {
				return err
			}

			// truncate frame buffer
			frame = frame[:0]
		}
		if err != nil {
			return err
		}
	}
}
