// Copyright 2026 The Bolero Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ui

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestFieldChecks(t *testing.T) {
	cases := []struct {
		name string
		f    Field
		want string
	}{
		{"empty text", Field{Kind: Text}, ""},
		{"required text", Field{Kind: Text, Required: true}, "required"},
		{"text ok", Field{Kind: Text, Value: "Ann", Required: true}, ""},
		{"integer", Field{Kind: Integer, Value: "12"}, ""},
		{"integer bad", Field{Kind: Integer, Value: "1.5"}, "not an integer"},
		{"integer min", Field{Kind: Integer, Value: "2", Min: "3"}, "below the minimum"},
		{"integer max", Field{Kind: Integer, Value: "9", Max: "4"}, "above the maximum"},
		{"decimal", Field{Kind: Decimal, Value: "-2.50"}, ""},
		{"decimal bad", Field{Kind: Decimal, Value: "1e2"}, "not a decimal number"},
		{"decimal min", Field{Kind: Decimal, Value: "0.5", Min: "1"}, "below the minimum"},
		{"selection", Field{Kind: Selection, Value: "a", Options: []Option{{Value: "a"}}}, ""},
		{"selection bad", Field{Kind: Selection, Value: "z", Options: []Option{{Value: "a"}}}, "not one of the choices"},
		{"date", Field{Kind: Date, Value: "2026-11-07"}, ""},
		{"date bad", Field{Kind: Date, Value: "7 Nov"}, "not a date"},
		{"time", Field{Kind: Time, Value: "09:30"}, ""},
		{"datetime", Field{Kind: DateTime, Value: "2026-11-07T09:30"}, ""},
		{"bool empty", Field{Kind: Boolean}, ""},
		{"bool required", Field{Kind: Boolean, Required: true}, "required"},
		{"bool true", Field{Kind: Boolean, Required: true, Value: "true"}, ""},
		{"bool junk", Field{Kind: Boolean, Value: "yes"}, "not a boolean"},
		{"validate", Field{Kind: Text, Value: "x", Validate: func(string) error { return errSample }}, "nope"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.f.check()
			if tc.f.Error != tc.want {
				t.Fatalf("error %q, want %q", tc.f.Error, tc.want)
			}
		})
	}
}

var errSample = sampleError("nope")

type sampleError string

func (e sampleError) Error() string { return string(e) }

func TestRenderEscapes(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)
	Register(Route{
		Path:  "/customers",
		Title: "Customers",
		Page: func(*Context) Page {
			return Page{
				Title: "A <b>",
				Groups: []Group{{
					Title: "G",
					Items: []Elem{
						Form{Name: "f", FieldGroups: []FieldGroup{{
							Fields: []Field{{
								Name:        "name",
								Label:       "<script>",
								Description: `a"b`,
								Value:       `a&b`,
							}},
						}}},
						Table{Name: "rows", Rows: [][]string{{"<cell>"}}, FieldGroups: []FieldGroup{{
							Fields: []Field{{Name: "c", Label: "C"}},
						}}},
					},
				}},
			}
		},
	})
	SetAppName("Shop")
	req := httptest.NewRequest(http.MethodGet, "/customers", nil)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, bad := range []string{"<script>", "<b>", "<cell>"} {
		if strings.Contains(body, bad) {
			t.Fatalf("body contains raw %s", bad)
		}
	}
	for _, good := range []string{"&lt;script&gt;", "&lt;b&gt;", "&lt;cell&gt;", "a&amp;b", "Shop", "Customers"} {
		if !strings.Contains(body, good) {
			t.Fatalf("body missing %s\n%s", good, body)
		}
	}
	cookie := rec.Result().Cookies()
	if len(cookie) != 1 || cookie[0].Name != cookieName {
		t.Fatalf("cookie %#v", cookie)
	}
	if !cookie[0].HttpOnly || cookie[0].SameSite != http.SameSiteLaxMode || cookie[0].Secure {
		t.Fatalf("cookie flags %+v", cookie[0])
	}
	if strings.Contains(body, cookie[0].Value) {
		t.Fatal("session id leaked into the page")
	}
}

func TestSecureCookieOnTLS(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)
	Register(Route{Path: "/customers", Title: "Customers", Page: func(*Context) Page {
		return Page{Title: "Customers"}
	}})
	srv := httptest.NewTLSServer(Handler())
	t.Cleanup(srv.Close)
	res, err := srv.Client().Get(srv.URL + "/customers")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	var cookie *http.Cookie
	for _, c := range res.Cookies() {
		if c.Name == cookieName {
			cookie = c
		}
	}
	if cookie == nil || !cookie.Secure {
		t.Fatalf("cookie %+v", cookie)
	}
}

