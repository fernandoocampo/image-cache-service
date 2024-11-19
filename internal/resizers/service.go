package resizers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"bufio"
	"bytes"
	"image/jpeg"
	"io"
	"net/http"

	jpgresize "github.com/nfnt/resize"

	lru "github.com/hashicorp/golang-lru"
)

type ServiceSetup struct {
	Cache *lru.Cache
}

type Service struct {
	cache              *lru.Cache
	workerWorkStream   <-chan string
	workerResultStream chan (<-chan string)
	workers            sync.Map
	cacheDeadline      sync.Map
}

// NewService create a new resizer service
func NewService(setup *ServiceSetup) *Service {
	svc := Service{
		cache:              setup.Cache,
		workerResultStream: make(chan (<-chan string)),
	}

	return &svc
}

// GetImage get image from cache, if the image is still in the process of being resized,
// will wait until the context deadline or when the image has finished processing.
func (s *Service) GetImage(ctx context.Context, key string) ([]byte, error) {
	err := s.waitIfImageIsStillInProgress(ctx, key)
	if err != nil && errors.Is(err, errContextCancelled) {
		return nil, errContextCancelled
	}

	if err != nil {
		slog.Error("waiting for image to be processed", slog.String("error", err.Error()))

		return nil, fmt.Errorf("unable to check if image is still in resize process: %w", err)
	}

	data, ok := s.cache.Get(key)
	if !ok {
		return nil, nil
	}

	result, ok := data.([]byte)
	if !ok {
		slog.Info("invalid data stored in cache", slog.String("type", fmt.Sprintf("%T", data)))
		return nil, nil
	}

	return result, nil
}

// ProcessResizes resize images based on the given parameters. If request is marked
// as asynchronous the service will resize the image asynchronously.
func (s *Service) ProcessResizes(request ResizeRequest) ([]ResizeResult, error) {
	var err error
	var result []ResizeResult
	switch request.Async {
	case true:
		slog.Debug("processing async")
		result, err = s.processResizesAsync(request)
	case false:
		slog.Debug("processing sync")
		result, err = s.processResizesSync(request)
	}

	if err != nil {
		slog.Error("processing resize request", slog.String("error", err.Error()))
		return nil, fmt.Errorf("unable to resize request: %w", err)
	}

	return result, nil
}

// processResizesSync resize image synchronously.
func (s *Service) processResizesSync(request ResizeRequest) ([]ResizeResult, error) {
	results := make([]ResizeResult, 0, len(request.URLs))
	for _, url := range request.URLs {
		var result ResizeResult
		key := genKey(url)
		newURL := newURL(key)

		if s.cache.Contains(key) {
			result.URL = newURL
			result.Result = success
			result.Cached = true
			results = append(results, result)
			continue
		}

		resizeData := ResizeData{
			Key:    key,
			URL:    url,
			Width:  request.Width,
			Height: request.Height,
		}

		err := s.resizeAndCache(resizeData)
		if err != nil {
			slog.Error("resizing and caching", slog.String("url", url), slog.String("error", err.Error()))
			result.Result = failure
			results = append(results, result)
			continue
		}

		result.URL = newURL
		result.Result = success
		result.Cached = false
		results = append(results, result)
	}

	return results, nil
}

// processResizesAsync resize image synchronously.
func (s *Service) processResizesAsync(request ResizeRequest) ([]ResizeResult, error) {
	results := make([]ResizeResult, 0, len(request.URLs))
	for _, url := range request.URLs {
		var result ResizeResult

		key := genKey(url)
		result.URL = newURL(key)

		if s.cache.Contains(key) {
			result.Result = success
			result.Cached = true
			results = append(results, result)
			continue
		}

		result.Result = inProgress
		result.Cached = false

		workerResizeData := ResizeData{
			Key:    key,
			URL:    url,
			Width:  request.Width,
			Height: request.Height,
		}

		s.makeWorkerResizeImage(workerResizeData)

		results = append(results, result)
	}

	return results, nil
}

// makeWorkerResizeImage create a worker and run it to resize an image asynchronously
func (s *Service) makeWorkerResizeImage(resizeData ResizeData) {
	ctx := context.Background() // in case a timeout is needed for workers.
	workerWork := s.newResizeWorker(ctx, resizeData)
	s.workers.Store(resizeData.Key, workerWork)
	// s.watchProcess(ctx, workerWork.value)
}

// waitIfImageIsStillInProgress check if the requested image is still in progress and wait.
// if the image is not in progress then it will return immediately.
func (s *Service) waitIfImageIsStillInProgress(ctx context.Context, key string) error {
	value, ok := s.workers.Load(key)
	if !ok {
		return nil
	}

	workerResult, ok := value.(WorkerResizeResult)
	if !ok {
		slog.Info("invalid worker result value stored in workers map", slog.String("type", fmt.Sprintf("%T", value)))
		return nil
	}

	slog.Debug("waiting for image to be resized", slog.String("image", key))

	select {
	case <-ctx.Done():
		slog.Debug("context was cancelled", slog.String("error", ctx.Err().Error()))
		return errContextCancelled
	case <-workerResult.done:
		slog.Debug("image was processed", slog.String("image", key))
	}

	return nil
}

