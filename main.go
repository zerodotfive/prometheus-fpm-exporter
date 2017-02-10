// Usage: http://127.0.0.1:9030/fpm_metrics?pools=[{"protocol": "tcp", "address": "127.0.0.1:9001", "location": "/status"}]

package main

import (
    "net/http"
    "encoding/json"
    "log"
    "bytes"
    "io/ioutil"
    "github.com/tomasen/fcgi_client"
    "github.com/prometheus/client_golang/prometheus"
    "flag"
    "github.com/prometheus/client_golang/prometheus/promhttp"
    "sync"
)

const (
    namespace = "fpm"
)

type FpmConnection struct {
    Protocol            string
    Address             string
    Location            string

    Exporters           *PoolExporter
}

type FpmPool struct {
    Pool                string  `json:"pool"`
    ProcessManager      string  `json:"process manager"`
    StartTime           int     `json:"start time"`
    StartSince          int     `json:"start since"`
    AcceptedConn        int     `json:"accepted conn"`
    ListenQueue         int     `json:"listen queue"`
    MaxListenQueue      int     `json:"max listen queue"`
    ListenQueueLen      int     `json:"listen queue len"`
    IdleProcesses       int     `json:"idle processes"`
    ActiveProcesses     int     `json:"active processes"`
    TotalProcesses      int     `json:"total processes"`
    MaxActiveProcesses  int     `json:"max active processes"`
    MaxChildrenReached  int     `json:"max children reached"`
    SlowRequests        int     `json:"slow requests"`
}

type PoolExporter struct {
    pool                 *FpmConnection
    mutex                sync.RWMutex

    start_time           prometheus.Gauge
    start_since          prometheus.Gauge
    accepted_conn        prometheus.Gauge
    listen_queue         prometheus.Gauge
    max_listen_queue     prometheus.Gauge
    listen_queue_len     prometheus.Gauge
    idle_processes       prometheus.Gauge
    active_processes     prometheus.Gauge
    total_processes      prometheus.Gauge
    max_active_processes prometheus.Gauge
    max_children_reached prometheus.Gauge
    slow_requests        prometheus.Gauge
}

func (self *PoolExporter) getStatus() ([]byte, error) {
    env := make(map[string]string)
    env["REQUEST_METHOD"] = "GET"
    env["SCRIPT_NAME"] = self.pool.Location;
    env["REMOTE_ADDR"] = "127.0.0.1";
    env["QUERY_STRING"] = "json";
    var bufferScriptFilename bytes.Buffer
    bufferScriptFilename.WriteString("/var/www/nonexistent")
    bufferScriptFilename.WriteString(self.pool.Location)
    env["SCRIPT_FILENAME"] = bufferScriptFilename.String();

    fcgi, err := fcgiclient.Dial(self.pool.Protocol, self.pool.Address)
    if err != nil {
        return nil, err
    }

    resp, err := fcgi.Get(env)
    if err != nil {
        fcgi.Close()
        return nil, err
    }
    
    content, err := ioutil.ReadAll(resp.Body)
    fcgi.Close()

    if err != nil {
        return nil, err
    }

    return content, nil
}

func CreateExporters(pool FpmConnection) (*PoolExporter, error) {
    return &PoolExporter{
        pool:                   &pool,
        start_time:             prometheus.NewGauge(prometheus.GaugeOpts{
                Namespace:          namespace,
                Name:               "start_time",
                Help:               "Pool start time",
                ConstLabels:        prometheus.Labels{"pool": pool.Address},
            }),
        start_since:            prometheus.NewGauge(prometheus.GaugeOpts{
                Namespace:          namespace,
                Name:               "start_since",
                Help:               "Time since pool start",
                ConstLabels:        prometheus.Labels{"pool": pool.Address},
            }),
        accepted_conn:          prometheus.NewGauge(prometheus.GaugeOpts{
                Namespace:          namespace,
                Name:               "accepted_conn",
                Help:               "Num accepted connections",
                ConstLabels:        prometheus.Labels{"pool": pool.Address},
            }),
        listen_queue:           prometheus.NewGauge(prometheus.GaugeOpts{
                Namespace:          namespace,
                Name:               "listen_queue",
                Help:               "Listen queue",
                ConstLabels:        prometheus.Labels{"pool": pool.Address},
            }),
        max_listen_queue:       prometheus.NewGauge(prometheus.GaugeOpts{
                Namespace:          namespace,
                Name:               "max_listen_queue",
                Help:               "Max listen queue",
                ConstLabels:        prometheus.Labels{"pool": pool.Address},
            }),
        listen_queue_len:       prometheus.NewGauge(prometheus.GaugeOpts{
                Namespace:          namespace,
                Name:               "listen_queue_len",
                Help:               "Listen Queue Len",
                ConstLabels:        prometheus.Labels{"pool": pool.Address},
            }),
        idle_processes:         prometheus.NewGauge(prometheus.GaugeOpts{
                Namespace:          namespace,
                Name:               "idle_processes",
                Help:               "Idle Processes",
                ConstLabels:        prometheus.Labels{"pool": pool.Address},
            }),
        active_processes:       prometheus.NewGauge(prometheus.GaugeOpts{
                Namespace:          namespace,
                Name:               "active_processes",
                Help:               "Active Processes",
                ConstLabels:        prometheus.Labels{"pool": pool.Address},
            }),
        total_processes:        prometheus.NewGauge(prometheus.GaugeOpts{
                Namespace:          namespace,
                Name:               "total_processes",
                Help:               "Total Processes",
                ConstLabels:        prometheus.Labels{"pool": pool.Address},
            }),
        max_active_processes:   prometheus.NewGauge(prometheus.GaugeOpts{
                Namespace:          namespace,
                Name:               "max_active_processes",
                Help:               "Max Active Processes",
                ConstLabels:        prometheus.Labels{"pool": pool.Address},
            }),
        max_children_reached:   prometheus.NewGauge(prometheus.GaugeOpts{
                Namespace:          namespace,
                Name:               "max_children_reached",
                Help:               "Max Children Reached Times ",
                ConstLabels:        prometheus.Labels{"pool": pool.Address},
            }),
        slow_requests:          prometheus.NewGauge(prometheus.GaugeOpts{
                Namespace:          namespace,
                Name:               "slow_requests",
                Help:               "Slow Requests Count",
                ConstLabels:        prometheus.Labels{"pool": pool.Address},
            }),
    }, nil
}

