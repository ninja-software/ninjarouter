package ninjarouter

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Mux contains a map of handler treenodes and the NotFound handler func.

type Mux struct {
	root     map[string]*node
	Timeout  time.Duration
	listener ninjaListener
	NotFound http.HandlerFunc
	Port     int
	Opened   func(*Mux)
	Closed   func(*Mux)

	idle   map[net.Conn]struct{}
	active map[net.Conn]struct{}

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
	return &Mux{
		root:    make(map[string]*node),
		Timeout: 10 * time.Second,
		active:  make(map[net.Conn]struct{}),
		idle:    make(map[net.Conn]struct{}),
		Opened:  func(m *Mux) {},
		Closed:  func(m *Mux) {},
	}
}

//Closes the server gracefully
func (m *Mux) Close() error {
	var err error

	m.listener.closing = true

	for conn, _ := range m.idle {
		conn.Close()
	}

	if m.Timeout > 0 && len(m.active) > 0 {
		timer := time.NewTimer(m.Timeout)
		<-timer.C
		err = m.listener.Close()
	} else {
		err = m.listener.Close()
	}

	m.Closed(m)

	return err
}

// Accept connections and spawn a goroutine to serve each one.  Stop listening
// if anything is received on the service's channel.

func (m *Mux) activeConnection(conn net.Conn) {
	m.Lock()
	m.active[conn] = struct{}{}
	delete(m.idle, conn)
	m.Unlock()
}

func (m *Mux) removeConnection(conn net.Conn) {
	m.Lock()
	delete(m.active, conn)
	delete(m.idle, conn)
	m.Unlock()
}

func (m *Mux) idleConnection(conn net.Conn) {
	m.Lock()
	delete(m.active, conn)
	m.idle[conn] = struct{}{}
	m.Unlock()
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
