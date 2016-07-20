# NinjaRouter

NinjaRouter is a simple and fast HTTP router for Go, and has been forked from github.com/daryl/zeus

#### Install

    go get github.com/blockninja/ninjarouter

    glide get github.com/blockninja/ninjarouter

#### Usage

```go
package main

import (
    "fmt"
    "github.com/BlockNinja/ninjarouter"
    "net/http"
    "io"
)

func main() {
    rtr := ninjarouter.New()
    // Supports named parameters.
    rtr.GET("/hellow/:name", helloName)
    // Supports wildcards anywhere.
    rtr.GET("/pokemon/*", catchFoo)
    // Custom 404 handler.
    rtr.NotFound = notFound
    // Listen and serve.
    rtr.Listen(":4545")
}

func helloName(w http.ResponseWriter, r *http.Request) {
    // Get a map of all
    // route variables.
    vrs := ninjarouter.Vars(r)

    name := vrs["name"]

    io.WriteString(w, fmt.Sprintf("hello, %s", name))
}

func catchFoo(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("This catches 'em all"))
}

func notFound(w http.ResponseWriter, r *http.Request) {
    w.Write([]byte("404 go away"))
}
```

#### Documentation

For further documentation, check out [GoDoc](http://godoc.org/github.com/blockninja/ninjarouter).
