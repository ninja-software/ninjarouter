# NinjaRouter

NinjaRouter is a simple, fast, threadsafe tree based HTTP router for Go, which implements a graceful HTTP server for graceful closing of servers.

Not compatible with anything <1.7, as this router uses Context to store the REST parameters.

#### Install

    go get github.com/ninja-software/ninjarouter

    //or

    glide get github.com/ninja-software/ninjarouter

#### Usage

```go
package main

import (
    "fmt"
    "github.com/ninja-software/ninjarouter"
    "net/http"
    "io"
)

func main() {
    rtr := ninjarouter.New()
    // Supports named parameters.
    rtr.GET("/hello/:firstname", helloName)
    // You can overload variables (leading variables must be the same)
    rtr.GET("/hello/:firstname/:lastname", helloFullName)
    // Supports wildcards anywhere.
    rtr.GET("/pokemon/*", catchAll)
    // Even after variable catching
    rtr.GET("/hello/:firstname/:lastname/*", helloAll)
    // Wrap the file server handler in a http.HandlerFunc
    rtr.GET("/*", ninjarouter.HandlerFunc(http.FileServer(http.Dir("./public/"))))
    // Custom 404 handler.
    rtr.NotFound = notFound
    // Listen and serve.
    fmt.Println("Listening: :9000")
    if err := rtr.Listen(":9000"); err != nil {
        panic(err)
    }
}

func helloName(w http.ResponseWriter, r *http.Request) {
    // Get named variable
    params := r.Context().Value('params').(map[string]string)

    firstname, _ := params["firstname"]

    io.WriteString(w, fmt.Sprintf("hello, %s", firstname))
}

func helloFullName(w http.ResponseWriter, r *http.Request) {
    // Get a map of all
    // route variables.
    vrs, _ := ninjarouter.Vars(r)
    firstname := vrs["firstname"]
    lastname := vrs["lastname"]

    io.WriteString(w, fmt.Sprintf("hello, %s %s", firstname, lastname))
}

func helloAll(w http.ResponseWriter, r *http.Request) {
    // Get a map of all
    // route variables.
    vrs, _ := ninjarouter.Vars(r)
    firstname := vrs["firstname"]
    lastname := vrs["lastname"]

    io.WriteString(w, fmt.Sprintf("hello, %s %s and all!", firstname, lastname))
}

func catchAll(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("This catches 'em all"))
}

func notFound(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("404 go away"))
}
```

#### How to gracefully close the server

```go
func main() {
    rtr := ninjarouter.New()

    rtr.Timeout = 5 * time.Second // default is 10 seconds
    rtr.Opened = func(m *ninjarouter.Mux) {
        fmt.Printf("Opened server on port: %d\n", m.Port)
    }
    rtr.Closed = func(m *ninjarouter.Mux) {
        fmt.Println("closed server")
    }

    go func() {
        err := rtr.Listen("0.0.0.0:0")
        if err != nil {
            fmt.Println()
            os.Exit(-1)
        }
    }()
    c := make(chan os.Signal, 1)
    signal.Notify(c,
        syscall.SIGHUP,
        syscall.SIGINT,
        syscall.SIGTERM,
        syscall.SIGQUIT
        os.Interrupt)
    for sig := range c {
        rtr.Close()
        os.Exit(0)
    }
}
```

#### Documentation

For further documentation, check out [GoDoc](http://godoc.org/github.com/BlockNinja/ninjarouter).

#### License
MIT
