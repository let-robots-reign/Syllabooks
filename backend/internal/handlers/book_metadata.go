package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

const maxMetadataBytes = 1 << 20

var (
	errMetadataNotFound = errors.New("book metadata not found")
	htmlTagPattern      = regexp.MustCompile(`(?s)<[^>]*>`)
)

type metadataProviderError struct {
	Provider string
	Err      error
}

func (e *metadataProviderError) Error() string {
	return e.Provider + ": " + e.Err.Error()
}

func (e *metadataProviderError) Unwrap() error {
	return e.Err
}

type metadataHTTPError struct {
	StatusCode int
}

func (e *metadataHTTPError) Error() string {
	return fmt.Sprintf("metadata provider returned HTTP %d", e.StatusCode)
}

type metadataDecodeError struct {
	Err error
}

func (e *metadataDecodeError) Error() string {
	return "decode metadata: " + e.Err.Error()
}

func (e *metadataDecodeError) Unwrap() error {
	return e.Err
}

type bookMetadata struct {
	Title       string
	Author      string
	PageCount   int32
	Description string
	CoverURL    string
	Source      string
}

func (s *Server) lookupMetadata(ctx context.Context, isbn string) (bookMetadata, bool, error) {
	openLibrary, openFound, openErr := s.lookupOpenLibrary(ctx, isbn)
	result := openLibrary
	found := openFound

	missingFields := missingMetadataFields(result)
	needsGoogle := !openFound || len(missingFields) > 0
	var googleErr error
	if needsGoogle {
		switch {
		case openErr != nil:
			log.Printf("Open Library metadata lookup for ISBN %s failed: %v; trying Google Books", isbn, openErr)
		case !openFound:
			log.Printf("Open Library metadata lookup for ISBN %s: book not found; trying Google Books", isbn)
		default:
			log.Printf("Open Library metadata lookup for ISBN %s returned incomplete data (missing: %s); trying Google Books", isbn, strings.Join(missingFields, ", "))
		}
		google, googleFound, err := s.lookupGoogleBooks(ctx, isbn)
		googleErr = err
		if googleFound {
			if !found {
				result = google
				found = true
			} else {
				mergeMetadata(&result, google)
				result.Source = "openlibrary+google"
			}
		}
	}

	if found {
		result.Description = shortenDescription(result.Description)
		return result, true, nil
	}
	if openErr != nil || googleErr != nil {
		return bookMetadata{}, false, errors.Join(openErr, googleErr)
	}
	return bookMetadata{}, false, nil
}

func missingMetadataFields(metadata bookMetadata) []string {
	missing := make([]string, 0, 5)
	if metadata.Title == "" {
		missing = append(missing, "title")
	}
	if metadata.Author == "" {
		missing = append(missing, "author")
	}
	if metadata.PageCount == 0 {
		missing = append(missing, "page_count")
	}
	if metadata.Description == "" {
		missing = append(missing, "description")
	}
	if metadata.CoverURL == "" {
		missing = append(missing, "cover")
	}
	return missing
}

func mergeMetadata(dst *bookMetadata, fallback bookMetadata) {
	if dst.Title == "" {
		dst.Title = fallback.Title
	}
	if dst.Author == "" {
		dst.Author = fallback.Author
	}
	if dst.PageCount == 0 {
		dst.PageCount = fallback.PageCount
	}
	if dst.Description == "" {
		dst.Description = fallback.Description
	}
	if dst.CoverURL == "" {
		dst.CoverURL = fallback.CoverURL
	}
}

