// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ui

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

type wsIn struct {
	Event string `json:"event"`
	View  string `json:"view"`
	ID    string `json:"id"`
	Value string `json:"value"`
}

type wsOut struct {
	Replace []wsPart `json:"replace"`
}

type wsPart struct {
	ID   string `json:"id"`
	HTML string `json:"html"`
}

func serveWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !originOK(r) {
		http.Error(w, "bad origin", http.StatusForbidden)
		return
	}
	c, err := r.Cookie(cookieName)
	if err != nil {
		http.Error(w, "no session", http.StatusUnauthorized)
		return
	}
	sessionsMu.Lock()
	sess := sessions[c.Value]
	sessionsMu.Unlock()
	if sess == nil {
		http.Error(w, "no session", http.StatusUnauthorized)
		return
	}
	conn, rw, err := upgrade(w, r)
	if err != nil {
		return
	}
	defer conn.Close()

	payload, err := readMessage(rw)
	if err != nil {
		return
	}
	var open wsIn
	if err := json.Unmarshal(payload, &open); err != nil || open.Event != "open" || open.View == "" {
		return
	}
	v := sess.lookup(open.View)
	if v == nil {
		return
	}
	for {
		payload, err = readMessage(rw)
		if err != nil {
			return
		}
		var msg wsIn
		if err := json.Unmarshal(payload, &msg); err != nil {
			continue
		}
		v.mu.Lock()
		id, fragment, ok := v.handle(msg)
		v.mu.Unlock()
		if !ok {
			continue
		}
		out, err := json.Marshal(wsOut{Replace: []wsPart{{ID: id, HTML: fragment}}})
		if err != nil {
			log.Printf("websocket: %v", err)
			return
		}
		if err := writeFrame(rw, opText, out); err != nil {
			return
		}
		if err := rw.Flush(); err != nil {
			return
		}
	}
}

func originOK(r *http.Request) bool {
	raw := r.Header.Get("Origin")
	if raw == "" || strings.EqualFold(raw, "null") {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

func upgrade(w http.ResponseWriter, r *http.Request) (net.Conn, *bufio.ReadWriter, error) {
	if !headerHas(r.Header.Get("Connection"), "upgrade") || !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		http.Error(w, "not a websocket", http.StatusBadRequest)
		return nil, nil, io.EOF
	}
	if r.Header.Get("Sec-WebSocket-Version") != "13" {
		http.Error(w, "bad websocket version", http.StatusBadRequest)
		return nil, nil, io.EOF
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		http.Error(w, "missing websocket key", http.StatusBadRequest)
		return nil, nil, io.EOF
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "websocket unavailable", http.StatusInternalServerError)
		return nil, nil, io.EOF
	}
	conn, rw, err := hj.Hijack()
	if err != nil {
		return nil, nil, err
	}
	sum := sha1.Sum([]byte(key + wsGUID))
	accept := base64.StdEncoding.EncodeToString(sum[:])
	_, err = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " + accept + "\r\n\r\n")
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	if err := rw.Flush(); err != nil {
		conn.Close()
		return nil, nil, err
	}
	return conn, rw, nil
}

func headerHas(header, token string) bool {
	for _, part := range strings.Split(header, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}
