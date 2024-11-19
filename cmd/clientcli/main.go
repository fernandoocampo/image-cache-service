package main

import (
	"context"
	"net/http"
	"time"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost:8080/v1/image/ROTm3h7JoDi_3H92HK3pIkCcWtedRl6NjTDKM7J4hkY=.jpeg", nil)
	if err != nil {
		panic(err)
	}

	_, err = http.DefaultClient.Do(r)
	// panic: context deadline exceeded
	if err != nil {
		panic(err)
	}
}
