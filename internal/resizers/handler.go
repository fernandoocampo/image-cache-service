package resizers

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
)

// HandlerMaker defines behavior to create http handlers.
type HandlerMaker struct {
	service *Service
}

// NewHandlerMaker creates a new resizer service handler maker.
func NewHandlerMaker(service *Service) *HandlerMaker {
	newHandler := HandlerMaker{
		service: service,
	}

	return &newHandler
}

const asyncParamName string = "async"

// MakeResizeHandler creates a new resize handler.
func (h *HandlerMaker) MakeResizeHandler() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Expecting POST request"))
			return
		}

		var request ResizeRequest

		err := json.NewDecoder(io.LimitReader(r.Body, 8*1024)).Decode(&request)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Failed to parse request"))
			return
		}

		filters := r.URL.Query()
		if v, ok := filters[asyncParamName]; ok {
			asyncValue, err := strconv.ParseBool(v[0])
			if err != nil {
				slog.Debug("the async parameter exists but is an invalid boolean value", slog.Any("value", v[0]))
				// Ignore the error because it is interpreted as a synchronous request
			}

			slog.Debug("async value", slog.Bool("value", asyncValue))

			request.Async = asyncValue
		}

		results, err := h.service.ProcessResizes(request)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Failed to process request"))
			return
		}

		data, err := json.Marshal(results)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Failed to marshal response"))
			return
		}

		w.WriteHeader(http.StatusCreated)
		w.Header().Add("content-type", "application/json")
		w.Write(data)
	})
}

// MakeGetImageHandler creates a new get image handler.
func (h *HandlerMaker) MakeGetImageHandler() http.HandlerFunc {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Info("fetching", slog.String("image", r.URL.String()))

		data, err := h.service.GetImage(r.Context(), r.URL.String())
		if err != nil && !errors.Is(err, errContextCancelled) {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if errors.Is(err, errContextCancelled) {
			w.WriteHeader(http.StatusRequestTimeout)
			return
		}

		if data == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Header().Add("content-type", "image/jpeg")
		w.Write(data)
	})
}
