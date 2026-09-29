package main

import (
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	os.Exit(run())
}

func run() int {
	if len(os.Args) < 2 {
		return 1
	}
	flags := flag.NewFlagSet("retry", flag.ContinueOnError)
	method := flags.String("method", "GET", "HTTP method")
	maxAttempts := flags.Int("max-attempts", 5, "maximum number of attempts")
	idempotencyKey := flags.String("idempotency-key", "", "Idempotency-Key")
	if flags.Parse(os.Args[2:]) != nil || *maxAttempts < 1 || flags.NArg() != 0 {
		return 1
	}

	req, err := http.NewRequest(strings.ToUpper(*method), os.Args[1], nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if req.URL.Scheme != "http" || req.URL.Host == "" {
		fmt.Fprintln(os.Stderr, "expected HTTP url")
		return 1
	}
	if *idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", *idempotencyKey)
	}

	canRetryByMethod := false
	switch req.Method {
	case "POST":
		canRetryByMethod = *idempotencyKey != ""
	case "GET", "HEAD", "PUT", "DELETE", "OPTIONS", "TRACE":
		canRetryByMethod = true
	}

	conn := &http.Client{
		Timeout:   5 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true},
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	backoff := 400
	for i := 1; ; i++ {
		resp, err := conn.Do(req)
		if err == nil {
			_, err = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		canRetryByStatusCode := err != nil
		if err != nil {
			fmt.Printf("attempt %d error %v\n", i, err)
		} else {
			fmt.Printf("attempt %d status %d\n", i, resp.StatusCode)
			// if resp.StatusCode == 200 {}
			if resp.StatusCode >= 200 && resp.StatusCode <= 399 {
				fmt.Printf("result success attempts %d\n", i)
				return 0
			}

			switch resp.StatusCode {
			case 429, 500, 502, 503, 504:
				canRetryByStatusCode = true
			}

		}
		if !canRetryByMethod || !canRetryByStatusCode || i == *maxAttempts {
			fmt.Printf("result failure attempts %d\n", i)
			return 1
		}

		wait := time.Duration(rand.IntN(backoff+1)) * time.Millisecond
		if resp != nil {
			s, err := strconv.ParseUint(resp.Header.Get("Retry-After"), 10, 32)
			if err == nil {
				wait = time.Duration(s) * time.Second
			}
		}
		fmt.Printf("sleep_ms %d\n", wait.Milliseconds())
		time.Sleep(wait)

		backoff = min(backoff*2, 2000)
	}
}
