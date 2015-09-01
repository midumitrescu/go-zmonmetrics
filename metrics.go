// go package for use with goat (https://github.com/bahlo/goat) and 
// zmon (https://github.com/zalando/zmon) to collect metrics
package zmonmetrics

import (
	"github.com/bahlo/goat"
	"math"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"
)

var mutex = &sync.Mutex{}

type apiMetric struct {
	Time time.Duration
	When time.Time
}

var apiMetrics = make(map[string][]apiMetric)

type Metrics struct {
	handler  http.Handler
	UrlToKey func(r *http.Request) string
}

// The "Metrics" struct needs to be wrapped as Handler
// somewhere in main:
//
//  import (
//     zmon2 "github.com/zalando/go-zmonmetrics"
//     "net/http"
//     "strings"
//  )
// 
//  func zMetrics(h http.Handler) {
//	  return zmon2.Handler(h, url2key)
//  }
// 
//  func url2key(r *http.Request) string {
//    e := strings.Split(strings.Replace(r.URL.Path, "//", "/", -1), "/")
//    if len(e) > 1 {
//       e[1] = "{id}"
//    }
//    return strings.Join(e, ".")
//  }
//  ...
//  
//  go zmon2.ExpireMetrics()
//  r.Use(zMetrics)
//  r.Get("/metrics", "ZMon2 Metrics", zmon2.MetricsHandler)
func Handler(h http.Handler, u func(r *http.Request) string) http.Handler {
	return &Metrics{
		handler:  h,
		UrlToKey: u,
	}
}

var mRecords = map[int]string{
	1:  "oneMinute",
	5:  "fiveMinute",
	15: "fifteenMinute",
}

func MetricsHandler(w http.ResponseWriter, r *http.Request, p goat.Params) {
	now := time.Now()
	counts := make(map[string]float64)
	minutes := make(map[int]map[string]float64)
	times := make(map[int]map[string]float64)

	for i := range mRecords {
		minutes[i] = make(map[string]float64)
		times[i] = make(map[string]float64)
	}

	for key := range apiMetrics {
		zres := "zmon.response." + key
		var all []float64
		for _, m := range apiMetrics[key] {
			counts[zres+".count"]++
			for i := range mRecords {
				if m.When.Add(time.Duration(i) * time.Minute).After(now) {
					minutes[i][zres]++
					times[i][zres] += m.Time.Seconds()
				}
			}
			all = append(all, m.Time.Seconds())
		}

		for i := range mRecords {
			if _, ok := minutes[i][zres]; !ok {
				minutes[i][zres] = 0
			}
		}

		sort.Float64s(all)
		nintyFive := int(math.Ceil(float64(len(all)) * 0.95))
		nintyNine := int(math.Ceil(float64(len(all)) * 0.99))
		nintyNineDotNine := int(math.Ceil(float64(len(all)) * 0.999))
		counts[zres+".snapshot.95thPercentile"] = all[nintyFive-1]
		counts[zres+".snapshot.99thPercentile"] = all[nintyNine-1]
		counts[zres+".snapshot.999thPercentile"] = all[nintyNineDotNine-1]
	}

	for i, str := range mRecords {
		for zres := range minutes[i] {
			counts[zres+"."+str+"Rate"] = minutes[i][zres] / 60.0
			if minutes[i][zres] != 0 {
				counts[zres+"."+str+"Response"] = times[i][zres] / minutes[i][zres]
			}
		}
	}

	goat.WriteJSON(w, counts)
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
	key := strconv.Itoa(h.code) + "." + r.Method + "." + m.UrlToKey(r)

	mutex.Lock()
	defer mutex.Unlock()
	apiMetrics[key] = append(apiMetrics[key], apiMetric{When: now, Time: dur})
}

func ExpireMetrics() {
	for {
		time.Sleep(1 * time.Minute)
		go func() {
			now := time.Now()
			am := make(map[string][]apiMetric)

			mutex.Lock()
			defer mutex.Unlock()

			for key := range apiMetrics {
				for _, m := range apiMetrics[key] {
					if m.When.Add(16 * time.Minute).After(now) {
						am[key] = append(am[key], m)
					}
				}
			}
			apiMetrics = am
		}()
	}
}

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
