package roundrobin

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

type ApiServer struct {
	addr  string
	alive bool
	mu    sync.RWMutex
}

type ApiServerList struct {
	Servers []*ApiServer
	current atomic.Uint32
	config  Config
	mu      sync.RWMutex
}

type Config struct {
	Port         string        `yaml:"port"`
	Backends     []string      `yaml:"backends"`
	health_check time.Duration `yaml:"health_check_interval"`
}

func NewApiServerList(cfg Config) *ApiServerList {
	servers := make([]*ApiServer, len(cfg.Backends))
	for i, addr := range cfg.Backends {
		servers[i] = &ApiServer{
			addr:  addr,
			alive: true,
		}
	}
	return &ApiServerList{
		Servers: servers,
		config:  cfg,
	}
}

func (server *ApiServer) healthCheck(timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", server.addr, timeout)
	if err != nil {
		server.mu.Lock()
		server.alive = false
		server.mu.Unlock()
		return false
	}
	conn.Close()
	server.mu.Lock()
	server.alive = true
	server.mu.Unlock()
	return true
}

func (slist *ApiServerList) nextServer() (*ApiServer, error) {
	// define the number of current Servers
	// Using RWMutex for the possible data race
	slist.mu.RLock()
	numOfServers := len(slist.Servers)
	if numOfServers == 0 {
		return nil, errors.New("error: there are no available servers")
	}
	defer slist.mu.RUnlock()

	for i := 0; i < numOfServers; i++ {
		idx := slist.current.Add(1) % uint32(len(slist.Servers))
		server := slist.Servers[idx]

		server.mu.RLock()
		alive := server.alive
		server.mu.RUnlock()

		if alive {
			return server, nil
		}
	}
	return nil, nil
}

func loadBalancing() {
	// add standard rr load balancer logic
}

func main() {
	loadBalancer := &http.Server{
		Addr: ":8080",
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {})

	go func() {
		if err := loadBalancer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("Load balancer error: %v", err)
		}
		log.Println("Load balancer is stopped")
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()

	if err := loadBalancer.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Load balancer shutdown error: %v", err)
	}
	log.Println("Graceful shutdown complete")
}
