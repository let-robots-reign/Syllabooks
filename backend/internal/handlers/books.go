package handlers

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"syllabooks/db/gen"
)

type bookResponse struct {
	ID          uuid.UUID     `json:"id"`
	ISBN        *string       `json:"isbn"`
	Title       string        `json:"title"`
	Author      string        `json:"author"`
	Level       gen.BookLevel `json:"level"`
	PageCount   int32         `json:"page_count"`
	Description *string       `json:"description"`
	CoverURL    *string       `json:"cover_url"`
}

type bookInput struct {
	ISBN        string        `json:"isbn"`
	Title       string        `json:"title"`
	Author      string        `json:"author"`
	Level       gen.BookLevel `json:"level"`
	PageCount   int32         `json:"page_count"`
	Description string        `json:"description"`
}

type validatedBook struct {
	ISBN        *string
	Title       string
	Author      string
	Level       gen.BookLevel
	PageCount   int32
	Description *string
}

func presentBook(book gen.Book) bookResponse {
	return bookResponse{
		ID:          book.ID,
		ISBN:        book.Isbn,
		Title:       book.Title,
		Author:      book.Author,
		Level:       book.Level,
		PageCount:   book.PageCount,
		Description: book.Description,
		CoverURL:    book.CoverUrl,
	}
}

func (s *Server) listBooks(w http.ResponseWriter, r *http.Request, _ gen.User) {
	books, err := gen.New(s.Pool).ListBooks(r.Context())
	if err != nil {
		serverError(w, r, fmt.Errorf("list books: %w", err))
		return
	}
	result := make([]bookResponse, 0, len(books))
	for _, book := range books {
		result = append(result, presentBook(book))
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) getBook(w http.ResponseWriter, r *http.Request, _ gen.User) {
	id, ok := bookID(w, r)
	if !ok {
		return
	}
	book, err := gen.New(s.Pool).GetBook(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		serverError(w, r, fmt.Errorf("get book: %w", err))
		return
	}
	writeJSON(w, http.StatusOK, presentBook(book))
}

func (s *Server) createBook(w http.ResponseWriter, r *http.Request, _ gen.User) {
	var input bookInput
	if !decodeJSON(w, r, &input) {
		return
	}

	var metadata bookMetadata
	if normalized, err := normalizeISBN(input.ISBN); err == nil && normalized != nil {
		foundMetadata, found, lookupErr := s.lookupMetadata(r.Context(), *normalized)
		if lookupErr != nil {
			log.Printf("book metadata lookup for %s: %v", *normalized, lookupErr)
		} else if found {
			metadata = foundMetadata
			if strings.TrimSpace(input.Title) == "" {
				input.Title = metadata.Title
			}
			if strings.TrimSpace(input.Author) == "" {
				input.Author = metadata.Author
			}
			if input.PageCount == 0 {
				input.PageCount = metadata.PageCount
			}
			if strings.TrimSpace(input.Description) == "" {
				input.Description = metadata.Description
			}
		}
	}

	book, ok := validateBook(w, input)
	if !ok {
		return
	}
	created, err := gen.New(s.Pool).CreateBook(r.Context(), gen.CreateBookParams{
		Isbn: book.ISBN, Title: book.Title, Author: book.Author, Level: book.Level,
		PageCount: book.PageCount, Description: book.Description,
	})
	if err != nil {
		writeBookDBError(w, r, "create book", err)
		return
	}

	// Metadata and cover fetching are intentionally synchronous: this happens
	// a few dozen times, and the created book remains usable if a provider is down.
	if metadata.CoverURL != "" && s.BookCoversDir != "" {
		coverURL, saveErr := s.downloadAndStoreCover(r.Context(), metadata.CoverURL)
		if saveErr != nil {
			log.Printf("store cover for book %s: %v", created.ID, saveErr)
		} else {
			updated, updateErr := gen.New(s.Pool).UpdateBookCover(r.Context(), gen.UpdateBookCoverParams{
				CoverUrl: &coverURL,
				ID:       created.ID,
			})
			if updateErr != nil {
				log.Printf("attach cover to book %s: %v", created.ID, updateErr)
				s.removeStoredCover(&coverURL)
			} else {
				created = updated
			}
		}
	}

	writeJSON(w, http.StatusCreated, presentBook(created))
}

func (s *Server) updateBook(w http.ResponseWriter, r *http.Request, _ gen.User) {
	id, ok := bookID(w, r)
	if !ok {
		return
	}
	var input bookInput
	if !decodeJSON(w, r, &input) {
		return
	}
	book, ok := validateBook(w, input)
	if !ok {
		return
	}
	updated, err := gen.New(s.Pool).UpdateBook(r.Context(), gen.UpdateBookParams{
		Isbn: book.ISBN, Title: book.Title, Author: book.Author, Level: book.Level,
		PageCount: book.PageCount, Description: book.Description, ID: id,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeBookDBError(w, r, "update book", err)
		return
	}
	writeJSON(w, http.StatusOK, presentBook(updated))
}

func (s *Server) deleteBook(w http.ResponseWriter, r *http.Request, _ gen.User) {
	id, ok := bookID(w, r)
	if !ok {
		return
	}
	coverURL, err := gen.New(s.Pool).DeleteBook(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			writeError(w, http.StatusConflict, "Нельзя удалить книгу с историей выдач.")
			return
		}
		serverError(w, r, fmt.Errorf("delete book: %w", err))
		return
	}
	s.removeStoredCover(coverURL)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) lookupBookMetadata(w http.ResponseWriter, r *http.Request, _ gen.User) {
	var input struct {
		ISBN string `json:"isbn"`
	}
	if !decodeJSON(w, r, &input) {
		return
	}
	isbn, err := normalizeISBN(input.ISBN)
	if err != nil || isbn == nil {
		writeError(w, http.StatusBadRequest, "Введи корректный ISBN из 13 цифр.")
		return
	}
	metadata, found, err := s.lookupMetadata(r.Context(), *isbn)
	if err != nil && !found {
		log.Printf("book metadata lookup for %s: %v", *isbn, err)
		writeError(w, http.StatusBadGateway, metadataLookupErrorMessage(err))
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "Книга с таким ISBN не найдена. Заполни поля вручную.")
		return
	}

	var preview *string
	if metadata.CoverURL != "" {
		data, contentType, downloadErr := s.downloadCover(r.Context(), metadata.CoverURL)
		if downloadErr == nil {
			encoded := "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data)
			preview = &encoded
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"isbn":          *isbn,
		"title":         metadata.Title,
		"author":        metadata.Author,
		"page_count":    metadata.PageCount,
		"description":   metadata.Description,
		"cover_preview": preview,
		"source":        metadata.Source,
	})
}

