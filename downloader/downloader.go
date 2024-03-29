package downloader

import (
	"context"
	"errors"
	"github.com/ViRb3/go-parallel/throttler"
	"github.com/ViRb3/sling/v2"
	"io"
	"net/http"
	"os"
	"strconv"
)

type Job[T any] struct {
	SaveFilePath string
	Url          string
	Tag          T
}

type Result struct {
	Request  *http.Request
	Response *http.Response
	Job      Job[any]
	Skipped  bool
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
	// If true, send a HEAD request to each Job.Url before downloading.
	// If the HEAD request succeeds, contains a Content-Length header, there is a local file at the same path as
	// Job.SaveFilePath, and the local file's size matches Content-Length, then assume that the files are identical
	// and skip downloading.SkipSameLength bool
	SkipSameLength bool
	// How many parallel workers to run.
	Workers int
	// Download jobs to run.
	Jobs []Job[any]
	// If not empty, response status codes other than the defined will return an error.
	ExpectedStatusCodes []int
}

func NewMultiDownloader(config ConfigRaw) *MultiDownloader {
	downloader := MultiDownloader{
		client: sling.New().Client(config.Client),
		shared: config.SharedConfig,
	}
	return &downloader
}

func NewMultiDownloaderSling[T any](config ConfigSling) *MultiDownloader {
	downloader := MultiDownloader{
		client: config.Client,
		shared: config.SharedConfig,
	}
	return &downloader
}

func (s *MultiDownloader) IsStatusCodeExpected(code int) bool {
	if len(s.shared.ExpectedStatusCodes) < 1 {
		return true
	}
	for _, code2 := range s.shared.ExpectedStatusCodes {
		if code == code2 {
			return true
		}
	}
	return false
}

var (
	ErrUnexpectedStatusCode = errors.New("unexpected status code")
)

func (s *MultiDownloader) Run() (<-chan Result, context.CancelFunc) {
	ctx, cancelFunc := context.WithCancel(context.Background())

	downloadThrottler := throttler.NewThrottler(throttler.Config[Job[any], Result]{
		ShowProgress: s.shared.ShowProgress,
		Ctx:          ctx,
		ResultBuffer: 0,
		Workers:      s.shared.Workers,
		Source:       s.shared.Jobs,
		Operation: func(job Job[any]) Result {
			var result Result
			result.Err = func() error {
				req, err := s.client.New().Head(job.Url).Request()
				result = Result{Job: job, Request: req}
				if err != nil {
					return err
				}
				resp, err := s.client.New().DoRaw(req)
				result.Response = resp
				if err != nil {
					return err
				}
				// if HEAD is supported, compare file lengths to see if we can skip re-downloading
				if s.shared.SkipSameLength && resp.StatusCode >= 200 && resp.StatusCode <= 299 {
					if contentLen := resp.Header.Get("Content-Length"); contentLen != "" {
						contentLenInt, err := strconv.ParseInt(contentLen, 10, 64)
						if err == nil {
							if stat, err := os.Stat(job.SaveFilePath); err == nil && stat.Size() == contentLenInt {
								result.Skipped = true
								return nil
							}
						}
					}
				}
				req, err = s.client.New().Get(job.Url).Request()
				result = Result{Job: job, Request: req}
				if err != nil {
					return err
				}
				resp, err = s.client.New().DoRaw(req)
				result.Response = resp
				if err != nil {
					return err
				}
				defer resp.Body.Close()
				file, err := os.Create(job.SaveFilePath)
				if err != nil {
					return err
				}
				defer file.Close()
				if _, err := io.Copy(file, resp.Body); err != nil {
					os.Remove(file.Name())
					return err
				}
				return nil
			}()
			return result
		},
	})

	returnResult := make(chan Result, 10)
	go func() {
		for result := range downloadThrottler.Run() {
			if result.Err == nil && !s.IsStatusCodeExpected(result.Response.StatusCode) {
				result.Err = ErrUnexpectedStatusCode
			}
			returnResult <- result
		}
		close(returnResult)
	}()
	return returnResult, cancelFunc
}
