// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ui

import (
	"bufio"
	"encoding/binary"
	"errors"
	"io"
)

const maxMessage = 1 << 20

const (
	opCont  = 0x0
	opText  = 0x1
	opClose = 0x8
	opPing  = 0x9
	opPong  = 0xA
)

func readMessage(rw *bufio.ReadWriter) ([]byte, error) {
	var buf []byte
	started := false
	for {
		fin, op, payload, err := readFrame(rw)
		if err != nil {
			return nil, err
		}
		switch op {
		case opPing:
			if err := writeFrame(rw, opPong, payload); err != nil {
				return nil, err
			}
			if err := rw.Flush(); err != nil {
				return nil, err
			}
		case opPong:
		case opClose:
			_ = writeFrame(rw, opClose, nil)
			_ = rw.Flush()
			return nil, io.EOF
		case opText:
			if started {
				return nil, errors.New("unexpected text frame")
			}
			started = true
			if len(payload) > maxMessage {
				return nil, errors.New("message too large")
			}
			buf = append(buf, payload...)
			if fin {
				return buf, nil
			}
		case opCont:
			if !started {
				return nil, errors.New("unexpected continuation")
			}
			if len(buf)+len(payload) > maxMessage {
				return nil, errors.New("message too large")
			}
			buf = append(buf, payload...)
			if fin {
				return buf, nil
			}
		default:
			return nil, errors.New("unsupported frame")
		}
	}
}

func readFrame(r io.Reader) (fin bool, op byte, payload []byte, err error) {
	var hdr [2]byte
	if _, err = io.ReadFull(r, hdr[:]); err != nil {
		return false, 0, nil, err
	}
	if hdr[0]&0x70 != 0 {
		return false, 0, nil, errors.New("reserved bits set")
	}
	fin = hdr[0]&0x80 != 0
	op = hdr[0] & 0x0f
	if !masked(hdr[1]) {
		return false, 0, nil, errors.New("client frame is not masked")
	}
	n := uint64(hdr[1] & 0x7f)
	switch n {
	case 126:
		var ext [2]byte
		if _, err = io.ReadFull(r, ext[:]); err != nil {
			return false, 0, nil, err
		}
		n = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err = io.ReadFull(r, ext[:]); err != nil {
			return false, 0, nil, err
		}
		n = binary.BigEndian.Uint64(ext[:])
		if n>>63 != 0 {
			return false, 0, nil, errors.New("frame length has the high bit set")
		}
	}
	if n > maxMessage {
		return false, 0, nil, errors.New("frame too large")
	}
	if op >= opClose && (!fin || n > 125) {
		return false, 0, nil, errors.New("bad control frame")
	}
	var mask [4]byte
	if _, err = io.ReadFull(r, mask[:]); err != nil {
		return false, 0, nil, err
	}
	payload = make([]byte, n)
	if _, err = io.ReadFull(r, payload); err != nil {
		return false, 0, nil, err
	}
	for i := range payload {
		payload[i] ^= mask[i%4]
	}
	return fin, op, payload, nil
}

func masked(b byte) bool { return b&0x80 != 0 }

func writeFrame(w io.Writer, op byte, payload []byte) error {
	var hdr []byte
	b0 := byte(0x80) | op
	switch {
	case len(payload) < 126:
		hdr = []byte{b0, byte(len(payload))}
	case len(payload) <= 65535:
		hdr = []byte{b0, 126, byte(len(payload) >> 8), byte(len(payload))}
	default:
		hdr = make([]byte, 10)
		hdr[0] = b0
		hdr[1] = 127
		binary.BigEndian.PutUint64(hdr[2:], uint64(len(payload)))
	}
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// writeClientFrame is the masked frame a test client sends.
func writeClientFrame(w io.Writer, op byte, payload []byte, mask [4]byte) error {
	var hdr []byte
	b0 := byte(0x80) | op
	switch {
	case len(payload) < 126:
		hdr = []byte{b0, 0x80 | byte(len(payload))}
	case len(payload) <= 65535:
		hdr = []byte{b0, 0x80 | 126, byte(len(payload) >> 8), byte(len(payload))}
	default:
		hdr = make([]byte, 10)
		hdr[0] = b0
		hdr[1] = 0x80 | 127
		binary.BigEndian.PutUint64(hdr[2:], uint64(len(payload)))
	}
	if _, err := w.Write(hdr); err != nil {
		return err
	}
	if _, err := w.Write(mask[:]); err != nil {
		return err
	}
	masked := make([]byte, len(payload))
	for i := range payload {
		masked[i] = payload[i] ^ mask[i%4]
	}
	_, err := w.Write(masked)
	return err
}
