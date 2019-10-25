// Copyright 2015-2017 Brett Vickers.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package ntp provides an implementation of a Simple NTP (SNTP) client
// capable of querying the current time from a remote NTP server.  See
// RFC5905 (https://tools.ietf.org/html/rfc5905) for more details.
//
// This approach grew out of a go-nuts post by Michael Hofmann:
// https://groups.google.com/forum/?fromgroups#!topic/golang-nuts/FlcdMU5fkLQ

// modified for use of generic network connection handler by Jürgen Enge

package ntp

import (
	"golang.org/x/net/ipv4"
	"net"
	"time"
)

func MakeDefaultHandler(
	Host string,
	Protocol string,
	Port string,
	LocalAddress string,
	TTL int,
	Timeout time.Duration) func(data []byte) ([]byte, error) {
	// Set a timeout on the connection.
	if Timeout == 0 {
		Timeout = 5 * time.Second
	}
	if Protocol == "" {
		Protocol = "udp"
	}
	if Port == "" {
		Port = "123"
	}

	return func(data []byte) ([]byte, error) {
		// Resolve the remote NTP server address.
		raddr, err := net.ResolveUDPAddr(Protocol, net.JoinHostPort(Host, Port))
		if err != nil {
			return nil, err
		}

		var laddr *net.UDPAddr
		if LocalAddress != "" {
			laddr, err = net.ResolveUDPAddr(Protocol, net.JoinHostPort(LocalAddress, "0"))
			if err != nil {
				return nil, err
			}
		}
		// Prepare a "connection" to the remote server.
		con, err := net.DialUDP(Protocol, laddr, raddr)
		if err != nil {
			return nil, err
		}
		defer con.Close()

		// Set a TTL for the packet if requested.
		if TTL != 0 {
			ipcon := ipv4.NewConn(con)
			err = ipcon.SetTTL(TTL)
			if err != nil {
				return nil, err
			}
		}

		con.SetDeadline(time.Now().Add(Timeout))

		// Transmit the query.
		_, err = con.Write(data)
		//err = binary.Write(con, binary.BigEndian, data)
		if err != nil {
			return nil, err
		}

		// Receive the response.
		recvMsg := make([]byte, len(data))
		_, err = con.Read(recvMsg)
		//err = binary.Read(con, binary.BigEndian, recvMsg)
		if err != nil {
			return nil, err
		}

		return recvMsg, nil
	}

}
