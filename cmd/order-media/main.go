package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"example.com/checkout-image-service/internal/checkoutmedia"
)

func main() {
	apiKey := os.Getenv("INFRAI_API_KEY")
	if apiKey == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	bucket := os.Getenv("INFRAI_BUCKET")
	if bucket == "" {
		bucket = "checkout-product-images"
	}
	client := checkoutmedia.NewInfraiClient(apiKey)
	setupContext, cancelSetup := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelSetup()
	if err := client.EnsureBucket(setupContext, bucket); err != nil {
		log.Fatalf("create media bucket: %v", err)
	}

	workflow := checkoutmedia.Workflow{Store: client, Bucket: bucket}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders/image", func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		var input checkoutmedia.ConvertRequest
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<20))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		receipt, err := workflow.Process(r.Context(), input)
		if err != nil {
			status := http.StatusBadGateway
			var validationErr *checkoutmedia.ValidationError
			if errors.As(err, &validationErr) {
				status = http.StatusBadRequest
			}
			var apiErr *checkoutmedia.InfraiError
			if errors.As(err, &apiErr) {
				if apiErr.Status >= 400 && apiErr.Status < 500 {
					status = apiErr.Status
				} else {
					status = http.StatusBadGateway
				}
			}
			writeJSON(w, status, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, receipt)
	})

	server := &http.Server{Addr: ":8080", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Printf("order media service listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
