package ninjarouter

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type connections struct {
	conns map[net.Conn]struct{}
	sync.RWMutex
}

// Mux contains a map of handler treenodes and the NotFound handler func.
type Mux struct {
	root     map[string]*node
	Timeout  time.Duration
	listener ninjaListener
	NotFound http.HandlerFunc
	Port     int
	Opened   func(*Mux)
	Closed   func(*Mux)
	Timed    bool
	Log      func(...interface{})

	idle   connections
	active connections

	sync.RWMutex
}

type ninjaListener struct {
	net.Listener
	closing bool
}

func (nl ninjaListener) Accept() (net.Conn, error) {
	if nl.closing {
		return nil, errors.New("Listener is closing")
	}
	return nl.Listener.Accept()
}
func (nl ninjaListener) Close() error {
	return nl.Listener.Close()
}
func (nl ninjaListener) Addr() net.Addr {
	return nl.Listener.Addr()
}

// Handler contains the pattern and handler func.
type Handler struct {
	patt     string
	parts    []string
	wild     bool
	handlers []http.HandlerFunc
}

type node struct {
	pattern  string
	handler  *Handler
	children map[string]*node
}

var vars = struct {
	sessions map[*http.Request]map[string]string
	sync.Mutex
}{
	sessions: map[*http.Request]map[string]string{},
}

func deleteVars(r *http.Request) {
	vars.Lock()
	defer vars.Unlock()

	delete(vars.sessions, r)
}

// Vars returns a map of variables associated with supplied request.
func Vars(r *http.Request) map[string]string {
	vars.Lock()
	defer vars.Unlock()
	if v, ok := vars.sessions[r]; ok {
		return v
	}
	return map[string]string{}
}

// Var returns named variable associated with supplied request
func Var(r *http.Request, n string) string {
	vars.Lock()
	defer vars.Unlock()

	if session, ok := vars.sessions[r]; ok {
		if v, ok := session[n]; ok {
			return v
		}
	}
	return ""
}

// New returns a new Mux instance.
func New() *Mux {
	return &Mux{
		root:    make(map[string]*node),
		Timeout: 10 * time.Second,
		Opened:  func(m *Mux) {},
		Closed:  func(m *Mux) {},
		Timed:   false,
		Log:     func(li ...interface{}) {},
		active: connections{
			conns: make(map[net.Conn]struct{}),
		},
		idle: connections{
			conns: make(map[net.Conn]struct{}),
		},
	}
}

// Close closes the server gracefully
func (m *Mux) Close() error {
	var err error

	m.listener.closing = true

	m.idle.RLock()

	for conn := range m.idle.conns {
		conn.Close()
	}

	m.idle.RUnlock()
	m.active.RLock()

	if m.Timeout > 0 && len(m.active.conns) > 0 {
		timer := time.NewTimer(m.Timeout)
		<-timer.C
		err = m.listener.Close()
	} else {
		err = m.listener.Close()
	}
	m.active.RUnlock()

	m.Closed(m)

	return err
}

// Accept connections and spawn a goroutine to serve each one.  Stop listening
// if anything is received on the service's channel.

func (m *Mux) removeIdleConnection(conn net.Conn) {
	m.idle.Lock()
	delete(m.idle.conns, conn)
	m.idle.Unlock()
}

func (m *Mux) removeActiveConnection(conn net.Conn) {
	m.active.Lock()
	delete(m.active.conns, conn)
	m.active.Unlock()
}

func (m *Mux) activeConnection(conn net.Conn) {
	m.active.Lock()
	m.active.conns[conn] = struct{}{}
	m.active.Unlock()
	m.removeIdleConnection(conn)
}

func (m *Mux) removeConnection(conn net.Conn) {
	m.removeActiveConnection(conn)
	m.removeIdleConnection(conn)
}

func (m *Mux) idleConnection(conn net.Conn) {
	m.idle.Lock()
	m.idle.conns[conn] = struct{}{}
	delete(m.idle.conns, conn)
	m.idle.Unlock()
}