func validateBook(w http.ResponseWriter, input bookInput) (validatedBook, bool) {
	isbn, err := normalizeISBN(input.ISBN)
	if err != nil {
		writeError(w, http.StatusBadRequest, "ISBN должен содержать 13 цифр с верной контрольной суммой.")
		return validatedBook{}, false
	}
	title := strings.TrimSpace(input.Title)
	author := strings.TrimSpace(input.Author)
	if title == "" || author == "" {
		writeError(w, http.StatusBadRequest, "Укажи название и автора.")
		return validatedBook{}, false
	}
	if !input.Level.Valid() {
		writeError(w, http.StatusBadRequest, "Выбери уровень книги.")
		return validatedBook{}, false
	}
	if input.PageCount <= 0 {
		writeError(w, http.StatusBadRequest, "Количество страниц должно быть больше нуля.")
		return validatedBook{}, false
	}
	descriptionText := strings.TrimSpace(input.Description)
	if utf8.RuneCountInString(descriptionText) > 240 {
		writeError(w, http.StatusBadRequest, "Описание не должно быть длиннее 240 знаков.")
		return validatedBook{}, false
	}
	var description *string
	if descriptionText != "" {
		description = &descriptionText
	}
	return validatedBook{
		ISBN: isbn, Title: title, Author: author, Level: input.Level,
		PageCount: input.PageCount, Description: description,
	}, true
}

func normalizeISBN(value string) (*string, error) {
	var digits strings.Builder
	for _, r := range strings.TrimSpace(value) {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r == '-' || r == ' ':
		default:
			return nil, errors.New("isbn contains an unsupported character")
		}
	}
	normalized := digits.String()
	if normalized == "" {
		return nil, nil
	}
	if len(normalized) != 13 {
		return nil, errors.New("isbn is not 13 digits")
	}
	sum := 0
	for i := 0; i < 12; i++ {
		digit := int(normalized[i] - '0')
		if i%2 == 1 {
			digit *= 3
		}
		sum += digit
	}
	if (10-sum%10)%10 != int(normalized[12]-'0') {
		return nil, errors.New("isbn checksum is invalid")
	}
	return &normalized, nil
}

func bookID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return uuid.Nil, false
	}
	return id, true
}

func writeBookDBError(w http.ResponseWriter, r *http.Request, operation string, err error) {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		writeError(w, http.StatusConflict, "Книга с таким ISBN уже есть в каталоге.")
		return
	}
	serverError(w, r, fmt.Errorf("%s: %w", operation, err))
}
