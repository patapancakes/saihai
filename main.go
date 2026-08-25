package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.bug.st/serial"

	_ "embed"
)

const (
	// data link escape
	DLE = '\x10'

	// end of text
	ETX = '\x03'
)

var (
	// 8KHz 8-bit unsigned PCM
	//go:embed assets/dialtone.wav
	dialtone []byte

	ErrNoValidModem = errors.New("no valid modem detected")
	ErrUnexpected   = errors.New("unexpected response")

	verbose *bool
)

func main() {
	fmt.Println("Saihai by Pancakes (pancakes@mooglepowered.com)")
	fmt.Println()

	verbose = flag.Bool("verbose", false, "verbose mode")
	port := flag.String("port", "", "serial port identifier")

	mode := flag.String("mode", "ppp", "backend mode to use (ppp, cmd)")
	ppp := flag.String("ppp", "dcnet.flyca.st:7654", "address for ppp backend")
	cmd := flag.String("cmd", "pppd nodetach notty 192.168.0.1:192.168.0.2 ms-dns 192.168.0.1", "command for command backend")

	flag.Parse()

	ports, _ := serial.GetPortsList()
	ports = slices.DeleteFunc(ports, filterPorts)
	if len(ports) < 1 {
		fmt.Println("No ports detected! (is the modem plugged in?)")
		os.Exit(1)
	}

	fmt.Println("Available ports:", strings.Join(ports, ", "))

	var err error

	// auto detect port
	if *port == "" {
		*port, err = getModemPort(ports)
		if err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
	}

	fmt.Println("Using", *port)
	fmt.Println()

	fmt.Println("Press [Control-C] at any time to reset")
	fmt.Println()

	for {
		// HACK: even with mutex, port isnt ready fast enough on windows
		// retry until port is ready
		var p serial.Port
		for range time.NewTicker(time.Millisecond * 100).C {
			p, err = serial.Open(*port, &serial.Mode{BaudRate: 115200})
			if err == nil {
				break
			}
			if !isPortErr(err, serial.PortBusy) {
				fmt.Println("Error:", err)
				os.Exit(1)
			}
		}

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)

		// would use SetReadTimeout but Read can't retroactively timeout on Windows...
		context.AfterFunc(ctx, func() { p.Close() })

		var backend Backend

		switch *mode {
		case "cmd":
			backend = CommandBackend{command: *cmd}
		default: //case "ppp":
			backend = PPPBackend{address: *ppp}
		}

		err = mainLoop(ctx, &ModemSession{cancel, p}, backend)
		if err != nil && err != ErrNoCarrier && !isPortErr(err, serial.PortClosed) {
			fmt.Println("Error:", err)
		}

		// fallback cancel
		// TODO: maybe redundant
		cancel()

		fmt.Println()
	}
}

func filterPorts(p string) bool {
	// skip if not mac
	if runtime.GOOS != "darwin" {
		return false
	}

	// delete non-cu devices
	dev, cu := strings.CutPrefix(p, "/dev/cu.")
	if !cu {
		return true
	}

	// delete internal devices
	if slices.Contains([]string{"Bluetooth-Incoming-Port", "debug-console"}, dev) {
		return true
	}

	return false
}

func getModemPort(ports []string) (string, error) {
	result := make(chan string)

	for _, portName := range ports {
		go func() {
			p, err := serial.Open(portName, &serial.Mode{BaudRate: 115200})
			if err != nil {
				return
			}

			defer p.Close()

			valid := make(chan bool)

			go func() {
				p.ResetInputBuffer()
				resp, err := writeCommand(p, "ATZE0")
				if err != nil || resp != "OK" {
					valid <- false
				}
				valid <- true
			}()

			select {
			case <-time.NewTimer(time.Second).C:
				return
			case valid := <-valid:
				if !valid {
					return
				}
			}

			result <- portName
		}()
	}

	select {
	case name := <-result:
		return name, nil
	case <-time.NewTimer(time.Second * 5).C:
	}

	return "", ErrNoValidModem
}

