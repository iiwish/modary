package httpapi

import "net/http"

// trackedResponseWriter records response commitment while preserving access to
// the underlying writer through http.ResponseController.
type trackedResponseWriter struct {
	http.ResponseWriter
	written bool
}

// WriteHeader commits at most one response status and records that commitment
// before calling the underlying dependency.
func (writer *trackedResponseWriter) WriteHeader(status int) {
	if writer.written {
		return
	}
	writer.written = true
	writer.ResponseWriter.WriteHeader(status)
}

// Write commits an implicit success status when needed and forwards body data.
func (writer *trackedResponseWriter) Write(data []byte) (int, error) {
	if !writer.written {
		writer.WriteHeader(http.StatusOK)
	}
	return writer.ResponseWriter.Write(data)
}

// Written reports whether response headers have been committed.
func (writer *trackedResponseWriter) Written() bool {
	return writer != nil && writer.written
}

// Unwrap exposes the underlying writer to net/http response controllers.
func (writer *trackedResponseWriter) Unwrap() http.ResponseWriter {
	if writer == nil {
		return nil
	}
	return writer.ResponseWriter
}

// containResponsePanic writes a stable fallback only before commitment. Once a
// response is committed, net/http must abort the connection or stream so a
// truncated response cannot be mistaken for a complete one. The original panic
// value is deliberately discarded because it may be caller-owned and unsafe to
// format. A fallback-writer panic is reduced to the same abort sentinel.
func containResponsePanic(writer *trackedResponseWriter, fallback func()) {
	if writer == nil || writer.Written() || fallback == nil {
		panic(http.ErrAbortHandler)
	}
	returned := false
	defer func() {
		if !returned {
			_ = recover()
			panic(http.ErrAbortHandler)
		}
	}()
	fallback()
	returned = true
	if !writer.Written() {
		panic(http.ErrAbortHandler)
	}
}
