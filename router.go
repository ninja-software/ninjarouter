package ninjarouter

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
)

// Mux contains a map of handlers and the NotFound handler func.
type Mux struct {
	handlers map[string][]*Handler
	NotFound http.HandlerFunc
}

// Handler contains the pattern and handler func.
type Handler struct {
	patt  string
	parts []string
	wild  bool
	http.HandlerFunc
}

var vars = struct {
	sessions map[*http.Request]map[string]string
	sync.Mutex
}{
	sessions: map[*http.Request]map[string]string{},
}

// ErrNoSession is returned by Vars when there is no match for that request
var ErrNoSession = errors.New("Session does not exist")

// ErrNoVar is returned by Var when there is no match for that variable
var ErrNoVar = errors.New("Variable does not exist")

func deleteVars(r *http.Request) {
	vars.Lock()
	defer vars.Unlock()

	delete(vars.sessions, r)
}

// Vars returns a map of variables associated with supplied request.
func Vars(r *http.Request) (map[string]string, error) {
	vars.Lock()
	defer vars.Unlock()
	if v, ok := vars.sessions[r]; ok {
		return v, nil
	}
	return map[string]string{}, ErrNoSession
}

// Var returns named variable associated with supplied request
func Var(r *http.Request, n string) (string, error) {
	vars.Lock()
	defer vars.Unlock()

	if session, ok := vars.sessions[r]; ok {
		if v, ok := session[n]; ok {
			return v, ErrNoVar
		}
		return "", ErrNoVar
	}
	return "", ErrNoSession
}

// New returns a new Mux instance.
func New() *Mux {
	return &Mux{make(map[string][]*Handler), nil}
}

// Listen is a shorthand way of doing http.ListenAndServe.
func (m *Mux) Listen(port string) {
	fmt.Printf("Listening: %s\n", port[1:])
	http.ListenAndServe(port, m)
}

func (m *Mux) add(meth, patt string, handler http.HandlerFunc) {
	h := &Handler{
		patt,
		split(trim(patt, "/"), "/"),
		patt[len(patt)-1:] == "*",
		handler,
	}

	m.handlers[meth] = append(
		m.handlers[meth],
		h,
	)
}

// GET adds a new route for GET requests.
func (m *Mux) GET(patt string, handler http.HandlerFunc) {
	m.add("GET", patt, handler)
	m.add("HEAD", patt, handler)
}

// HEAD adds a new route for HEAD requests.
func (m *Mux) HEAD(patt string, handler http.HandlerFunc) {
	m.add("HEAD", patt, handler)
}

// POST adds a new route for POST requests.
func (m *Mux) POST(patt string, handler http.HandlerFunc) {
	m.add("POST", patt, handler)
}

// PUT adds a new route for PUT requests.
func (m *Mux) PUT(patt string, handler http.HandlerFunc) {
	m.add("PUT", patt, handler)
}

// DELETE adds a new route for DELETE requests.
func (m *Mux) DELETE(patt string, handler http.HandlerFunc) {
	m.add("DELETE", patt, handler)
}

// OPTIONS adds a new route for OPTIONS requests.
func (m *Mux) OPTIONS(patt string, handler http.HandlerFunc) {
	m.add("OPTIONS", patt, handler)
}

// PATCH adds a new route for PATCH requests.
func (m *Mux) PATCH(patt string, handler http.HandlerFunc) {
	m.add("PATCH", patt, handler)
}

func (m *Mux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	l := len(r.URL.Path)
	// Redirect trailing slash URL's.
	if l > 1 && r.URL.Path[l-1:] == "/" {
		http.Redirect(w, r, r.URL.Path[:l-1], 301)
		return
	}
	// Split the URL into segments.
	segments := split(trim(r.URL.Path, "/"), "/")
	// Map over the registered handlers for
	// the current request (if there is any).
	for _, handler := range m.handlers[r.Method] {
		// Try and match the pattern
		if handler.patt == r.URL.Path {
			handler.ServeHTTP(w, r)
			return
		}
		// Compare pattern segments to URL.
		if ok, v := handler.try(segments); ok {
			vars.Lock()
			vars.sessions[r] = v
			defer deleteVars(r)
			vars.Unlock()
			handler.ServeHTTP(w, r)
			return
		}
	}
	// Custom 404 handler?
	if m.NotFound != nil {
		w.WriteHeader(404)
		m.NotFound.ServeHTTP(w, r)
		return
	}
	// Default 404.
	http.NotFound(w, r)
}

func (h *Handler) try(usegs []string) (bool, map[string]string) {
	if len(h.parts) != len(usegs) && !h.wild {
		return false, nil
	}

	vars := map[string]string{}

	for idx, part := range h.parts {
		if part == "*" {
			continue
		}
		if part != "" && part[0] == ':' {
			vars[part[1:]] = usegs[idx]
			continue
		}
		if part != usegs[idx] {
			return false, nil
		}
	}

	return true, vars
}
