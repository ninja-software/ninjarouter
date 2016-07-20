# NinjaRouter

NinjaRouter is a simple, fast, threadsafe tree based HTTP router for Go

#### Install

    go get github.com/blockninja/ninjarouter

    //or

    glide get github.com/blockninja/ninjarouter

#### Usage

```go
package main

import (
    "fmt"
    "github.com/blockninja/ninjarouter"
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
    rtr.Listen(":4545")
}

func helloName(w http.ResponseWriter, r *http.Request) {
    // Get named variable
    firstname, _ := ninjarouter.Var(r, "firstname")

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

#### Documentation

For further documentation, check out [GoDoc](http://godoc.org/github.com/BlockNinja/ninjarouter).

#### License
MIT