package ninjarouter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func Benchmark_GET_Simple(b *testing.B) {
	router := New()
	router.GET("/action", helloHandler)

	rw, req := testRequest("GET", "/action")

	for i := 0; i < b.N; i++ {
		router.ServeHTTP(rw, req)
	}
}

func Benchmark_GET_Extreme(b *testing.B) {
	router := New()
	router.GET("/action/:lots/:of/:vars", helloVarsHandler)

	rw, req := testRequest("GET", "/action/:lots/:of/:vars")

	for i := 0; i < b.N; i++ {
		router.ServeHTTP(rw, req)
	}
}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "hello")
}

func helloVarsHandler(w http.ResponseWriter, r *http.Request) {
	params, _ := Vars(r)
	fmt.Fprintf(w, "hello %s %s %s %s", params["action"], params["lots"], params["of"], params["vars"])
}

func testRequest(method, path string) (*httptest.ResponseRecorder, *http.Request) {
	request, _ := http.NewRequest(method, path, nil)
	recorder := httptest.NewRecorder()

	return recorder, request
}
