package main

import (
	"github.com/somakalla1-droid/gke-request-info-service/internal/server"
	"log"
	"net/http"
	"os"
)

func main() {
	port := env("PORT", "8080")
	h := server.New(server.Config{AppName: "gke-request-info-service", Version: env("APP_VERSION", "dev"), ClusterName: env("CLUSTER_NAME", "local"), Region: env("REGION", "local"), PodName: env("POD_NAME", hostname()), ResponseServiceURL: env("RESPONSE_SERVICE_URL", "")})
	log.Printf(`{"severity":"INFO","message":"server starting","port":%q}`, port)
	log.Fatal(http.ListenAndServe(":"+port, h))
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
func hostname() string {
	name, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return name
}