// Listen starts a graceful HTTP server
func (m *Mux) Listen(a string, statefns ...func(net.Conn, http.ConnState)) error {
	l, err := net.Listen("tcp", a)
	if err != nil {
		return err
	}

	listener := ninjaListener{
		Listener: l,
	}

	m.listener = listener
	m.Port = listener.Addr().(*net.TCPAddr).Port

	//state := make(chan http.ConnState)

	srv := &http.Server{
		Handler: m,
		Addr:    a,
		ConnState: func(conn net.Conn, state http.ConnState) {
			switch state {
			case http.StateNew:
				m.activeConnection(conn)
			case http.StateActive:
				m.activeConnection(conn)
			case http.StateIdle:
				m.idleConnection(conn)
			case http.StateClosed, http.StateHijacked:
				m.removeConnection(conn)
			}
			for _, connstate := range statefns {
				connstate(conn, state)
			}
		},
	}

	m.Opened(m)

	err = srv.Serve(listener)

	return err
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

// Add adds many handler funcs to a route
func (m *Mux) Add(meth, patt string, handlers ...http.HandlerFunc) {
	m.add(meth, patt, handlers)
}

func (m *Mux) add(meth, patt string, handlers []http.HandlerFunc) {
	h := &Handler{
		patt,
		split(trim(patt, "/"), "/"),
		patt[len(patt)-1:] == "*",
		handlers,
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
func (m *Mux) GET(patt string, handlers ...http.HandlerFunc) {
	m.add("GET", patt, handlers)
	m.add("HEAD", patt, handlers)
}

// HEAD adds a new route for HEAD requests.
func (m *Mux) HEAD(patt string, handlers ...http.HandlerFunc) {
	m.add("HEAD", patt, handlers)
}

// POST adds a new route for POST requests.
func (m *Mux) POST(patt string, handlers ...http.HandlerFunc) {
	m.add("POST", patt, handlers)
}

// PUT adds a new route for PUT requests.
func (m *Mux) PUT(patt string, handlers ...http.HandlerFunc) {
	m.add("PUT", patt, handlers)
}

// DELETE adds a new route for DELETE requests.
func (m *Mux) DELETE(patt string, handlers ...http.HandlerFunc) {
	m.add("DELETE", patt, handlers)
}

// OPTIONS adds a new route for OPTIONS requests.
func (m *Mux) OPTIONS(patt string, handlers ...http.HandlerFunc) {
	m.add("OPTIONS", patt, handlers)
}

// PATCH adds a new route for PATCH requests.
func (m *Mux) PATCH(patt string, handlers ...http.HandlerFunc) {
	m.add("PATCH", patt, handlers)
}

func hh(w http.ResponseWriter, r *http.Request) {}

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

// HandlerFunc takes a stdlib Handler and returns itself
func (m *Mux) HandlerFunc(h http.Handler) http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
	})
}

// HandlerFunc takes a stdlib Handler and returns itself
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
		if xnode, ok = nd.children[seg]; !ok {
			if xnode, ok = nd.children["*"]; ok {
				nd = xnode
				break
			}

			//check for variables

			for k, v := range nd.children {
				if len(k) > 0 {
					if string([]rune(k)[0]) == ":" {
						nd = v
						vrs[strings.TrimPrefix(k, ":")] = seg
						break
					}
				}
			}
			if len(vrs) > 0 {
				if i > len(segments) {
					break
				} else {
					continue
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

	if nd == nil {
		m.notFound(w, r)
		return
	}

	ctx := context.Background()

	for _, handler := range nd.handler.handlers {
		r = r.WithContext(ctx)
		if len(vrs) > 0 {
			vars.Lock()
			vars.sessions[r] = vrs
			vars.Unlock()
			defer deleteVars(r)
		}
		if m.Timed {
			t1 := time.Now()
			handler.ServeHTTP(w, r)
			t2 := time.Now()
			m.Log(fmt.Sprintf("[%s] %q %v\n", r.Method, r.URL.String(), t2.Sub(t1)))
		} else {
			handler.ServeHTTP(w, r)
		}
	}
}