func (self *PoolExporter) Describe(ch chan<- *prometheus.Desc) {
    ch <- self.accepted_conn.Desc()
    ch <- self.active_processes.Desc()
    ch <- self.idle_processes.Desc()
    ch <- self.listen_queue.Desc()
    ch <- self.listen_queue_len.Desc()
    ch <- self.max_active_processes.Desc()
    ch <- self.max_children_reached.Desc()
    ch <- self.max_listen_queue.Desc()
    ch <- self.slow_requests.Desc()
    ch <- self.start_since.Desc()
    ch <- self.start_time.Desc()
    ch <- self.total_processes.Desc()
}

func (self *PoolExporter) Collect(ch chan<- prometheus.Metric) {
    self.mutex.Lock()
    defer self.mutex.Unlock()

    var pool FpmPool
    var content, err = self.getStatus()
    if err != nil {
        log.Print("Error getting pool from fpm: ", err)
    } else {
        err := json.Unmarshal(content, &pool)
        if err != nil {
            log.Print("Error getting pool from fpm: ", err)
        }
    }

    self.accepted_conn.Set(float64(pool.AcceptedConn))
    self.active_processes.Set(float64(pool.ActiveProcesses))
    self.idle_processes.Set(float64(pool.IdleProcesses))
    self.listen_queue.Set(float64(pool.ListenQueue))
    self.listen_queue_len.Set(float64(pool.ListenQueueLen))
    self.max_active_processes.Set(float64(pool.MaxActiveProcesses))
    self.max_children_reached.Set(float64(pool.MaxChildrenReached))
    self.max_listen_queue.Set(float64(pool.MaxListenQueue))
    self.slow_requests.Set(float64(pool.SlowRequests))
    self.start_since.Set(float64(pool.StartSince))
    self.start_time.Set(float64(pool.StartTime))
    self.total_processes.Set(float64(pool.TotalProcesses))

    ch <- self.accepted_conn
    ch <- self.active_processes
    ch <- self.idle_processes
    ch <- self.listen_queue
    ch <- self.listen_queue_len
    ch <- self.max_active_processes
    ch <- self.max_children_reached
    ch <- self.max_listen_queue
    ch <- self.slow_requests
    ch <- self.start_since
    ch <- self.start_time
    ch <- self.total_processes
}

func main() {
    var (
        listenAddress             = flag.String("listen", "0.0.0.0:9030", "Address to listen on for web interface and telemetry.")
        metricsPath               = flag.String("metrics-path", "/metrics", "Path under which to expose metrics.")
        configFile                = flag.String("config", "/etc/prometheus-fpm-exporter.json", "Pools list config")
   )
    flag.Parse()

    var pools = make([]FpmConnection, 256)
    config, err := ioutil.ReadFile(*configFile)
    if err != nil {
        log.Fatal("Couldn't read config: ", err)
    }
    err = json.Unmarshal(config, &pools)
    if err != nil {
        log.Fatal("Couldn't parse config: ", err)
    }

    for pool := range pools {
        exporter, err := CreateExporters(pools[pool])
        if err != nil {
            log.Fatal(err)
        }

        prometheus.MustRegister(exporter)
    }

    http.Handle(*metricsPath, promhttp.Handler())
    err = http.ListenAndServe(*listenAddress, nil)
    if err != nil {
        log.Fatal("ListenAndServe: ", err)
    }
}
