package main

import (
	"encoding/json"
	"flag"
	"net/http"
	"time"

	"github.com/01-edu/runner"
)

func handle(rw http.ResponseWriter, r *http.Request) {
	b, ok, err := runner.RunTest(r)
	message := string(b)
	if err != nil {
		message = err.Error()
		rw.WriteHeader(http.StatusBadRequest)
	}
	json.NewEncoder(rw).Encode(struct {
		Output string
		Ok     bool
	}{message, ok})
}

func main() {
	http.HandleFunc("/", handle)
	port := flag.String("port", "8080", "listening port")
	flag.Parse()
	srv := &http.Server{
		Addr:         ":" + *port,
		ReadTimeout:  time.Minute,
		WriteTimeout: time.Minute,
	}
	srv.ListenAndServe()
}
