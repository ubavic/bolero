// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ui

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	cookieName  = "bolero_session"
	idleTimeout = 12 * time.Hour
	sweepEvery  = time.Minute
)

type session struct {
	id       string
	lastSeen time.Time
	views    map[string]*view
}

type view struct {
	mu    sync.Mutex
	token string
	path  string
	query url.Values
	page  Page
}

var (
	sessionsMu sync.Mutex
	sessions   = map[string]*session{}
)

func newSecret() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func sessionFor(w http.ResponseWriter, r *http.Request) (*session, error) {
	now := time.Now()
	if c, err := r.Cookie(cookieName); err == nil {
		sessionsMu.Lock()
		s := sessions[c.Value]
		if s != nil {
			s.lastSeen = now
			sessionsMu.Unlock()
			return s, nil
		}
		sessionsMu.Unlock()
	}
	id, err := newSecret()
	if err != nil {
		return nil, err
	}
	s := &session{
		id:       id,
		lastSeen: now,
		views:    map[string]*view{},
	}
	sessionsMu.Lock()
	sessions[id] = s
	sessionsMu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	})
	return s, nil
}

func (s *session) addView(v *view) {
	sessionsMu.Lock()
	s.views[v.token] = v
	s.lastSeen = time.Now()
	sessionsMu.Unlock()
}

func (s *session) lookup(token string) *view {
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	s.lastSeen = time.Now()
	return s.views[token]
}

func sweep(olderThan time.Duration) {
	cutoff := time.Now().Add(-olderThan)
	sessionsMu.Lock()
	defer sessionsMu.Unlock()
	for id, s := range sessions {
		if s.lastSeen.Before(cutoff) {
			delete(sessions, id)
		}
	}
}

func startSweep() {
	go func() {
		t := time.NewTicker(sweepEvery)
		defer t.Stop()
		for range t.C {
			sweep(idleTimeout)
		}
	}()
}
