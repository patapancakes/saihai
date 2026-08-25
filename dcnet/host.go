package dcnet

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"time"
)

const (
	master = "dcnet.flyca.st"

	AccessPort = 7654

	queryPort  = 7655
	queryMagic = 0xDC15C001
)

const (
	queryPing = 1 + iota
	queryPong
	queryDiscover
)

var (
	ErrInvalidMagic   = errors.New("invalid magic")
	ErrUnexpectedType = errors.New("unexpected type")
	ErrNoHostsReplied = errors.New("no hosts replied")
)

type Host struct {
	Address net.IP
	Name    string
}

func GetHosts() ([]Host, error) {
	conn, err := net.Dial("udp", master+":"+strconv.Itoa(queryPort))
	if err != nil {
		return nil, err
	}

	defer conn.Close()

	req := make([]byte, 5)
	binary.LittleEndian.PutUint32(req, queryMagic)
	req[4] = queryDiscover

	_, err = conn.Write(req)
	if err != nil {
		return nil, err
	}

	conn.SetReadDeadline(time.Now().Add(time.Second * 3))

	resp := make([]byte, 1400)
	n, err := conn.Read(resp)
	if err != nil {
		return nil, err
	}

	r := bytes.NewReader(resp[:n])

	var magic uint32
	err = binary.Read(r, binary.LittleEndian, &magic)
	if err != nil {
		return nil, err
	}
	if magic != queryMagic {
		return nil, ErrInvalidMagic
	}

	var packetType uint8
	err = binary.Read(r, binary.LittleEndian, &packetType)
	if err != nil {
		return nil, err
	}
	if packetType != queryDiscover {
		return nil, ErrUnexpectedType
	}

	var hosts []Host
	for {
		ip := make([]byte, 4)
		err = binary.Read(r, binary.LittleEndian, &ip)
		if err != nil {
			if err == io.EOF {
				break
			}

			return nil, err
		}

		var nameLen uint8
		err = binary.Read(r, binary.LittleEndian, &nameLen)
		if err != nil {
			return nil, err
		}

		name := make([]byte, nameLen)
		_, err = io.ReadAtLeast(r, name, len(name))
		if err != nil {
			return nil, err
		}

		hosts = append(hosts, Host{
			Address: net.IP(ip),
			Name:    string(name),
		})
	}

	return hosts, nil
}

func GetBestHost() (Host, error) {
	type Result struct {
		Host Host
		RTT  time.Duration
	}

	hosts, err := GetHosts()
	if err != nil {
		return Host{}, err
	}

	req := make([]byte, 4+1+8)
	binary.LittleEndian.PutUint32(req, queryMagic)
	req[4] = queryPing
	binary.LittleEndian.PutUint64(req[5:], uint64(time.Now().UnixMilli()))

	results := make(chan Result, len(hosts))

	var wg sync.WaitGroup
	for _, host := range hosts {
		wg.Go(func() {
			conn, err := net.Dial("udp", host.Address.String()+":"+strconv.Itoa(queryPort))
			if err != nil {
				return
			}

			defer conn.Close()
			conn.SetReadDeadline(time.Now().Add(time.Second * 3))

			_, err = conn.Write(req)
			if err != nil {
				return
			}

			sent := time.Now()

			resp := make([]byte, 13) // doesn't really matter
			_, err = conn.Read(resp)
			if err != nil {
				return
			}

			results <- Result{
				Host: host,
				RTT:  time.Since(sent),
			}
		})
	}

	wg.Wait()
	close(results)

	var best Result
	for result := range results {
		if best.RTT != 0 && best.RTT < result.RTT {
			continue
		}

		best = result
	}
	if best.Host.Name == "" {
		return Host{}, ErrNoHostsReplied
	}

	return best.Host, nil
}