func (s *Server) lookupOpenLibrary(ctx context.Context, isbn string) (bookMetadata, bool, error) {
	base := strings.TrimRight(s.OpenLibraryBaseURL, "/")
	if base == "" {
		base = "https://openlibrary.org"
	}
	query := url.Values{
		"bibkeys": {"ISBN:" + isbn},
		"jscmd":   {"data"},
		"format":  {"json"},
	}
	var response map[string]struct {
		Title         string `json:"title"`
		NumberOfPages int32  `json:"number_of_pages"`
		Authors       []struct {
			Name string `json:"name"`
		} `json:"authors"`
		Cover struct {
			Small  string `json:"small"`
			Medium string `json:"medium"`
			Large  string `json:"large"`
		} `json:"cover"`
	}
	if err := s.getJSON(ctx, base+"/api/books?"+query.Encode(), &response); err != nil {
		if errors.Is(err, errMetadataNotFound) {
			return bookMetadata{}, false, nil
		}
		return bookMetadata{}, false, &metadataProviderError{Provider: "Open Library", Err: err}
	}
	entry, ok := response["ISBN:"+isbn]
	if !ok {
		return bookMetadata{}, false, nil
	}
	authors := make([]string, 0, len(entry.Authors))
	for _, author := range entry.Authors {
		if name := strings.TrimSpace(author.Name); name != "" {
			authors = append(authors, name)
		}
	}
	metadata := bookMetadata{
		Title:     strings.TrimSpace(entry.Title),
		Author:    strings.Join(authors, ", "),
		PageCount: entry.NumberOfPages,
		CoverURL:  firstNonEmpty(entry.Cover.Large, entry.Cover.Medium, entry.Cover.Small),
		Source:    "openlibrary",
	}
	metadata.CoverURL = strings.Replace(metadata.CoverURL, "http://", "https://", 1)
	// Descriptions usually live on the Edition or its linked Work rather than
	// in the compact Books API response. Failure here does not discard the
	// useful title, author, pages, or cover already found.
	metadata.Description = s.openLibraryDescription(ctx, base, isbn)
	return metadata, true, nil
}

func (s *Server) openLibraryDescription(ctx context.Context, base, isbn string) string {
	var edition struct {
		Description json.RawMessage `json:"description"`
		Works       []struct {
			Key string `json:"key"`
		} `json:"works"`
	}
	if err := s.getJSON(ctx, base+"/isbn/"+isbn+".json", &edition); err != nil {
		return ""
	}
	if description := descriptionValue(edition.Description); description != "" {
		return description
	}
	if len(edition.Works) == 0 || !strings.HasPrefix(edition.Works[0].Key, "/works/") {
		return ""
	}
	var work struct {
		Description json.RawMessage `json:"description"`
	}
	if err := s.getJSON(ctx, base+edition.Works[0].Key+".json", &work); err != nil {
		return ""
	}
	return descriptionValue(work.Description)
}

func (s *Server) lookupGoogleBooks(ctx context.Context, isbn string) (bookMetadata, bool, error) {
	base := strings.TrimRight(s.GoogleBooksBaseURL, "/")
	if base == "" {
		base = "https://www.googleapis.com/books/v1"
	}
	query := url.Values{"q": {"isbn:" + isbn}, "maxResults": {"1"}}
	if s.GoogleBooksAPIKey != "" {
		query.Set("key", s.GoogleBooksAPIKey)
	}
	var response struct {
		Items []struct {
			VolumeInfo struct {
				Title       string   `json:"title"`
				Authors     []string `json:"authors"`
				PageCount   int32    `json:"pageCount"`
				Description string   `json:"description"`
				ImageLinks  struct {
					ExtraLarge string `json:"extraLarge"`
					Large      string `json:"large"`
					Medium     string `json:"medium"`
					Small      string `json:"small"`
					Thumbnail  string `json:"thumbnail"`
				} `json:"imageLinks"`
			} `json:"volumeInfo"`
		} `json:"items"`
	}
	if err := s.getJSON(ctx, base+"/volumes?"+query.Encode(), &response); err != nil {
		if errors.Is(err, errMetadataNotFound) {
			return bookMetadata{}, false, nil
		}
		return bookMetadata{}, false, &metadataProviderError{Provider: "Google Books", Err: err}
	}
	if len(response.Items) == 0 {
		return bookMetadata{}, false, nil
	}
	info := response.Items[0].VolumeInfo
	coverURL := firstNonEmpty(info.ImageLinks.ExtraLarge, info.ImageLinks.Large,
		info.ImageLinks.Medium, info.ImageLinks.Small, info.ImageLinks.Thumbnail)
	coverURL = strings.Replace(coverURL, "http://", "https://", 1)
	return bookMetadata{
		Title:       strings.TrimSpace(info.Title),
		Author:      strings.Join(info.Authors, ", "),
		PageCount:   info.PageCount,
		Description: info.Description,
		CoverURL:    coverURL,
		Source:      "google",
	}, true, nil
}

