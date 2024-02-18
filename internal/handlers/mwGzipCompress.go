package handlers

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"
)

type gzipResponseWriter struct {
	http.ResponseWriter
	w *gzip.Writer
}

func (gzw gzipResponseWriter) Write(b []byte) (int, error) {
	return gzw.w.Write(b)
}

func (gzw gzipResponseWriter) Close() error {
	return gzw.w.Close()
}

func newGzipResponseWriter(w http.ResponseWriter) gzipResponseWriter {
	return gzipResponseWriter{ResponseWriter: w, w: gzip.NewWriter(w)}
}

func GzipHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		acceptGzip := strings.Contains(r.Header.Get("Accept-Encoding"), "gzip")
		encodedGzip := strings.Contains(r.Header.Get("Content-Encoding"), "gzip")

		if encodedGzip {
			gr, _ := gzip.NewReader(r.Body)
			bodyData := bytes.Buffer{}
			_, err := bodyData.ReadFrom(gr)
			if err != nil {
				http.Error(w, "GZIP decompression error", 500)
			}
			r.Body = io.NopCloser(bytes.NewReader(bodyData.Bytes()))
			r.ContentLength = int64(len(bodyData.Bytes()))
		}

		if acceptGzip {
			wh := newGzipResponseWriter(w)
			defer wh.Close()
			w.Header().Set("Content-Encoding", "gzip")
			h.ServeHTTP(wh, r)
			return
		}

		h.ServeHTTP(w, r)

	})
}