// StartWorkerHandler start the worker lifecycle handler.
func (s *Service) StartWorkerHandler(ctx context.Context) {
	s.workerWorkStream = s.getWorkerWorkStream(ctx)
	s.startWorkerCleaner(ctx)
}

// getWorkerWorkStream creates a bridge to centralize worker works stream in one channel
func (s *Service) getWorkerWorkStream(ctx context.Context) <-chan string {
	bridgeStream := make(chan string)
	go func() {
		defer close(bridgeStream)

		for {
			var stream <-chan string

			select {
			case <-ctx.Done():
				return
			case maybeStream, ok := <-s.workerResultStream:
				if !ok {
					return
				}

				stream = maybeStream
			}
			// read values off stream and add them to bridge stream
			// once the stream is closed we continue with the other
			// channels.
			for val := range stream {
				select {
				case <-ctx.Done():
				case bridgeStream <- val:
				}
			}
		}
	}()
	return bridgeStream
}

// watchProcess creates a goroutine to push worker result channel into the worker work stream bridge.
func (s *Service) watchProcess(ctx context.Context, workerResult <-chan string) {
	go func() {
		select {
		case <-ctx.Done():
			return
		case s.workerResultStream <- workerResult:
		}
	}()
}

// startWorkerCleaner starts worker cleaner to remove worker results from
// worker work map
func (s *Service) startWorkerCleaner(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case key, ok := <-s.workerWorkStream:
				if !ok {
					return
				}
				slog.Debug("deleting processed image", slog.String("key", key))
				s.workers.Delete(key)
			}
		}
	}()
}

// newResizeWorker creates a new resize worker to process the given image.
func (s *Service) newResizeWorker(ctx context.Context, request ResizeData) WorkerResizeResult {
	resultStream := WorkerResizeResult{
		value: make(chan string),
		done:  make(chan struct{}),
	}

	go func() {
		defer resultStream.Close()

		select {
		case <-ctx.Done():
			slog.Info("context was cancelled while creating a new worker", "err", ctx.Err())
			return
		default:

			err := s.resizeAndCache(request)
			if err != nil {
				slog.Error("resizing and caching image", slog.String("url", request.URL), slog.String("error", err.Error()))
			}
			s.workers.Delete(request.Key)
			// select {
			// case <-ctx.Done():
			// 	slog.Info("context was cancelled while resizing an image", "err", ctx.Err())
			// 	return
			// case resultStream.value <- request.Key:
			// }
		}
	}()

	return resultStream
}

// resizeAndCache resize image and cache the result.
func (s *Service) resizeAndCache(request ResizeData) error {
	data, err := fetchAndResize(request)
	if err != nil {
		slog.Error("resizing image", slog.String("url", request.URL), slog.String("error", err.Error()))

		return fmt.Errorf("unable to resize: %w", err)
	}

	slog.Debug("caching", slog.String("key", request.Key))
	_ = s.cache.Add(request.Key, data)
	s.cacheDeadline.Store(request.Key, time.Now())

	return nil
}

func fetchAndResize(request ResizeData) ([]byte, error) {
	data, err := fetch(request.URL)
	if err != nil {
		return nil, err
	}

	return resize(data, request.Width, request.Height)
}

func fetch(url string) ([]byte, error) {
	slog.Debug("fetching", slog.String("url", url))

	r, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch failed: %v", err)
	}
	defer r.Body.Close()

	if r.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("non-200 status: %d", r.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(r.Body, 15*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("failed to read fetch data: %v", err)
	}

	return data, nil
}

func resize(data []byte, width uint, height uint) ([]byte, error) {
	// decode jpeg into image.Image
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to jped decode: %v", err)
	}

	// if either width or height is 0, it will resize respecting the aspect ratio
	newImage := jpgresize.Resize(width, height, img, jpgresize.Lanczos3)

	newData := bytes.Buffer{}
	err = jpeg.Encode(bufio.NewWriter(&newData), newImage, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to jpeg encode resized image: %v", err)
	}

	return newData.Bytes(), nil
}

func (s *Service) StartCacheEviction(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
				s.cacheDeadline.Range(func(key any, value any) bool {
					timeValue, ok := value.(time.Time)
					if !ok {
						slog.Info("hey that's a time value")
						return true
					}
					calculatedTime := timeValue.Add(5 * time.Second)
					if calculatedTime.Before(time.Now()) {
						return true
					}

					ok = s.cache.Remove(key)
					if !ok {
						return true
					}

					slog.Info("removed")

					s.cacheDeadline.Delete(key)

					return true
				})
			}
		}
	}()
}
