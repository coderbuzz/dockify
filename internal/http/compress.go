package http

import (
	"bufio"
	"errors"
	"net"
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// compressMinSize: smaller bodies are sent as-is; gzip framing would cost more than it saves.
const compressMinSize = 1024

// compress gzips text responses of at least compressMinSize bytes. WebSockets pass through:
// the 101 response has no Content-Type, so chi never wraps the hijacked connection.
func compress(next http.Handler) http.Handler {
	gz := chimw.Compress(5, "text/html", "text/css", "text/javascript", "application/javascript",
		"application/json", "image/svg+xml", "application/manifest+json")
	return gz(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mw := &minSizeWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(mw, r)
		mw.finish()
	}))
}

// minSizeWriter holds the first compressMinSize bytes back. If the handler ends before that,
// the body goes to the writer under chi's compressor, which never decides to compress.
type minSizeWriter struct {
	http.ResponseWriter // chi's compress writer
	code                int
	buf                 []byte
	committed           bool
}

func (m *minSizeWriter) WriteHeader(code int) {
	if m.committed {
		m.ResponseWriter.WriteHeader(code)
		return
	}
	m.code = code
	if code < 200 || code == http.StatusNoContent || code == http.StatusNotModified {
		m.commit() // no body follows
	}
}

func (m *minSizeWriter) Write(p []byte) (int, error) {
	if m.committed {
		return m.ResponseWriter.Write(p)
	}
	m.buf = append(m.buf, p...)
	if len(m.buf) >= compressMinSize {
		if err := m.commit(); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

// commit hands the status and buffered bytes to chi, which then picks gzip by Content-Type.
func (m *minSizeWriter) commit() error {
	m.committed = true
	m.ResponseWriter.WriteHeader(m.code)
	if len(m.buf) == 0 {
		return nil
	}
	_, err := m.ResponseWriter.Write(m.buf)
	m.buf = nil
	return err
}

func (m *minSizeWriter) finish() {
	if m.committed {
		return
	}
	m.committed = true
	raw := m.ResponseWriter
	if u, ok := raw.(interface{ Unwrap() http.ResponseWriter }); ok {
		raw = u.Unwrap()
	}
	raw.WriteHeader(m.code)
	raw.Write(m.buf)
}

func (m *minSizeWriter) Flush() {
	if !m.committed {
		m.commit()
	}
	if f, ok := m.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (m *minSizeWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	m.committed = true // the handler owns the connection now
	if hj, ok := m.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, errors.New("http.Hijacker unavailable")
}

func (m *minSizeWriter) Unwrap() http.ResponseWriter { return m.ResponseWriter }