func (s *Server) getJSON(ctx context.Context, target string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Syllabooks/1.0 (book catalogue metadata lookup)")
	client := s.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return errMetadataNotFound
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return &metadataHTTPError{StatusCode: response.StatusCode}
	}
	reader := io.LimitReader(response.Body, maxMetadataBytes+1)
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(v); err != nil {
		return &metadataDecodeError{Err: err}
	}
	return nil
}

func metadataLookupErrorMessage(err error) string {
	parts := make([]string, 0, 2)
	for _, providerErr := range metadataProviderErrors(err) {
		parts = append(parts, providerErr.Provider+" — "+metadataFailureReason(providerErr.Err))
	}
	if len(parts) == 0 {
		return "Не удалось получить данные из каталогов книг. Заполни поля вручную."
	}
	return "Не удалось получить данные: " + strings.Join(parts, "; ") + ". Заполни поля вручную."
}

func metadataProviderErrors(err error) []*metadataProviderError {
	if err == nil {
		return nil
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var result []*metadataProviderError
		for _, child := range joined.Unwrap() {
			result = append(result, metadataProviderErrors(child)...)
		}
		return result
	}
	var providerErr *metadataProviderError
	if errors.As(err, &providerErr) {
		return []*metadataProviderError{providerErr}
	}
	return nil
}

func metadataFailureReason(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "превышено время ожидания"
	}
	if errors.Is(err, context.Canceled) {
		return "запрос отменён"
	}

	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return "не удалось определить адрес сервиса"
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) && networkErr.Timeout() {
		return "превышено время ожидания"
	}

	var httpErr *metadataHTTPError
	if errors.As(err, &httpErr) {
		switch {
		case httpErr.StatusCode == http.StatusUnauthorized || httpErr.StatusCode == http.StatusForbidden:
			return fmt.Sprintf("сервис отказал в доступе (HTTP %d)", httpErr.StatusCode)
		case httpErr.StatusCode == http.StatusTooManyRequests:
			return "превышен лимит запросов (HTTP 429)"
		case httpErr.StatusCode >= 500:
			return fmt.Sprintf("сервис временно недоступен (HTTP %d)", httpErr.StatusCode)
		default:
			return fmt.Sprintf("сервис вернул HTTP %d", httpErr.StatusCode)
		}
	}

	var decodeErr *metadataDecodeError
	if errors.As(err, &decodeErr) {
		return "сервис вернул некорректный ответ"
	}
	var requestErr *url.Error
	if errors.As(err, &requestErr) {
		return "ошибка подключения"
	}
	return "неизвестная ошибка"
}

func descriptionValue(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var object struct {
		Value string `json:"value"`
		Text  string `json:"text"`
	}
	if json.Unmarshal(raw, &object) == nil {
		return firstNonEmpty(object.Value, object.Text)
	}
	return ""
}

func shortenDescription(value string) string {
	value = html.UnescapeString(htmlTagPattern.ReplaceAllString(value, " "))
	value = strings.Join(strings.Fields(value), " ")
	if utf8.RuneCountInString(value) <= 240 {
		return value
	}
	runes := []rune(value)
	short := strings.TrimSpace(string(runes[:239]))
	if lastSpace := strings.LastIndex(short, " "); lastSpace >= 160 {
		short = strings.TrimSpace(short[:lastSpace])
	}
	return short + "…"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
