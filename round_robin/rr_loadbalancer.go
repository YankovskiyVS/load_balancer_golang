package roundrobin

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type ApiServer struct {
	router string
	alive  bool
}

type ApiServerList struct {
	Servers []ApiServer
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
