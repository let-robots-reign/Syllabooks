package handlers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"syllabooks/db/gen"
)

const (
	maxCoverBytes  = 8 << 20
	coverURLPrefix = "/api/book-covers/"
)

var coverTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

func (s *Server) bookCover(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")
	if s.BookCoversDir == "" || filename == "" || filename != filepath.Base(filename) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, filepath.Join(s.BookCoversDir, filename))
}

func (s *Server) uploadBookCover(w http.ResponseWriter, r *http.Request, _ gen.User) {
	id, ok := bookID(w, r)
	if !ok {
		return
	}
	current, err := gen.New(s.Pool).GetBook(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, fmt.Errorf("get book before cover upload: %w", err))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxCoverBytes+(1<<20))
	if err := r.ParseMultipartForm(maxCoverBytes); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "Обложка должна быть не больше 8 МБ.")
		return
	}
	file, _, err := r.FormFile("cover")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Выбери файл обложки.")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxCoverBytes+1))
	if err != nil {
		serverError(w, r, fmt.Errorf("read uploaded cover: %w", err))
		return
	}
	if len(data) > maxCoverBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "Обложка должна быть не больше 8 МБ.")
		return
	}
	contentType := http.DetectContentType(data)
	if _, ok := coverTypes[contentType]; !ok {
		writeError(w, http.StatusUnsupportedMediaType, "Загрузи обложку в формате JPEG, PNG или WebP.")
		return
	}
	coverURL, err := s.storeCover(data, contentType)
	if err != nil {
		serverError(w, r, fmt.Errorf("store uploaded cover: %w", err))
		return
	}
	updated, err := gen.New(s.Pool).UpdateBookCover(r.Context(), gen.UpdateBookCoverParams{
		CoverUrl: &coverURL,
		ID:       id,
	})
	if err != nil {
		s.removeStoredCover(&coverURL)
		serverError(w, r, fmt.Errorf("attach uploaded cover: %w", err))
		return
	}
	s.removeStoredCover(current.CoverUrl)
	writeJSON(w, http.StatusOK, presentBook(updated))
}

func (s *Server) deleteBookCover(w http.ResponseWriter, r *http.Request, _ gen.User) {
	id, ok := bookID(w, r)
	if !ok {
		return
	}
	current, err := gen.New(s.Pool).GetBook(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, fmt.Errorf("get book before cover removal: %w", err))
		return
	}
	updated, err := gen.New(s.Pool).UpdateBookCover(r.Context(), gen.UpdateBookCoverParams{ID: id})
	if err != nil {
		serverError(w, r, fmt.Errorf("remove book cover: %w", err))
		return
	}
	s.removeStoredCover(current.CoverUrl)
	writeJSON(w, http.StatusOK, presentBook(updated))
}

func (s *Server) downloadAndStoreCover(ctx context.Context, target string) (string, error) {
	data, contentType, err := s.downloadCover(ctx, target)
	if err != nil {
		return "", err
	}
	return s.storeCover(data, contentType)
}

func (s *Server) downloadCover(ctx context.Context, target string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Syllabooks/1.0 (local book cover storage)")
	client := s.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil, "", errMetadataNotFound
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, "", fmt.Errorf("cover provider returned %s", response.Status)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxCoverBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxCoverBytes {
		return nil, "", errors.New("cover exceeds 8 MiB")
	}
	contentType := http.DetectContentType(data)
	if _, ok := coverTypes[contentType]; !ok {
		return nil, "", fmt.Errorf("unsupported cover content type %q", contentType)
	}
	return data, contentType, nil
}

func (s *Server) storeCover(data []byte, contentType string) (string, error) {
	extension, ok := coverTypes[contentType]
	if !ok {
		return "", errors.New("unsupported cover type")
	}
	if s.BookCoversDir == "" {
		return "", errors.New("BOOK_COVERS_DIR is not configured")
	}
	if err := os.MkdirAll(s.BookCoversDir, 0o750); err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(s.BookCoversDir, ".cover-*")
	if err != nil {
		return "", err
	}
	temporaryName := temporary.Name()
	keep := false
	defer func() {
		temporary.Close()
		if !keep {
			os.Remove(temporaryName)
		}
	}()
	if err := temporary.Chmod(0o644); err != nil {
		return "", err
	}
	if _, err := temporary.Write(data); err != nil {
		return "", err
	}
	if err := temporary.Sync(); err != nil {
		return "", err
	}
	if err := temporary.Close(); err != nil {
		return "", err
	}
	filename := uuid.NewString() + extension
	if err := os.Rename(temporaryName, filepath.Join(s.BookCoversDir, filename)); err != nil {
		return "", err
	}
	keep = true
	return coverURLPrefix + filename, nil
}

func (s *Server) removeStoredCover(coverURL *string) {
	if coverURL == nil || s.BookCoversDir == "" || !strings.HasPrefix(*coverURL, coverURLPrefix) {
		return
	}
	filename := strings.TrimPrefix(*coverURL, coverURLPrefix)
	if filename == "" || filename != filepath.Base(filename) {
		return
	}
	if err := os.Remove(filepath.Join(s.BookCoversDir, filename)); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("remove cover %s: %v", filename, err)
	}
}