func TestRoutesAndStatic(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)
	SetAppName("Shop")
	Register(Route{Path: "/b", Title: "B", Order: 2, Page: func(*Context) Page { return Page{Title: "B"} }})
	Register(Route{Path: "/a", Title: "A", Order: 1, Page: func(*Context) Page { return Page{Title: "A"} }})
	h := Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rec.Body.String()
	a := strings.Index(body, `href="/a"`)
	b := strings.Index(body, `href="/b"`)
	if a < 0 || b < 0 || a > b {
		t.Fatalf("nav order a=%d b=%d", a, b)
	}
	if !strings.Contains(body, "Shop") {
		t.Fatalf("missing app name\n%s", body)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/a/", nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/a" {
		t.Fatalf("redirect %d %s", rec.Code, rec.Header().Get("Location"))
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/a", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/bolero/static/main.css", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "bolero-nav") {
		t.Fatalf("css %d", rec.Code)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff")
	}
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "script-src 'self'") {
		t.Fatal("missing content security policy")
	}
}

func TestOriginRequired(t *testing.T) {
	resetForTest()
	h := Handler()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/bolero/ws", nil)
	req.Host = "127.0.0.1"
	req.Header.Set("Origin", "http://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "http://127.0.0.1/bolero/ws", nil)
	req.Host = "127.0.0.1"
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("missing origin status %d", rec.Code)
	}
}

func TestSessionIsolationAndEvents(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)

	var picked int
	Register(Route{
		Path:  "/customers",
		Title: "Customers",
		Page: func(c *Context) Page {
			id := c.Query().Get("id")
			results := &Table{
				Name: "results",
				FieldGroups: []FieldGroup{{
					Fields: []Field{{Name: "name", Label: "Name"}},
				}},
				Rows: [][]string{{"Bea"}, {"Ann"}},
				OnRow: func(c *Context, row int) {
					picked = row
				},
			}
			return Page{
				Title: "Customer " + id,
				Groups: []Group{{
					Items: []Elem{
						Form{Name: "search", FieldGroups: []FieldGroup{{
							Fields: []Field{
								{Name: "name", Label: "Name", Value: "Ann", Required: true},
								{Name: "qty", Label: "Qty", Kind: Integer, Value: "1"},
								{Name: "note", Label: "Note", Value: "keep", ReadOnly: true},
							},
						}}},
						ActionPicker{Name: "acts", Actions: []Action{{
							Name:  "find",
							Label: "Find",
							Form:  "search",
							OnClick: func(c *Context) {
								name, _ := c.Input("search", "name")
								results.Rows = [][]string{{name}}
							},
						}}},
						results,
					},
				}},
			}
		},
	})
	srv := httptest.NewServer(Handler())
	t.Cleanup(srv.Close)

	body, cookie := getPage(t, srv, "/customers?id=42", nil)
	if !strings.Contains(body, "Customer 42") {
		t.Fatalf("query not shown\n%s", body)
	}
	if !strings.Contains(body, "value=\"Ann\"") {
		t.Fatalf("initial value missing\n%s", body)
	}

	ws := dialWS(t, srv, cookie, viewToken(t, body))
	defer ws.conn.Close()

	ws.send(t, `{"id":"search/name","event":"change","value":""}`)
	html := ws.recvHTML(t, "search/name")
	if !strings.Contains(html, ">required<") {
		t.Fatalf("required not shown\n%s", html)
	}

	ws.send(t, `{"id":"search/name","event":"change","value":"Bea"}`)
	html = ws.recvHTML(t, "search/name")
	if strings.Contains(html, ">required<") || !strings.Contains(html, "value=\"Bea\"") {
		t.Fatalf("change html\n%s", html)
	}

	ws.send(t, `{"id":"search/note","event":"change","value":"changed"}`)
	html = ws.recvHTML(t, "search/note")
	if !strings.Contains(html, "value=\"keep\"") {
		t.Fatalf("readonly changed\n%s", html)
	}

	ws.send(t, `{"id":"acts/find","event":"click"}`)
	html = ws.recvHTML(t, mainID)
	if !strings.Contains(html, ">Bea<") {
		t.Fatalf("action did not update the table\n%s", html)
	}

	ws.send(t, `{"id":"results/name","event":"sort"}`)
	// The find action replaced the rows with one row, so sort still answers.
	html = ws.recvHTML(t, "results")
	if !strings.Contains(html, "▲") {
		t.Fatalf("sort marker missing\n%s", html)
	}

	// A reload of the same session starts a new view from Field.Value.
	body2, _ := getPage(t, srv, "/customers", cookie)
	if !strings.Contains(body2, "value=\"Ann\"") {
		t.Fatalf("reload kept the draft\n%s", body2)
	}
	ws2 := dialWS(t, srv, cookie, viewToken(t, body2))
	defer ws2.conn.Close()
	ws2.send(t, `{"id":"results","event":"row","value":"1"}`)
	ws2.recvHTML(t, mainID)
	if picked != 1 {
		t.Fatalf("picked %d", picked)
	}

	// The first tab still shows the edit from before the reload.
	ws.send(t, `{"id":"acts/find","event":"click"}`)
	html = ws.recvHTML(t, mainID)
	if !strings.Contains(html, ">Bea<") || strings.Contains(html, "value=\"Ann\"") {
		t.Fatalf("tabs shared state\n%s", html)
	}

	// A view token from this session is useless with another session's cookie.
	_, other := getPage(t, srv, "/customers", nil)
	foreign := dialWS(t, srv, other, viewToken(t, body2))
	defer foreign.conn.Close()
	foreign.expectClosed(t)
}

