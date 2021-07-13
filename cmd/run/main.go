package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/01-edu/runner"
)

func run(image string, env, args []string, b []byte) ([]byte, bool, error) {
	return runner.Run(&http.Request{
		URL: &url.URL{
			RawQuery: url.Values{
				"args": args,
				"env":  env,
			}.Encode(),
			Path: image,
		},
		Body: io.NopCloser(bytes.NewReader(b)),
	})
}

func main() {
	var (
		filename = flag.String("file", "archive.zip", "Archive containing the sources to run")
		image    = flag.String("image", "alpine", "Docker image to run")
		parallel = flag.Uint("parallel", 1, "Number of tests to execute in parallel")
		total    = flag.Uint("total", 1, "Total number of tests")
	)
	flag.Parse()
	b, err := os.ReadFile(*filename)
	if err != nil {
		panic(err)
	}
	queue := make(chan struct{})
	var wg sync.WaitGroup
	for i := uint(0); i < *parallel; i++ {
		wg.Add(1)
		go func() {
			for range queue {
				b, ok, err := run(*image, nil, flag.Args(), b)
				if err != nil {
					panic(err)
				}
				if !ok {
					panic(string(b))
				}
			}
			wg.Done()
		}()
	}
	log.SetOutput(io.Discard)
	before := time.Now()
	for i := uint(0); i < *total; i++ {
		queue <- struct{}{}
	}
	close(queue)
	wg.Wait()
	fmt.Println("avg", (time.Since(before) / time.Duration(*total)).Round(time.Millisecond))
}
