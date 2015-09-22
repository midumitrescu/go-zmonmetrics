// go package for use with goat (https://github.com/bahlo/goat) and
// zmon (https://github.com/zalando/zmon) to collect metrics
package zmonmetrics

import (
	"github.com/bahlo/goat"
	metrics "github.com/rcrowley/go-metrics"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Metrics struct {
	handler  http.Handler
	UrlToKey func(r *http.Request, c int) string
}

// this one needs to be wrapped in main like
//  func ZMon2Metrics (h http.Handler) http.Handler {
//     return zmon2.Handler(h, url2key)
//  }
//
//  // this is the default if the func is nil:
//  func url2key(r *http.Request, code int) (url string) {
//     // bare minimum is to replace all "/" by ".":
//     url = strings.Replace(r.URL.Path, "//", "/", -1)[1:] // exclude leading "/"
//     url = strings.Replace(url, "/", ".", -1)
//     return
//  }
//
// ... and use it:
//  func main() {
//    ...
//    r.Use(ZMon2Metrics)
//    r.Get("/metrics", "ZMon2 Metrics", zmon2.MetricsHandler)
//  }
func Handler(h http.Handler, u func(r *http.Request, c int) string) http.Handler {
	if u == nil {
		u = defaultUrlToKey
	}
	return &Metrics{
		handler:  h,
		UrlToKey: u,
	}
}

func defaultUrlToKey(r *http.Request, c int) (url string) {
	url = strings.Replace(r.URL.Path, "//", "/", -1)[1:] // exclude leading "/"
	url = strings.Replace(url, "/", ".", -1)
	return url
}

var reg = metrics.NewRegistry()

func Registry() metrics.Registry {
	return reg
}

func MetricsHandler(w http.ResponseWriter, r *http.Request, p goat.Params) {
	goat.WriteJSON(w, reg)
	return
}

func (m *Metrics) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	now := time.Now()

	h := &monitor{writer: w}
	m.handler.ServeHTTP(h, r)

	dur := time.Now().Sub(now)

	if h.code == 0 {
		h.code = 200
	}
	key := "zmon.response." + strconv.Itoa(h.code) + "." + r.Method + "." + m.UrlToKey(r, h.code)
	metrics.GetOrRegisterTimer(key, reg).Update(dur)
}

// the http.ResponseWriter implementation to record the code
type monitor struct {
	writer http.ResponseWriter
	code   int
	bytes  int64
}

func (m *monitor) Write(data []byte) (count int, err error) {
	count, err = m.writer.Write(data)
	m.bytes = int64(count)
	return
}

func (m *monitor) WriteHeader(code int) {
	m.writer.WriteHeader(code)
	if code == 0 {
		code = 200
	}
	m.code = code
}

func (m *monitor) Header() http.Header {
	return m.writer.Header()
}

// vim: ts=4 sw=4 noexpandtab nolist syn=go
