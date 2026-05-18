package main

import (
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"
)

const defaultHTTPRetries = 4

func wordPressHTTPClient() *http.Client {
	return &http.Client{Timeout: 2 * time.Minute}
}

type retryableError struct{ err error }

func (e retryableError) Error() string { return e.err.Error() }
func (e retryableError) Unwrap() error { return e.err }

func markRetryable(err error) error {
	if err == nil {
		return nil
	}
	return retryableError{err: err}
}

func isRetryableHTTPStatus(code int) bool {
	switch code {
	case 429, 502, 503, 504:
		return true
	default:
		return false
	}
}

func isRetryableError(err error) bool {
	if err == nil {
		return false
	}
	var r retryableError
	if errors.As(err, &r) {
		return true
	}
	return isRetryableNetErr(err)
}

func isRetryableNetErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.EPIPE) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	s := strings.ToLower(err.Error())
	for _, sub := range []string{
		"connection reset",
		"broken pipe",
		"connection refused",
		"tls handshake timeout",
		"i/o timeout",
		"timeout exceeded",
		"temporary failure in name resolution",
		"no route to host",
	} {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func withHTTPRetry(fn func() error) error {
	var last error
	for attempt := 0; attempt < defaultHTTPRetries; attempt++ {
		if attempt > 0 {
			wait := time.Duration(attempt) * 2 * time.Second
			logger.Warning("Retrying request (%d/%d) after %v: %v", attempt+1, defaultHTTPRetries, wait, last)
			time.Sleep(wait)
		}
		last = fn()
		if last == nil {
			return nil
		}
		if !isRetryableError(last) {
			return last
		}
	}
	return last
}
