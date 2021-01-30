package downloader

import (
	"context"
	"github.com/ViRb3/go-parallel/throttler"
	"github.com/ViRb3/sling/v2"
	"io"
	"net/http"
	"os"
	"strconv"
)

type Request struct {
	SaveFilePath string
	Url          string
	Tag          interface{}
}

type Result struct {
	Response *http.Response
	Request  Request
	Err      error
}

type MultiDownloader struct {
	client *sling.Sling
	shared SharedConfig
}

type ConfigRaw struct {
	// The HTTP client to use for all requests.
	Client *http.Client
	SharedConfig
}

type ConfigSling struct {
	// The HTTP client to use for all requests.
	Client *sling.Sling
	SharedConfig
}

type SharedConfig struct {
	// If true, continuously print a progress bar.
	ShowProgress bool
	// If true, send a HEAD request to each Request.Url before downloading.
	// If the HEAD request succeeds, contains a Content-Length header, there is a local file at the same path as
	// Request.SaveFilePath, and the local file's size matches Content-Length, then assume that the files are identical
	// and skip downloading.SkipSameLength bool
	SkipSameLength bool
	// How many parallel workers to run.
	Workers int
	// Download requests to run.
	Requests []Request
}

func NewMultiDownloader(config ConfigRaw) *MultiDownloader {
	downloader := MultiDownloader{
		client: sling.New().Client(config.Client),
		shared: config.SharedConfig,
	}
	return &downloader
}

func NewMultiDownloaderSling(config ConfigSling) *MultiDownloader {
	downloader := MultiDownloader{
		client: config.Client,
		shared: config.SharedConfig,
	}
	return &downloader
}

func (s *MultiDownloader) Run() (<-chan Result, context.CancelFunc) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	var source []interface{}
	for i := range s.shared.Requests {
		source = append(source, s.shared.Requests[i])
	}

	downloadThrottler := throttler.NewThrottler(throttler.Config{
		ShowProgress: s.shared.ShowProgress,
		Ctx:          ctx,
		ResultBuffer: 0,
		Workers:      s.shared.Workers,
		Source:       source,
		Operation: func(sourceItem interface{}) interface{} {
			request := sourceItem.(Request)
			resp, err := s.client.New().Head(request.Url).Receive(nil, nil)
			if err != nil {
				return Result{resp, request, err}
			}
			// if HEAD is supported, compare file lengths to see if we can skip re-downloading
			if s.shared.SkipSameLength && resp.StatusCode >= 200 && resp.StatusCode <= 299 {
				if contentLen := resp.Header.Get("Content-Length"); contentLen != "" {
					contentLenInt, err := strconv.ParseInt(contentLen, 10, 64)
					if err == nil {
						if stat, err := os.Stat(request.SaveFilePath); err == nil && stat.Size() == contentLenInt {
							return Result{resp, request, nil}
						}
					}
				}
			}
			resp, err = s.client.New().Get(request.Url).ReceiveBody()
			if err != nil {
				return Result{resp, request, err}
			}
			defer resp.Body.Close()
			file, err := os.Create(request.SaveFilePath)
			if err != nil {
				return Result{resp, request, err}
			}
			defer file.Close()
			if _, err := io.Copy(file, resp.Body); err != nil {
				os.Remove(file.Name())
				return Result{resp, request, err}
			}
			return Result{resp, request, nil}
		},
	})

	returnResult := make(chan Result, 10)
	go func() {
		for result := range downloadThrottler.Run() {
			returnResult <- result.(Result)
		}
		close(returnResult)
	}()
	return returnResult, cancelFunc
}