func TestBadViewToken(t *testing.T) {
	resetForTest()
	t.Cleanup(resetForTest)
	Register(Route{Path: "/customers", Title: "Customers", Page: func(*Context) Page {
		return Page{Title: "Customers"}
	}})
	srv := httptest.NewServer(Handler())
	t.Cleanup(srv.Close)
	_, cookie := getPage(t, srv, "/customers", nil)
	ws := dialWS(t, srv, cookie, "not-a-view")
	defer ws.conn.Close()
	ws.expectClosed(t)
}

func getPage(t *testing.T, srv *httptest.Server, path string, cookie *http.Cookie) (string, *http.Cookie) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %d\n%s", path, res.StatusCode, b)
	}
	for _, c := range res.Cookies() {
		if c.Name == cookieName {
			cookie = c
		}
	}
	if cookie == nil {
		t.Fatal("no session cookie")
	}
	return string(b), cookie
}

func viewToken(t *testing.T, body string) string {
	t.Helper()
	const mark = `data-bolero-view="`
	i := strings.Index(body, mark)
	if i < 0 {
		t.Fatal("no view token")
	}
	rest := body[i+len(mark):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		t.Fatal("view token not closed")
	}
	return rest[:j]
}

type wsPipe struct {
	conn net.Conn
	rw   *bufio.ReadWriter
}

func dialWS(t *testing.T, srv *httptest.Server, cookie *http.Cookie, view string) *wsPipe {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := net.Dial("tcp", u.Host)
	if err != nil {
		t.Fatal(err)
	}
	conn.SetDeadline(deadline())
	key := "dGhlIHNhbXBsZSBub25jZQ=="
	req := "GET /bolero/ws HTTP/1.1\r\n" +
		"Host: " + u.Host + "\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Key: " + key + "\r\n" +
		"Sec-WebSocket-Version: 13\r\n" +
		"Origin: " + srv.URL + "\r\n" +
		"Cookie: " + cookie.Name + "=" + cookie.Value + "\r\n\r\n"
	if _, err := io.WriteString(conn, req); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(conn)
	status, err := br.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(status, "101") {
		t.Fatalf("handshake %q", status)
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if line == "\r\n" {
			break
		}
	}
	rw := bufio.NewReadWriter(br, bufio.NewWriter(conn))
	c := &wsPipe{conn: conn, rw: rw}
	c.send(t, `{"event":"open","view":"`+view+`"}`)
	return c
}

func (c *wsPipe) send(t *testing.T, payload string) {
	t.Helper()
	var mask [4]byte
	copy(mask[:], []byte{1, 2, 3, 4})
	if err := writeClientFrame(c.rw, opText, []byte(payload), mask); err != nil {
		t.Fatal(err)
	}
	if err := c.rw.Flush(); err != nil {
		t.Fatal(err)
	}
}

func (c *wsPipe) expectClosed(t *testing.T) {
	t.Helper()
	c.conn.SetReadDeadline(deadline())
	var hdr [2]byte
	if _, err := io.ReadFull(c.rw, hdr[:]); err == nil {
		t.Fatal("expected the socket to close")
	}
}

func (c *wsPipe) recvHTML(t *testing.T, id string) string {
	t.Helper()
	payload := readServerText(t, c.rw)
	var msg wsOut
	if err := json.Unmarshal(payload, &msg); err != nil {
		t.Fatalf("json: %v\n%s", err, payload)
	}
	if len(msg.Replace) != 1 || msg.Replace[0].ID != id {
		t.Fatalf("replace %#v, want id %s", msg.Replace, id)
	}
	return msg.Replace[0].HTML
}

func readServerText(t *testing.T, r io.Reader) []byte {
	t.Helper()
	var hdr [2]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		t.Fatal(err)
	}
	if hdr[0]&0x0f != opText {
		t.Fatalf("opcode %d", hdr[0]&0x0f)
	}
	if hdr[1]&0x80 != 0 {
		t.Fatal("server frame is masked")
	}
	n := int(hdr[1] & 0x7f)
	switch n {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			t.Fatal(err)
		}
		n = int(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(r, ext[:]); err != nil {
			t.Fatal(err)
		}
		n = int(binary.BigEndian.Uint64(ext[:]))
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		t.Fatal(err)
	}
	return buf
}

func deadline() time.Time {
	return time.Now().Add(5 * time.Second)
}
