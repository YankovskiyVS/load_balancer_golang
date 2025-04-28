package roundrobin

import "net/http"

type ApiServer struct {
	router string
	alive  bool
}

type ApiServerList struct {
	Servers []ApiServer
}

func main() {
	loadBalancerPort := ":8080"
	http.ListenAndServe(loadBalancerPort, nil)
}
