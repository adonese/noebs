package gateway

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/adonese/noebs/ebs_fields"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"
)

func Instrumentation() gin.HandlerFunc {
	counterVec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace:   "noebs",
		Subsystem:   "request",
		Name:        "requests_count",
		Help:        "Number of requests per each endpoint",
		ConstLabels: nil,
	}, []string{"code", "method", "handler", "host", "url"})

	resTime := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace:   "noebs",
		Subsystem:   "response",
		Name:        "response_time_hist",
		Help:        "noebs response duration",
		ConstLabels: nil,
		Buckets:     nil,
	})

	resSize := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace:   "noebs",
		Subsystem:   "response",
		Name:        "size_histogram",
		Help:        "noebs response size",
		ConstLabels: nil,
		Buckets:     nil,
	})

	reqSize := prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace:   "noebs",
		Subsystem:   "request",
		Name:        "size_hist",
		Help:        "Request size instrumenter",
		ConstLabels: nil,
		Buckets:     nil,
	})

	resTimeSum := prometheus.NewSummary(prometheus.SummaryOpts{
		Namespace:   "noebs",
		Subsystem:   "response",
		Name:        "latency_summary",
		Help:        "Computes responses latency",
		ConstLabels: nil,
		Objectives:  nil,
		MaxAge:      0,
		AgeBuckets:  0,
		BufCap:      0,
	})

	// prometheus collector
	colls := []prometheus.Collector{counterVec, resTime, resSize, reqSize, resTimeSum}
	for _, v := range colls {
		err := prometheus.Register(v)
		if err != nil {
			panic(err)
		}
	}
	return func(c *gin.Context) {

		if c.Request.URL.Path == "/metrics" {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()
		duration := float64(time.Since(start)) * 1e-6 // to millisecond

		rSize := c.Writer.Size()
		rqSize := c.Request.ContentLength

		status := strconv.Itoa(c.Writer.Status())
		url := getUrl(c)

		counterVec.WithLabelValues(status, c.Request.Method, c.HandlerName(), c.Request.Host, url).Inc()
		resTime.Observe(duration)
		resSize.Observe(float64(rSize))
		reqSize.Observe(float64(rqSize))
		resTimeSum.Observe(duration)

	}
}

func getUrl(c *gin.Context) string {
	return c.Request.URL.Path
}

type DbBackup struct {
	LastBackup time.Time
	Db         *gorm.DB
}

func (d *DbBackup) RemoteBackup() {
	ticker := time.NewTicker(BACKUP_TIME)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			// call http backup here
			log.Printf("last backup time: %v", d.LastBackup)
			if time.Since(d.LastBackup) > BACKUP_TIME {
				// Select from table user mobile, pubkey and send them across to the server
				var user []ebs_fields.User
				d.Db.Model(&ebs_fields.User{}).Select("mobile, public_key").Find(&user)
				log.Printf("user data: %v", user)
				requestToServer(user)
				d.LastBackup = time.Now()
			}
		}
	}
}

// requestToServer sends the user data to the server endpoint (dapi.noebs.sd) for backup
func requestToServer(user []ebs_fields.User) error {
	client := &http.Client{}
	body, err := json.Marshal(&user)
	if err != nil {
		log.Printf("error in marshaling user data: %v", err)
		return err
	}
	req, err := http.NewRequest("POST", "https://dapi.nil.sd/updates", bytes.NewBuffer(body))
	if err != nil {
		log.Printf("error in creating request: %v", err)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("error in sending request: %v", err)
		return err
	}
	defer resp.Body.Close()
	res, err := io.ReadAll(resp.Body)
	log.Printf("response from server: %v", string(res))
	return nil
}

const (
	BACKUP_TIME = 24 * time.Minute
)
