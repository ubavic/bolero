// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ui

import (
	"cmp"
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed static/main.css static/client.js
var staticFS embed.FS

const securityPolicy = "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"

var startSweepOnce sync.Once

// ListenAndServe serves the registered pages.
// An empty addr uses the BOLERO_ADDR environment variable when it is set,
// and otherwise 127.0.0.1:8080.
func ListenAndServe(addr string) error {
	if addr == "" {
		addr = os.Getenv("BOLERO_ADDR")
	}
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	startSweepOnce.Do(startSweep)
	srv := &http.Server{
		Addr:              addr,
		Handler:           Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	log.Printf("listening on http://%s", addr)

	// Interrupt and SIGQUIT both stop the process. The bolero command's
	// script tests cancel a background server with SIGQUIT, and the
	// runtime's default SIGQUIT handler would exit non-zero.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGQUIT)
	defer stop()
	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		os.Exit(0)
	}
	return nil
}

// Handler returns the application's HTTP handler.
// It uses the pages registered so far.
func Handler() http.Handler {
	name, routes := snapshot()
	slices.SortFunc(routes, func(a, b Route) int {
		if c := cmp.Compare(a.Order, b.Order); c != 0 {
			return c
		}
		return strings.Compare(a.Path, b.Path)
	})
	return &site{name: name, routes: routes}
}

type site struct {
	name   string
	routes []Route
}

func (s *site) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	securityHeaders(w)
	if r.URL.Path != "/" && strings.HasSuffix(r.URL.Path, "/") {
		target := strings.TrimRight(r.URL.Path, "/")
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusMovedPermanently)
		return
	}
	switch r.URL.Path {
	case "/bolero/static/main.css":
		serveStatic(w, r, "static/main.css", "text/css; charset=utf-8")
	case "/bolero/static/client.js":
		serveStatic(w, r, "static/client.js", "text/javascript; charset=utf-8")
	case "/bolero/ws":
		serveWS(w, r)
	default:
		s.servePage(w, r)
	}
}

func securityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "same-origin")
	h.Set("Content-Security-Policy", securityPolicy)
}

func serveStatic(w http.ResponseWriter, r *http.Request, name, contentType string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	b, err := fs.ReadFile(staticFS, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(b)
}

func (s *site) servePage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	sess, err := sessionFor(w, r)
	if err != nil {
		log.Printf("session: %v", err)
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	path := r.URL.Path
	if path == "/" {
		title := s.heading()
		io.WriteString(w, s.document("", "", title, headingInner(title, "")))
		return
	}
	var route *Route
	for i := range s.routes {
		if s.routes[i].Path == path {
			route = &s.routes[i]
			break
		}
	}
	if route == nil {
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, s.document(path, "", "Not found", headingInner("Not found", notice("There is no page at this address."))))
		return
	}
	token, err := newSecret()
	if err != nil {
		log.Printf("view: %v", err)
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	v := &view{
		token: token,
		path:  path,
		query: r.URL.Query(),
	}
	page, rec := callPage(route.Page, &Context{view: v})
	if rec != nil {
		log.Printf("page %s: %v\n%s", path, rec, debug.Stack())
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, s.document(path, "", "Error", headingInner("Error", notice(fmt.Sprint(rec)))))
		return
	}
	v.page = page
	sess.addView(v)
	v.mu.Lock()
	inner := v.mainInner()
	doc := s.document(path, token, v.page.Title, inner)
	v.mu.Unlock()
	io.WriteString(w, doc)
}

func (s *site) heading() string {
	if s.name != "" {
		return s.name
	}
	return "Bolero"
}

func headingInner(title, body string) string {
	var h htmlBuilder
	h.s("<header>")
	h.text(title)
	h.s("</header>")
	h.s(body)
	return h.b.String()
}

func callPage(fn func(*Context) Page, ctx *Context) (page Page, rec any) {
	defer func() { rec = recover() }()
	page = fn(ctx)
	return page, nil
}

func (v *view) mainInner() string {
	body, err := renderPageBody(&v.page)
	if err != nil {
		return headingInner(v.page.Title, notice(err.Error()))
	}
	return headingInner(v.page.Title, body)
}

func (v *view) mainElement() string {
	return `<main data-bolero-id="` + mainID + `">` + v.mainInner() + `</main>`
}
