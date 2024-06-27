package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ReanSn0w/tk4go/pkg/config"
	"github.com/ReanSn0w/tk4go/pkg/tools"
	"github.com/go-chi/chi"
	"github.com/go-pkgz/lgr"
)

var (
	revision = "dev"
	log      = lgr.Default()
	opts     = struct {
		Debug      bool   `long:"debug" env:"DEBUG" description:"Enable debug prints"`
		ListenExit bool   `short:"x" long:"listen-exit" env:"LISTEN_EXIT"`
		Port       int    `short:"p" long:"port" env:"PORT" default:"8080" desctiption:"Port to listen on"`
		Dest       string `short:"d" long:"dest" env:"DEST" default:"localhost:3000" description:"Address to pass"`
	}{}

	parsedURL *url.URL
)

func main() {
	err := config.Parse(&opts)
	if err != nil {
		log.Logf("[INFO] %s", err)
		os.Exit(2)
	}

	if opts.Debug {
		lgr.Setup(lgr.Debug, lgr.CallerFunc)
		lgr.Format(lgr.FullDebug)
		log = lgr.Default()
	}

	config.Print(log, "ReqLog", revision, opts)

	parsedURL, err = url.Parse(opts.Dest)
	if err != nil {
		log.Logf("[ERROR] %s", err)
		os.Exit(2)
	}

	ctx, cancel := context.WithCancelCause(context.TODO())
	defer cancel(nil)

	gs := tools.NewShutdownStack(log)
	srv := buildServer(cancel)
	gs.Add(func(ctx context.Context) {
		err := srv.Shutdown(ctx)
		if err != nil {
			log.Logf("[ERROR] server shutdown err: %s", err)
		}
	})

	if opts.ListenExit {
		go tools.AnyKeyToExit(log, func() {
			cancel(nil)
		})
	}

	gs.Wait(ctx, time.Second*3)
}

func buildServer(cancel context.CancelCauseFunc) *http.Server {
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", opts.Port),
		Handler: handler(),
	}

	go func() {
		log.Logf("[INFO] Starting server on port: %v", opts.Port)

		err := srv.ListenAndServe()
		if err != nil {
			cancel(err)
		}
	}()

	return srv
}

func handler() http.Handler {
	r := chi.NewRouter()
	r.Use(logData)
	r.HandleFunc("/*", proxyRequest)
	return r
}

func logData(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rb := &responseBuffer{w: w, buf: new(bytes.Buffer)}
		h.ServeHTTP(rb, r)
		respBytes := rb.buf.Bytes()

		defer func() {
			reqBytes, _ := httputil.DumpRequest(r, true)

			headers := ""
			for k, v := range w.Header() {
				headers += fmt.Sprintf("%v=%v\n", k, strings.Join(v, "; "))
			}

			lgr.Default().Logf(
				"[INFO] Handled:\n\nRequest:\n%s\n\nResponse:\n%s%v\n---",
				string(reqBytes), headers, string(respBytes))
		}()

		w.Write(respBytes)
	})
}

func proxyRequest(w http.ResponseWriter, r *http.Request) {
	originalURL := *r.URL

	{
		originalURL.Scheme = parsedURL.Scheme
		originalURL.User = parsedURL.User
		originalURL.Host = parsedURL.Host
	}

	req, err := http.NewRequest(r.Method, originalURL.String(), r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for k := range r.Header {
		req.Header.Add(k, r.Header.Get(k))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	defer resp.Body.Close()

	w.WriteHeader(resp.StatusCode)
	for k := range resp.Header {
		w.Header().Add(k, resp.Header.Get(k))
	}

	buffer := new(bytes.Buffer)
	buffer.ReadFrom(resp.Body)
	buffer.WriteTo(w)
}

// MARK: - ResponseBuffer
// Структура для записи ответа сервера в буффер

type responseBuffer struct {
	w    http.ResponseWriter
	buf  *bytes.Buffer
	code int
}

func (r *responseBuffer) Header() http.Header {
	return r.w.Header()
}

func (r *responseBuffer) Write(b []byte) (int, error) {
	return r.buf.Write(b)
}

func (r *responseBuffer) WriteHeader(statusCode int) {
	r.code = statusCode
	r.w.WriteHeader(statusCode)
}
