package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
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

	needsGoogle := !openFound || result.Title == "" || result.Author == "" ||
		result.PageCount == 0 || result.Description == "" || result.CoverURL == ""
	var googleErr error
	if needsGoogle {
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
		return bookMetadata{}, false, err
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
		return bookMetadata{}, false, err
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
		return fmt.Errorf("metadata provider returned %s", response.Status)
	}
	reader := io.LimitReader(response.Body, maxMetadataBytes+1)
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(v); err != nil {
		return fmt.Errorf("decode metadata: %w", err)
	}
	return nil
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
