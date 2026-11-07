# Website serving

A Bolero application serves pages from the `bolero/ui` package. A page is a Go value, not HTML. The server renders it, and edits come back over a WebSocket.

The values are ordinary Go structs. Component syntax, when it arrives, should lower to these structs.

## URLs

In an `app.json` module, a package that declares

```go
func Page(c *ui.Context) ui.Page
```

is a page. The function is a package-level function, not a method. The file may be `.go` or `.bo`. The parameter type is `*ui.Context` and the result is `ui.Page`, using whatever local name imports `bolero/ui`. A dot import is allowed.

The URL is the package directory with the module path removed. `example.com/shop/customers` is served at `/customers`. `example.com/shop/customers/notes` is served at `/customers/notes`. The package clause does not matter.

These directories are not pages: `internal`, `testdata`, `vendor`, names that start with `.` or `_`, the module root, and any package named `main`.

The program needs a `main` package that calls `ui.ListenAndServe`. The bolero command writes a registry into that package while building it. `bolero run .` and `bolero build` pick the pages up. `bolero run main.go` does not, because that build does not see the rest of the module. The source of `main` does not import the page packages.

```go
package main

import "bolero/ui"

func main() {
    ui.ListenAndServe("")
}
```

An empty address uses the `BOLERO_ADDR` environment variable when it is set, and otherwise `127.0.0.1:8080`. The address is not stored in `app.json`.

`PageTitle`, when the package declares it as a string constant, is the navigation label. Otherwise the label is the last path segment, with the first letter capitalized and hyphens turned into spaces. `PageOrder`, an integer constant, sorts the navigation. Equal orders are sorted by path.

A record id is a query parameter. `Page` reads it with `c.Query().Get("id")`, so `/customers?id=42` is the customer 42 page. A path such as `/customers/42` is not a route.

## The page value

```go
package customers

import "bolero/ui"

const PageTitle = "Customers"
const PageOrder = 10

func Page(c *ui.Context) ui.Page {
    results := &ui.Table{
        Name: "results",
        FieldGroups: []ui.FieldGroup{{
            Fields: []ui.Field{
                {Name: "name", Label: "Name"},
                {Name: "city", Label: "City"},
            },
        }},
        Rows: allCustomers(),
    }
    return ui.Page{
        Title: "Customers",
        Groups: []ui.Group{{
            Title: "Search",
            Items: []ui.Elem{
                ui.Form{
                    Name: "search",
                    FieldGroups: []ui.FieldGroup{{
                        Title: "Customer",
                        Fields: []ui.Field{
                            {Name: "name", Label: "Name", Kind: ui.Text, Required: true},
                            {Name: "city", Label: "City", Kind: ui.Text},
                        },
                    }},
                },
                ui.ActionPicker{
                    Name: "searchActions",
                    Actions: []ui.Action{{
                        Name:  "find",
                        Label: "Find",
                        Form:  "search",
                        OnClick: func(c *ui.Context) {
                            name, _ := c.Input("search", "name")
                            results.Rows = filterCustomers(name)
                        },
                    }},
                },
                results,
            },
        }},
    }
}
```

`Page` is called once for each page load. The returned value is kept for that browser tab. A reload calls `Page` again, so a draft does not survive a reload. Two tabs do not share a draft.

A handler closes over the variables of that call. A widget the handler will change, such as the table above, has to be a pointer stored in the page. `Context` reads and writes fields on the stored page:

```go
func (c *Context) Path() string
func (c *Context) Query() url.Values
func (c *Context) Input(form, field string) (string, bool)
func (c *Context) SetInput(form, field, value string)
func (c *Context) ClearInput(form, field string)
func (c *Context) FieldError(form, field string) string
func (c *Context) Valid(form string) bool
```

`Input` reports false when the form or the field does not exist. During `Page` the value is not stored yet, so `Input` reports false there too. `SetInput` clears the field error.

A group holds an ordered list of forms, tables, and action pickers. Names of those widgets are unique on the page and must not contain `/`. Field names are unique in their form or table. Action names are unique in their picker.

A missing name, a duplicate name, or a `/` in a name is reported when the page is rendered.

## Fields

The zero `Kind` is text. The other kinds are integer, decimal, selection, date, time, date-and-time, and boolean.

