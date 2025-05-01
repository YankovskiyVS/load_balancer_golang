package servers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func handler(ip string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Response from server: %s\n", ip)
	}
}

func main() {
	// make a list of the servers ips
	ips := []string{"127.0.0.1", "127.0.0.2", "127.0.0.3"}
	port := ":8080"

	servers := make([]*http.Server, len(ips))

	// start the servers by goroutines
	for i, ip := range ips {
		addr := fmt.Sprintf("%s:%s", ip, port)
		servers[i] = &http.Server{
			Addr:    addr,
			Handler: handler(ip),
		}
		// start goroutine for the servers start
		go func(server *http.Server) {
			log.Printf("Starting the server on: %s\n", server.Addr)
			if err := server.ListenAndServe(); err != nil {
				log.Printf("Server %s is failed to start: %v", server.Addr, err)
			}
		}(servers[i])
	}

	// Shutdown the servers gracefully
	// Wait for the interupt signal
	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-done
	log.Println("Shutting  down all servers...")

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond*5000)
	defer cancel()

	for _, server := range servers {
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("Error ocured dureing server %s shutting down: %v", server.Addr, err)
		}
		log.Printf("Gracefully stopped the server on %s address", server.Addr)
	}
}
