package network

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

func dialSOCKS5(ctx context.Context, settings RuntimeSettings, targetAddress string) (net.Conn, error) {
	dialer := net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}
	connection, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(settings.Host, strconv.Itoa(int(settings.Port))))
	if err != nil {
		return nil, fmt.Errorf("connect to SOCKS5 proxy: %w", err)
	}
	keep := false
	defer func() {
		if !keep {
			_ = connection.Close()
		}
	}()
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	} else {
		_ = connection.SetDeadline(time.Now().Add(20 * time.Second))
	}

	methods := []byte{0x00}
	if settings.Username != "" {
		methods = append(methods, 0x02)
	}
	greeting := append([]byte{0x05, byte(len(methods))}, methods...)
	if _, err := connection.Write(greeting); err != nil {
		return nil, fmt.Errorf("send SOCKS5 greeting: %w", err)
	}
	response := make([]byte, 2)
	if _, err := io.ReadFull(connection, response); err != nil {
		return nil, fmt.Errorf("read SOCKS5 greeting: %w", err)
	}
	if response[0] != 0x05 || response[1] == 0xff {
		return nil, errors.New("SOCKS5 proxy rejected authentication methods")
	}
	if response[1] == 0x02 {
		if settings.Username == "" {
			return nil, errors.New("SOCKS5 proxy requires credentials")
		}
		username := []byte(settings.Username)
		password := []byte(settings.Password)
		if len(username) > 255 || len(password) > 255 {
			return nil, errors.New("SOCKS5 credentials exceed protocol limits")
		}
		authRequest := make([]byte, 0, 3+len(username)+len(password))
		authRequest = append(authRequest, 0x01, byte(len(username)))
		authRequest = append(authRequest, username...)
		authRequest = append(authRequest, byte(len(password)))
		authRequest = append(authRequest, password...)
		if _, err := connection.Write(authRequest); err != nil {
			return nil, fmt.Errorf("send SOCKS5 credentials: %w", err)
		}
		if _, err := io.ReadFull(connection, response); err != nil {
			return nil, fmt.Errorf("read SOCKS5 authentication response: %w", err)
		}
		if response[0] != 0x01 || response[1] != 0x00 {
			return nil, errors.New("SOCKS5 proxy rejected credentials")
		}
	} else if response[1] != 0x00 {
		return nil, fmt.Errorf("SOCKS5 proxy selected unsupported method %d", response[1])
	}

	host, portText, err := net.SplitHostPort(targetAddress)
	if err != nil {
		return nil, fmt.Errorf("parse SOCKS5 target: %w", err)
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 {
		return nil, errors.New("SOCKS5 target port is invalid")
	}
	request := []byte{0x05, 0x01, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			request = append(request, 0x01)
			request = append(request, ip4...)
		} else {
			request = append(request, 0x04)
			request = append(request, ip.To16()...)
		}
	} else {
		if len(host) == 0 || len(host) > 255 {
			return nil, errors.New("SOCKS5 target hostname is invalid")
		}
		request = append(request, 0x03, byte(len(host)))
		request = append(request, host...)
	}
	portBytes := []byte{0, 0}
	binary.BigEndian.PutUint16(portBytes, uint16(port))
	request = append(request, portBytes...)
	if _, err := connection.Write(request); err != nil {
		return nil, fmt.Errorf("send SOCKS5 connect request: %w", err)
	}
	reader := bufio.NewReader(connection)
	header := make([]byte, 4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return nil, fmt.Errorf("read SOCKS5 connect response: %w", err)
	}
	if header[0] != 0x05 || header[1] != 0x00 {
		return nil, fmt.Errorf("SOCKS5 proxy connection failed with code %d", header[1])
	}
	if err := discardSOCKSAddress(reader, header[3]); err != nil {
		return nil, err
	}
	_ = connection.SetDeadline(time.Time{})
	keep = true
	return &readerConn{Conn: connection, reader: reader}, nil
}

func discardSOCKSAddress(reader *bufio.Reader, addressType byte) error {
	length := 0
	switch addressType {
	case 0x01:
		length = 4
	case 0x04:
		length = 16
	case 0x03:
		value, err := reader.ReadByte()
		if err != nil {
			return err
		}
		length = int(value)
	default:
		return fmt.Errorf("SOCKS5 proxy returned unknown address type %d", addressType)
	}
	_, err := io.CopyN(io.Discard, reader, int64(length+2))
	return err
}