func mainLoop(ctx context.Context, s *ModemSession, backend Backend) error {
	// clear leftover data
	s.ResetInputBuffer()

	fmt.Println("Initializing...")

	// reset modem
	// Z: reset
	// E0: disable echo
	writeCommand(s, "ATZE0")

	// enter voice mode
	writeCommand(s, "AT+FCLASS=8")

	// voice mode off hook
	// 1: "DCE off-hook. DCE connected to the line."
	writeCommand(s, "AT+VLS=1")

	// set voice codec
	// 1: 8-bit unsigned PCM
	// 8000: 8KHz sample rate
	writeCommand(s, "AT+VSM=1,8000")

	// start voice transmitting
	// TODO: find out why this sometimes sends OK in addition to CONNECT
	resp, _ := writeCommand(s, "AT+VTX")
	switch resp {
	case "CONNECT":
		break
	case "OK":
		if readResponse(s) != "CONNECT" {
			return ErrUnexpected
		}
	default:
		return ErrUnexpected
	}

	// dialtone stop signal
	stop := make(chan bool)

	// start dialtone
	go func() {
		for range time.NewTicker(time.Millisecond * 100).C {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			default:
			}

			// dialtone without wav header
			_, err := s.Write(dialtone[44:])
			if err != nil {
				fmt.Println("Error:", err)
				os.Exit(1)
			}
		}
	}()

	fmt.Println("Waiting for calls...")

	b := make([]byte, 1)
	var number string

	// wait for digits
	for {
		n, err := s.Read(b)
		if err != nil {
			// BUG: serial.Port.Read timeout returns nil error
			return err
		}

		// skip if not numeric
		_, err = strconv.Atoi(string(b))
		if err != nil {
			continue
		}

		// if first number dialed
		if number == "" {
			// stop dialtone
			close(stop)

			// set inter digit timeout
			s.SetReadTimeout(time.Second)
		} else {
			// if timed out
			if n == 0 {
				// reset timeout
				s.SetReadTimeout(serial.NoTimeout)
				break
			}
		}

		number += string(b)
	}

	fmt.Println("Answering...")

	// stop voice transmission
	writeCommand(s, string([]rune{DLE, ETX}))

	// enter data mode
	writeCommand(s, "AT+FCLASS=0")

	// answer
	resp, _ = writeCommand(s, "ATA")
	if !strings.HasPrefix(resp, "CONNECT") {
		return ErrUnexpected
	}

	fmt.Println("Connected!")
	defer fmt.Println("Disconnected.")

	// start status watchdog
	go modemStatusWatchdog(ctx, s)

	// run backend
	return backend.Run(ctx, s)
}

func isPortErr(err error, code serial.PortErrorCode) bool {
	portErr, ok := errors.AsType[*serial.PortError](err)
	return ok && portErr.Code() == code
}

func modemStatusWatchdog(ctx context.Context, s *ModemSession) error {
	// DCD returns false for a few seconds still, sleep before activating
	time.Sleep(time.Second * 5)

	for range time.NewTicker(time.Second).C {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		status, err := s.GetModemStatusBits()
		if err != nil {
			return err
		}
		// check carrier detect
		if !status.DCD {
			s.Close()
			return nil
		}
	}

	return nil
}

func readResponse(r io.Reader) string {
	s := bufio.NewScanner(r)

	// ignore command return newline
	s.Scan()

	// read response line
	s.Scan()

	if *verbose {
		fmt.Println("->", s.Text())
	}

	return s.Text()
}

func writeCommand(rw io.ReadWriter, cmd string) (string, error) {
	if *verbose {
		fmt.Println("<-", cmd)
	}

	_, err := fmt.Fprintf(rw, "%s\r", cmd)
	if err != nil {
		return "", err
	}

	return readResponse(rw), nil
}
