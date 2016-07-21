package ninjarouter

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// Mux contains a map of handler treenodes and the NotFound handler func.
type Mux struct {
	root     map[string]*node
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

type node struct {
	pattern  string
	handler  *Handler
	children map[string]*node
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
	return &Mux{make(map[string]*node), nil}
}

// Listen is a shorthand way of doing http.ListenAndServe.
func (m *Mux) Listen(port string) {
	fmt.Printf("Listening: %s\n", port[1:])
	http.ListenAndServe(port, m)
}

func addnode(nd *node, n *node) {
	segments := split(trim(n.pattern, "/"), "/")
	for i, seg := range segments {
		if seg == "*" {
			nd.children["*"] = n
			break
		}

		_, ok := nd.children[seg]

		if !ok && i < len(segments)-1 {
			nd.children[seg] = &node{"empty", nil, make(map[string]*node)}
		} else if i == len(segments)-1 {
			nd.children[seg] = n
			break
		}
		nd = nd.children[seg]
	}
}

func (m *Mux) add(meth, patt string, handler http.HandlerFunc) {

	h := &Handler{
		patt,
		split(trim(patt, "/"), "/"),
		patt[len(patt)-1:] == "*",
		handler,
	}

	if _, ok := m.root[meth]; !ok {
		m.root[meth] = &node{"", nil, make(map[string]*node)}
	}

	n := node{
		patt,
		h,
		make(map[string]*node),
	}

	addnode(m.root[meth], &n)
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

func (m *Mux) notFound(w http.ResponseWriter, r *http.Request) {
	if m.NotFound != nil {
		w.WriteHeader(404)
		m.NotFound.ServeHTTP(w, r)
		return
	}
	// Default 404.
	http.NotFound(w, r)
	return
}

func HandlerFunc(h http.Handler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
	})
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

	var ok bool
	var xnode *node
	var nd *node

	vrs := make(map[string]string)

	if nd, ok = m.root[r.Method]; !ok {
		m.notFound(w, r)
		return
	}

	for i, seg := range segments {
		xnode, ok = nd.children[seg]

		if !ok {
			if xnode, ok = nd.children["*"]; ok {
				nd = xnode
				break
			}

			//check for variables
			for k, v := range nd.children {
				if string([]rune(k)[0]) == ":" {
					nd = v
					vrs[strings.TrimPrefix(k, ":")] = seg
					break
				}
			}
			if len(vrs) > 0 {
				if i < len(segments)-1 {
					continue
				} else {
					break
				}
			}
			//check for custom 404
			m.notFound(w, r)
			return
		}
		if xnode.pattern == "empty" && i == len(segments)-1 {
			if xnode, ok = xnode.children["*"]; ok {
				nd = xnode
				break
			}
		}

		nd = xnode
	}

	if len(vrs) > 0 {
		vars.Lock()
		vars.sessions[r] = vrs
		defer deleteVars(r)
		vars.Unlock()
		nd.handler.ServeHTTP(w, r)
		return
	}

	if nd == nil {
		m.notFound(w, r)
		return
	}
	nd.handler.ServeHTTP(w, r)
}
