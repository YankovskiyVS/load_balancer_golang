package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type BackendServer struct {
	addr  string
	alive bool
	mu    sync.RWMutex
}

func (b *BackendServer) IsAlive() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.alive
}

type LoadBalancer struct {
	Servers  []*BackendServer
	current  atomic.Uint32
	config   Config
	stopChan chan struct{}
}

type Config struct {
	BindAddress         string        `yaml:"bind_address"`
	Backends            []string      `yaml:"backends"`
	HealthCheckInterval time.Duration `yaml:"health_check_interval"`
	DialTimeout         time.Duration `yaml:"dial_timeout"`
}

func NewLoadBalancer(cfg Config) *LoadBalancer {
	servers := make([]*BackendServer, len(cfg.Backends))
	for i, addr := range cfg.Backends {
		servers[i] = &BackendServer{
			addr:  addr,
			alive: true,
		}
	}
	return &LoadBalancer{
		Servers:  servers,
		config:   cfg,
		stopChan: make(chan struct{}),
	}
}

func (b *BackendServer) healthCheck(timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", b.addr, timeout)
	if err != nil {
		b.mu.Lock()
		b.alive = false
		b.mu.Unlock()
		return false
	}
	conn.Close()
	b.mu.Lock()
	b.alive = true
	b.mu.Unlock()
	return true
}

func (lb *LoadBalancer) StartHealthCheck() {
	ticker := time.NewTicker(lb.config.HealthCheckInterval)
	go func() {
		for {
			select {
			case <-ticker.C:
				lb.runHealthChecks()
			case <-lb.stopChan:
				ticker.Stop()
				return
			}
		}
	}()
}

func (lb *LoadBalancer) runHealthChecks() {
	var wg sync.WaitGroup
	for _, backend := range lb.Servers {
		wg.Add(1)
		go func(b *BackendServer) {
			defer wg.Done()
			b.healthCheck(lb.config.DialTimeout)
		}(backend)
	}
	wg.Wait()
}

func (lb *LoadBalancer) nextServer() (*BackendServer, error) {
	num := uint32(len(lb.Servers))
	if num == 0 {
		return nil, errors.New("no configured servers")
	}

	start := lb.current.Load()
	for i := uint32(0); i < num; i++ {
		idx := (start + i) % num
		if lb.Servers[idx].IsAlive() {
			lb.current.Store(idx + 1)
			return lb.Servers[idx], nil
		}
	}
	return nil, errors.New("no healthy servers available")
}

func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	server, err := lb.nextServer()
	if err != nil {
		http.Error(w, "Service unavailable", http.StatusServiceUnavailable)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(&url.URL{
		Scheme: "http",
		Host:   server.addr,
	})

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("Proxy error occurred: %v", err)
		server.mu.Lock()
		server.alive = false
		server.mu.Unlock()
		http.Error(w, "Bad gateway", http.StatusBadGateway)
	}

	proxy.ServeHTTP(w, r)
}

func main() {
	cfg := Config{
		BindAddress:         ":8080",
		Backends:            []string{"127.0.0.1:8081", "127.0.0.2:8081", "127.0.0.3:8081"},
		HealthCheckInterval: 5 * time.Second,
		DialTimeout:         2 * time.Second,
	}

	lb := NewLoadBalancer(cfg)
	lb.StartHealthCheck()
	defer close(lb.stopChan)

	server := &http.Server{
		Addr:    cfg.BindAddress,
		Handler: lb,
	}

	go func() {
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Load balancer error: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}
	log.Println("Graceful shutdown complete")
}
