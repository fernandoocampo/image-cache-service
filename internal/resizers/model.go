package resizers

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
)

type ResizeRequest struct {
	Async  bool     `json:"-"`
	URLs   []string `json:"urls"`
	Width  uint     `json:"width"`
	Height uint     `json:"height"`
}

type ResizeRequestAsync struct {
	Key    string
	URL    string
	Width  uint
	Height uint
}

type ResizeResult struct {
	Result string `json:"result"`
	URL    string `json:"url,omitempty"`
	Cached bool   `json:"cached"`
}

type WorkerResizeResult struct {
	value chan string
	done  chan struct{}
}

type ResizeData struct {
	Key           string
	URL           string
	Width, Height uint
}

const (
	proto      = "http://"
	hostport   = "localhost:8080"
	success    = "success"
	inProgress = "in-progress"
	failure    = "failure"
)

const (
	imageURLOutputPath = "/v1/image/"
	jpegExtension      = ".jpeg"
)

// errors
var (
	errContextCancelled = errors.New("context was cancelled")
)

func (w *WorkerResizeResult) Close() {
	close(w.done)
	close(w.value)
}

func genID(url string) string {
	hash := sha256.Sum256([]byte(url))
	return base64.URLEncoding.EncodeToString(hash[:])
}

func newURL(key string) string {
	return fmt.Sprintf("%s%s%s", proto, hostport, key)
}

func genKey(url string) string {
	return fmt.Sprintf("%s%s%s", imageURLOutputPath, genID(url), jpegExtension)
}
