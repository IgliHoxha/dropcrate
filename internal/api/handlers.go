package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/IgliHoxha/dropcrate/internal/files"
	"github.com/IgliHoxha/dropcrate/internal/service"
	"github.com/IgliHoxha/dropcrate/internal/urlsign"
)

// uploadResponse is returned after a successful upload.
type uploadResponse struct {
	files.File
	DownloadURL string `json:"download_url"`
}

func (a *API) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleReady reports readiness: it runs the dependency probe (MySQL, Redis,
// S3) and returns 503 if any is unreachable. Liveness (/healthz) never fails;
// readiness gates traffic in orchestrators.
func (a *API) handleReady(w http.ResponseWriter, r *http.Request) {
	if a.ready != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := a.ready(ctx); err != nil {
			a.log.Warn("readiness check failed", "error", err)
			writeError(w, http.StatusServiceUnavailable, "not ready")
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

// handleUpload accepts a multipart form with a single "file" field and an
// optional "ttl" field (a Go duration string, e.g. "24h"; "0" = never expire).
func (a *API) handleUpload(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, a.maxUploadBytes)

	file, header, err := r.FormFile("file")
	if err != nil {
		// The body exceeded MaxBytesReader's limit before the form could be read.
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeError(w, http.StatusRequestEntityTooLarge, "file exceeds the upload size limit")
			return
		}
		writeError(w, http.StatusBadRequest, "missing 'file' form field")
		return
	}
	defer file.Close()

	ttl, err := parseTTL(r.FormValue("ttl"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid 'ttl': "+err.Error())
		return
	}

	f, err := a.svc.Upload(r.Context(), service.UploadInput{
		Filename:    header.Filename,
		ContentType: header.Header.Get("Content-Type"),
		Size:        header.Size,
		Body:        file,
		TTL:         ttl,
	})
	if err != nil {
		if errors.Is(err, service.ErrTooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "file exceeds the upload size limit")
			return
		}
		a.log.Error("upload failed", "error", err)
		writeError(w, http.StatusInternalServerError, "could not store file")
		return
	}

	writeJSON(w, http.StatusCreated, uploadResponse{
		File:        f,
		DownloadURL: a.downloadURL(f.ID),
	})
}

// downloadURL builds the public download link for id, appending an expiring
// signature when URL signing is enabled.
func (a *API) downloadURL(id string) string {
	u := fmt.Sprintf("%s/v1/files/%s", a.baseURL, id)
	if a.signer != nil && a.signer.Enabled() {
		u += "?" + a.signer.Query(id, time.Now().UTC())
	}
	return u
}

// handleDownload streams a file's bytes to the client.
func (a *API) handleDownload(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if a.signer != nil && a.signer.Enabled() {
		q := r.URL.Query()
		if err := a.signer.Verify(id, q.Get(urlsign.ParamExpires), q.Get(urlsign.ParamSignature), time.Now().UTC()); err != nil {
			if errors.Is(err, urlsign.ErrExpired) {
				writeError(w, http.StatusForbidden, "download link expired")
				return
			}
			writeError(w, http.StatusForbidden, "invalid download link")
			return
		}
	}

	f, obj, err := a.svc.Download(r.Context(), id)
	if err != nil {
		a.respondLookupError(w, err, "download")
		return
	}
	defer obj.Body.Close()

	w.Header().Set("Content-Type", f.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(obj.ContentLength, 10))
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=%q", f.Filename))

	if _, err := io.Copy(w, obj.Body); err != nil {
		a.log.Error("stream failed", "id", id, "error", err)
	}
}

// handleMetadata returns a file's metadata without its bytes.
func (a *API) handleMetadata(w http.ResponseWriter, r *http.Request) {
	f, err := a.svc.Metadata(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		a.respondLookupError(w, err, "metadata")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// handleDelete removes a file.
func (a *API) handleDelete(w http.ResponseWriter, r *http.Request) {
	if err := a.svc.Delete(r.Context(), chi.URLParam(r, "id")); err != nil {
		a.log.Error("delete failed", "error", err)
		writeError(w, http.StatusInternalServerError, "could not delete file")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) respondLookupError(w http.ResponseWriter, err error, op string) {
	if errors.Is(err, files.ErrNotFound) {
		writeError(w, http.StatusNotFound, "file not found")
		return
	}
	a.log.Error(op+" failed", "error", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}

// parseTTL interprets the optional ttl form value. An empty value means "use
// the server default" (returned as 0). "0" or "never" pins the file forever
// (returned as -1).
func parseTTL(raw string) (time.Duration, error) {
	switch raw {
	case "":
		return 0, nil
	case "0", "never":
		return -1, nil
	default:
		return time.ParseDuration(raw)
	}
}
