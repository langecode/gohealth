package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	"gopkg.in/alecthomas/kingpin.v2"
)

var duration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "http_client_duration",
	Help:    "A histogram of durations for http client calls.",
	Buckets: prometheus.ExponentialBuckets(1, 2, 16),
}, []string{"status"})

func main() {
	app := kingpin.New(filepath.Base(os.Args[0]), "Utility to monitor health of http endpoint")
	url := app.Arg("url", "Url to poll").Required().URL()
	headers := app.Flag("header", "Add header to request, e.g., 'api-key: qwerty'").Strings()
	address := app.Flag("listen-address", "Address to listen for metrics requests").Default("0.0.0.0:9090").String()
	data := app.Flag("data", "Query parameter templated, e.g. 'startTime={{ .CurrentTime }}'").Strings()
	interval := app.Flag("interval", "The interval (seconds) to check health of endpoint").Default("60").Int()
	kingpin.MustParse(app.Parse(os.Args[1:]))

	funcMap := template.FuncMap{
		"format":      TimeFormat,
		"addDuration": AddDuration,
	}

	params := make([]*template.Template, 0)
	for i, d := range *data {
		t, err := template.New(fmt.Sprintf("data-%d", i)).Funcs(funcMap).Parse(d)
		if err != nil {
			app.FatalUsage("Unable to parse given data option: %s, error: %v", d, err)
		}
		params = append(params, t)
	}

	go func() {
		log.Infof("Serving metrics on %s", *address)
		http.Handle("/metrics", promhttp.Handler())
		if err := http.ListenAndServe(*address, nil); err != nil {
			log.Fatalf("Unable to start prometheus metrics endpoint: %v", err)
		}
	}()

	log.Infof("Starting poll of url: %v", *url)

	c := time.Tick(time.Duration(*interval) * time.Second)
	client := &http.Client{}
	go func() {
		for range c {
			u := **url
			qp := make([]string, 0)
			for _, p := range params {
				buf := new(bytes.Buffer)
				v := Values{
					CurrentTime: time.Now().UTC(),
				}
				p.Execute(buf, v)
				qp = append(qp, buf.String())
			}

			query := strings.Join(qp, "&")
			if len(u.RawQuery) > 0 {
				u.RawQuery = fmt.Sprintf("%s&%s", u.RawQuery, query)
			} else {
				u.RawQuery = query
			}

			req, err := http.NewRequest("GET", u.String(), nil)
			if err != nil {
				log.Fatalf("Unable to create http request - exiting: %v", err)
			}

			for _, h := range *headers {
				hv := strings.Split(h, ":")
				if len(hv) != 2 {
					log.Fatalf("Unable to parse given header: %v", h)
				}
				req.Header.Add(strings.TrimSpace(hv[0]), strings.TrimSpace(hv[1]))
			}

			start := time.Now()
			resp, err := client.Do(req)
			d := time.Since(start)
			if err != nil {
				log.Infof("Unable error from querying endpoint: %v", err)
			}
			status := -1
			if resp != nil {
				status = resp.StatusCode
			}
			duration.WithLabelValues(strconv.FormatInt(int64(status), 10)).Observe(float64(d.Milliseconds()))
		}
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	log.Info("Stopping health check")
}

// TimeFormat returns the formatted time
func TimeFormat(t time.Time) string {
	return t.Format("20060102T1504Z") // 20200318T1000Z
}

// AddDuration add the given hours to the give time
func AddDuration(t time.Time, hours int) time.Time {
	return t.Add(time.Duration(hours) * time.Hour)
}

// Values used for parameter templates
type Values struct {
	CurrentTime time.Time
}
