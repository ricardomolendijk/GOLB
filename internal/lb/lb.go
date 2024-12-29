package lb

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ricardomolendijk/GOLB/pkg/l"
)

type Backend struct {
	URL          string        `json:"URL"`
	Active       atomic.Bool   `json:"-"`
	ActiveStatus bool          `json:"Active"`
	Latency      time.Duration `json:"-"`
	LatencyStr   string        `json:"Latency"`
	RequestCount int64         `json:"RequestCount"`
	Weight       int           `json:"Weight"`
}

var (
	backends               []*Backend
	requestCounter         map[string]uint64
	mu                     sync.RWMutex
	clientLatency          map[string]time.Duration
	clientPreferredBackend map[string]*Backend
	sessionTimeout         time.Duration
	sessionExpiration      map[string]time.Time
)

func init() {
	requestCounter = make(map[string]uint64)
	clientLatency = make(map[string]time.Duration)
	clientPreferredBackend = make(map[string]*Backend)
	sessionExpiration = make(map[string]time.Time)
	sessionTimeout = 5 * time.Minute // Cache user session data for 5 minutes
}

func loadBackends(file string) ([]*Backend, error) {
	fileData, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("could not read backends file: %v", err)
	}

	var loadedBackends []Backend
	err = json.Unmarshal(fileData, &loadedBackends)
	if err != nil {
		return nil, fmt.Errorf("could not unmarshal backends data: %v", err)
	}
	backends := make([]*Backend, len(loadedBackends))
	for i := range loadedBackends {
		b := &loadedBackends[i]
		// Parse latency string to time.Duration
		b.Latency, err = time.ParseDuration(b.LatencyStr)
		if err != nil {
			return nil, fmt.Errorf("invalid latency format for backend %s: %v", b.URL, err)
		}
		b.Active.Store(b.ActiveStatus)
		backends[i] = b
	}
	return backends, nil
}

func healthCheck(interval time.Duration, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		var wgHealth sync.WaitGroup
		for _, backend := range backends {
			wgHealth.Add(1)
			go func(b *Backend) {
				defer wgHealth.Done()
				client := http.Client{Timeout: 5 * time.Second}
				start := time.Now()
				resp, err := client.Get(b.URL + "/health")
				b.Latency = time.Since(start)
				if err != nil {
					if b.Active.Load() {
						b.Active.Store(false)
						l.Warn("[Health Check] Backend has gone DOWN", "url", b.URL, "reason", err.Error())
					}
				} else if resp.StatusCode != http.StatusOK {
					if b.Active.Load() {
						b.Active.Store(false)
						l.Warn("[Health Check] Backend has gone DOWN", "url", b.URL, "reason", fmt.Sprintf("non-200 response: %d", resp.StatusCode))
					}
				} else {
					if !b.Active.Load() {
						b.Active.Store(true)
						l.Info("[Health Check] Backend is back UP", "url", b.URL)
					}
					resp.Body.Close()
				}
			}(backend)
		}
		wgHealth.Wait()
		time.Sleep(interval)
	}
}

func getNextBackend(clientIP string) *Backend {
	var selectedBackend *Backend
	var minLatency time.Duration = time.Hour

	mu.RLock()
	if preferredBackend, exists := clientPreferredBackend[clientIP]; exists && preferredBackend.Active.Load() {
		if time.Now().Before(sessionExpiration[clientIP]) {
			mu.RUnlock()
			return preferredBackend
		}
	}
	mu.RUnlock()

	var totalWeight int
	mu.RLock()
	for _, backend := range backends {
		if backend.Active.Load() {
			totalWeight += backend.Weight
		}
	}
	mu.RUnlock()

	mu.RLock()
	for _, backend := range backends {
		if backend.Active.Load() {
			weightFactor := float64(backend.Weight) / float64(totalWeight)
			adjustedLatency := time.Duration(float64(backend.Latency) * weightFactor)
			if selectedBackend == nil || adjustedLatency < minLatency {
				selectedBackend = backend
				minLatency = adjustedLatency
			}
		}
	}
	mu.RUnlock()

	if selectedBackend != nil {
		mu.Lock()
		if currentBackend, exists := clientPreferredBackend[clientIP]; !exists || currentBackend != selectedBackend {
			oldBackendURL := "none"
			if exists {
				oldBackendURL = currentBackend.URL
			}
			l.Info("User changed backend", "clientIP", clientIP, "from", oldBackendURL, "to", selectedBackend.URL)
		}
		clientPreferredBackend[clientIP] = selectedBackend
		sessionExpiration[clientIP] = time.Now().Add(sessionTimeout)
		mu.Unlock()
	}

	return selectedBackend
}

func handler(w http.ResponseWriter, r *http.Request) {
	clientIP, _, _ := net.SplitHostPort(r.RemoteAddr)
	backend := getNextBackend(clientIP)
	if backend == nil {
		l.Error("No active backends available!")
		http.Error(w, "All servers are currently unavailable. Please try again later.", http.StatusServiceUnavailable)
		return
	}

	l.Info("[Request] Forwarding request", "from", r.RemoteAddr, "to", backend.URL)
	proxyURL, _ := url.Parse(backend.URL)
	if !strings.HasPrefix(proxyURL.Scheme, "http") {
		proxyURL.Scheme = "https"
	}
	proxy := httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = proxyURL.Scheme
			req.URL.Host = proxyURL.Host
			req.Header.Set("X-Forwarded-For", r.RemoteAddr)
		},
	}
	proxy.ServeHTTP(w, r)

	go checkClientLatency(clientIP)
}

func checkClientLatency(clientIP string) {
	for _, backend := range backends {
		if backend.Active.Load() {
			client := http.Client{Timeout: 5 * time.Second}
			start := time.Now()
			_, err := client.Get(backend.URL + "/health")
			latency := time.Since(start)
			if err == nil {
				mu.Lock()
				clientLatency[clientIP] = latency
				mu.Unlock()

				mu.RLock()
				if clientPreferredBackend[clientIP] == nil || latency < clientPreferredBackend[clientIP].Latency {
					mu.RUnlock()
					mu.Lock()
					clientPreferredBackend[clientIP] = backend
					mu.Unlock()
				} else {
					mu.RUnlock()
				}
			}
		}
	}
}

func gracefulShutdown(server *http.Server) {
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan

	l.Info("Shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := server.Shutdown(ctx)
	if err != nil {
		l.Fatal("Server shutdown failed", "error", err)
	}
	l.Info("Server stopped successfully")
}

func NewLB(listen string) {
	var err error
	backends, err = loadBackends("backends.json")
	if err != nil {
		l.Fatal("Error loading backends", "error", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go healthCheck(10*time.Second, &wg)

	go func() {
		for {
			time.Sleep(1 * time.Hour)
			mu.Lock()
			for k := range requestCounter {
				requestCounter[k] = 0
			}
			mu.Unlock()
		}
	}()

	certFile := "certs/live/golb.ricardomolendijk.com/cert.pem"
	keyFile := "certs/live/golb.ricardomolendijk.com/key.pem"
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		l.Fatal("Failed to load certificates", "error", err)
	}
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	server := &http.Server{
		Addr:      listen,
		Handler:   http.HandlerFunc(handler),
		TLSConfig: tlsConfig,
	}

	go gracefulShutdown(server)

	l.Info("Load balancer running", "port", listen)
	if err := server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
		l.Fatal("Server failed", "error", err)
	}

	wg.Wait()
}
