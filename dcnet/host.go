package dcnet

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"strconv"
	"time"
)

const (
	masterHost = "dcnet.flyca.st"
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
	ErrInvalidIP      = errors.New("invalid ip")
	ErrNoHostsReplied = errors.New("no hosts replied")
)

type Host struct {
	Address netip.Addr
	Name    string
}

func GetHosts() ([]Host, error) {
	conn, err := net.Dial("udp", masterHost+":"+strconv.Itoa(queryPort))
	if err != nil {
		return nil, err
	}

	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(time.Second * 3))

	req := make([]byte, 4+1)
	binary.LittleEndian.PutUint32(req, queryMagic)
	req[4] = queryDiscover

	_, err = conn.Write(req)
	if err != nil {
		return nil, err
	}

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
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			return nil, ErrInvalidIP
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
			Address: addr,
			Name:    string(name),
		})
	}

	return hosts, nil
}

func GetBestHost() (Host, error) {
	hosts, err := GetHosts()
	if err != nil {
		return Host{}, err
	}

	req := make([]byte, 4+1+8)
	binary.LittleEndian.PutUint32(req, queryMagic)
	req[4] = queryPing
	binary.LittleEndian.PutUint64(req[5:], uint64(time.Now().UnixMilli()))

	result := make(chan Host, len(hosts))

	for _, host := range hosts {
		go func() {
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

			resp := make([]byte, 4+1+8) // doesn't really matter
			_, err = conn.Read(resp)
			if err != nil {
				return
			}

			result <- host
		}()
	}

	select {
	case best := <-result:
		return best, nil
	case <-time.NewTimer(time.Second * 3).C:
	}

	return Host{}, ErrNoHostsReplied
}
