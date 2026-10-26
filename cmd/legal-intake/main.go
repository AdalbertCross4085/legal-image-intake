package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/infrai-examples/legal-image-intake/internal/intake"
)

const bucket = "legal-intake"

type server struct {
	client *intake.Client
	now    func() time.Time
}

type intakeResult struct {
	MatterID    string `json:"matter_id"`
	ObjectKey   string `json:"object_key"`
	DownloadURL string `json:"download_url"`
	intake.Decision
}

func (s *server) handleIntake(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		http.Error(w, "invalid multipart request", http.StatusBadRequest)
		return
	}
	matterID := strings.TrimSpace(r.FormValue("matter_id"))
	format := strings.ToLower(r.FormValue("format"))
	deadline, err := time.Parse("2006-01-02", r.FormValue("deadline"))
	if err != nil || matterID == "" || (format != "webp" && format != "avif") {
		http.Error(w, "matter_id, deadline, and format=webp|avif are required", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("document")
	if err != nil {
		http.Error(w, "document is required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	input, err := io.ReadAll(io.LimitReader(file, 10<<20))
	if err != nil {
		http.Error(w, "cannot read document", http.StatusBadRequest)
		return
	}
	converted, err := s.client.Convert(r.Context(), input, header.Filename, format)
	if err != nil {
		s.writeError(w, err)
		return
	}
	objectKey := matterID + "/signed-document." + format
	if err := s.client.PutObject(r.Context(), bucket, objectKey, converted); err != nil {
		s.writeError(w, err)
		return
	}
	disposition := fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(objectKey))
	downloadURL, err := s.client.SignedDownload(r.Context(), bucket, objectKey, disposition)
	if err != nil {
		s.writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(intakeResult{MatterID: matterID, ObjectKey: objectKey, DownloadURL: downloadURL, Decision: intake.DecideDeadline(s.now().UTC(), deadline)})
}

func (s *server) writeError(w http.ResponseWriter, err error) {
	var apiErr *intake.InfraiError
	if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
		http.Error(w, apiErr.Error(), apiErr.StatusCode)
		return
	}
	http.Error(w, "upstream request failed", http.StatusBadGateway)
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	client := intake.NewClient(key)
	if err := client.EnsureBucket(context.Background(), bucket); err != nil {
		log.Fatal(err)
	}
	s := &server{client: client, now: time.Now}
	http.HandleFunc("/matters/intake", s.handleIntake)
	log.Println("legal intake listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