| Kind | Accepted text |
|---|---|
| Text | any string |
| Integer | an integer in the int64 range |
| Decimal | an optional sign, digits, and an optional fractional part. Compared with `math/big`, never with float64 |
| Selection | one of `Options[].Value` |
| Date | `YYYY-MM-DD` |
| Time | `HH:MM` |
| DateTime | `YYYY-MM-DDTHH:MM` |
| Boolean | `true` or `false` |

An empty value is accepted when the field is not required. A required boolean must be `true`. `Min` and `Max` use the same grammar as the value and apply to integer, decimal, date, time, and date-and-time fields. `Validate`, when set, runs after those checks. Its error text is shown on the field.

A change replaces the value, runs the checks, and the browser swaps that field. A read-only field ignores the change and is drawn again with the stored value. The browser sends the change when the field is left, or when a checkbox or selection changes. It does not send individual keystrokes.

In a form, field groups sit in a row and the fields of a group sit in a column. On a narrow window the row wraps. In a table, a field group with a title is a header spanning its columns. `Rows[i]` lines up with the fields from left to right. A short row is blank in the missing columns.

A header click sorts the current rows as text, toggling direction. The sort is a display order. `Rows` stays as the page set it. `OnRow` receives the index in `Rows`, which is not the index on screen while the table is sorted.

## Actions

Buttons live on an action picker. An action with `Form` set runs only when `Valid` is true for that form. Otherwise the page is drawn again with the field errors, and `OnClick` is not called. After `OnClick`, the page body is drawn again, so changes to a table or to other fields show up.

`OnClick` and `OnRow` run while the view is locked. They must return without starting a goroutine that uses the `Context`.

## Sessions

The session cookie is `bolero_session`. Its value is 32 bytes from `crypto/rand`, hex-encoded. The cookie is `HttpOnly`, `SameSite=Lax`, and `Path=/`. `Secure` is set when the request arrived on TLS. The value is not written into HTML, logs, or WebSocket messages.

Each page load creates a view with its own token, stored on `<body>`. The token is not a second secret. The cookie is the secret. A WebSocket event for a token owned by another session is ignored. Closing the socket keeps the session. A session with no requests for 12 hours is dropped. The store is in memory.

There is no login in this version, so every registered page is served to whoever can open the server. The default address is loopback.

## The WebSocket

The endpoint is `/bolero/ws`. The upgrade is rejected unless `Origin` is present and its host equals the request host. The handshake carries the cookie. The first text frame is JSON `{"event":"open","view":"..."}`. Later frames are:

```json
{"id":"search/name","event":"change","value":"Ada"}
{"id":"searchActions/find","event":"click"}
{"id":"results","event":"row","value":"1"}
{"id":"results/name","event":"sort"}
```

A frame is at most 1 MiB. The reply replaces markup:

```json
{"replace":[{"id":"search/name","html":"<div ...>...</div>"}]}
```

The client sets `outerHTML`. The script uses `location.host`, and `wss:` when the page is `https:`. Event handlers are delegated from `document`. The markup has no inline handlers.

HTML is escaped. There is no raw-HTML field. A panic in `Page` or in a handler is recovered. The stack is logged. The page shows the panic value, not the stack.

Response headers include `X-Content-Type-Options: nosniff`, `Referrer-Policy: same-origin`, and a content security policy of `default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'`.

## HTML and CSS

The document is a side navigation and a `<main>`. The navigation lists every page. `/` is that chrome with the application name and no groups. The name comes from `app.json`'s `name`, or the last element of the module path. `SetAppName` overrides it. A trailing slash redirects to the path without one. An unknown path is a 404 in the same chrome.

The stylesheet and the client script are served from `/bolero/static/main.css` and `/bolero/static/client.js`. The stylesheet is the lens2 theme: background `#fafafa`, green `#207a40` and `#1c703a`, table header `#c8dccf`, and Roboto with `system-ui` as the fallback. The custom elements are `bolero-nav`, `bolero-group`, `bolero-column`, and `bolero-tooltip`. `<main>` is offset by the navigation width. Widgets in a group stack. Field groups sit in a row.

Interrupt, `SIGTERM`, and `SIGQUIT` stop the process.

## Registering by hand

`Register` is for a program that is not an `app.json` module. A path must be a clean URL path other than `/`, and `Page` must be non-nil. Registering a path twice panics. In an `app.json` module the bolero command registers the pages, and a second `Register` of the same path panics at startup.

```go
ui.Register(ui.Route{
    Path:  "/customers",
    Title: "Customers",
    Order: 10,
    Page:  customers.Page,
})
```

`Handler` returns the HTTP handler for the pages registered so far.
