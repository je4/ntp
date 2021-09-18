package ntp

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"time"
)

// Query returns a response from the remote NTP server host. It contains
// the time at which the server transmitted the response as well as other
// useful information about the time and the remote server.
func Query(rawFunc func(data []byte) ([]byte, error)) (*Response, error) {
	return QueryWithOptions(rawFunc, QueryOptions{})
}

// QueryWithOptions performs the same function as Query but allows for the
// customization of several query options.
func QueryWithOptions(rawFunc func(data []byte) ([]byte, error), opt QueryOptions) (*Response, error) {
	m, now, err := getTime(rawFunc, opt)
	if err != nil {
		return nil, err
	}
	return parseTime(m, now), nil
}

// TimeV returns the current time using information from a remote NTP server.
// On error, it returns the local system time. The version may be 2, 3, or 4.
//
// Deprecated: TimeV is deprecated. Use QueryWithOptions instead.
func TimeV(rawFunc func(data []byte) ([]byte, error), version int) (time.Time, error) {
	m, recvTime, err := getTime(rawFunc, QueryOptions{Version: version})
	if err != nil {
		return time.Now(), err
	}

	r := parseTime(m, recvTime)
	err = r.Validate()
	if err != nil {
		return time.Now(), err
	}

	// Use the clock offset to calculate the time.
	return time.Now().Add(r.ClockOffset), nil
}

// Time returns the current time using information from a remote NTP server.
// It uses version 4 of the NTP protocol. On error, it returns the local
// system time.
func Time(rawFunc func(data []byte) ([]byte, error)) (time.Time, error) {
	return TimeV(rawFunc, defaultNtpVersion)
}

// getTime performs the NTP server query and returns the response message
// along with the local system time it was received.
func getTime(rawFunc func(data []byte) ([]byte, error), opt QueryOptions) (*msg, ntpTime, error) {

	if opt.Version == 0 {
		opt.Version = defaultNtpVersion
	}
	if opt.Version < 2 || opt.Version > 4 {
		return nil, 0, errors.New("invalid protocol version requested")
	}

	// Allocate a message to hold the response.
	recvMsg := new(msg)

	// Allocate a message to hold the query.
	xmitMsg := new(msg)
	xmitMsg.setMode(client)
	xmitMsg.setVersion(opt.Version)
	xmitMsg.setLeap(LeapNotInSync)

	// To ensure privacy and prevent spoofing, try to use a random 64-bit
	// value for the TransmitTime. If crypto/rand couldn't generate a
	// random value, fall back to using the system clock. Keep track of
	// when the messsage was actually transmitted.
	bits := make([]byte, 8)
	_, err := rand.Read(bits)
	var xmitTime time.Time
	if err == nil {
		xmitMsg.TransmitTime = ntpTime(binary.BigEndian.Uint64(bits))
		xmitTime = time.Now()
	} else {
		xmitTime = time.Now()
		xmitMsg.TransmitTime = toNtpTime(xmitTime)
	}

	var rawData bytes.Buffer
	rawWriter := bufio.NewWriter(&rawData)
	err = binary.Write(rawWriter, binary.BigEndian, xmitMsg)
	if err != nil {
		return nil, 0, err
	}
	rawWriter.Flush()

	recBuf, err := rawFunc(rawData.Bytes())
	recRead := bytes.NewReader(recBuf)
	err = binary.Read(recRead, binary.BigEndian, recvMsg)
	if err != nil {
		return nil, 0, err
	}

	// Keep track of the time the response was received.
	delta := time.Since(xmitTime)
	if delta < 0 {
		// The local system may have had its clock adjusted since it
		// sent the query. In go 1.9 and later, time.Since ensures
		// that a monotonic clock is used, so delta can never be less
		// than zero. In versions before 1.9, a monotonic clock is
		// not used, so we have to check.
		return nil, 0, errors.New("client clock ticked backwards")
	}
	recvTime := toNtpTime(xmitTime.Add(delta))

	// Check for invalid fields.
	if recvMsg.getMode() != server {
		return nil, 0, errors.New("invalid mode in response")
	}
	if recvMsg.TransmitTime == ntpTime(0) {
		return nil, 0, errors.New("invalid transmit time in response")
	}
	if recvMsg.OriginTime != xmitMsg.TransmitTime {
		return nil, 0, errors.New("server response mismatch")
	}
	if recvMsg.ReceiveTime > recvMsg.TransmitTime {
		return nil, 0, errors.New("server clock ticked backwards")
	}

	// Correct the received message's origin time using the actual
	// transmit time.
	recvMsg.OriginTime = toNtpTime(xmitTime)

	return recvMsg, recvTime, nil
}
