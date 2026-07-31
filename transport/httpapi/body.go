package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
)

// guardedRequestBody makes the Body.Close contract idempotent for the normal
// handler return path and the concurrent request-deadline path. net/http
// requires request bodies to allow Close concurrently with Read and to unblock
// a pending Read.
type guardedRequestBody struct {
	io.ReadCloser
	once sync.Once
	err  error
}

// Close closes the request body at most once and contains closer panics.
func (body *guardedRequestBody) Close() error {
	body.once.Do(func() {
		returned := false
		defer func() {
			if !returned {
				_ = recover()
				body.err = fmt.Errorf("close request body: callback panicked")
			}
		}()
		body.err = body.ReadCloser.Close()
		returned = true
	})
	return body.err
}

// bindRequestBody closes the body when ctx expires so a conforming Body.Read
// cannot outlive the handler's configured request deadline. The returned
// release function must be deferred by the handler.
func bindRequestBody(ctx context.Context, request *http.Request) func() {
	body := request.Body
	if body == nil {
		body = http.NoBody
	}
	guarded := &guardedRequestBody{ReadCloser: body}
	request.Body = guarded
	stop := context.AfterFunc(ctx, func() { _ = guarded.Close() })
	return func() {
		stop()
		_ = guarded.Close()
	}
}
