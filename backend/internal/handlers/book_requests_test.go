package handlers

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestBookRequests(t *testing.T) {
	env := newTestEnv(t, Server{})
	adminID, adminCode := env.newCodeUser(t)
	env.exec(t, "UPDATE users SET is_admin = true WHERE id = $1", adminID)
	adminToken := env.login(t, adminCode, "учитель")
	studentID, studentCode := env.newCodeUser(t)
	env.exec(t, "UPDATE users SET display_name = 'Аня' WHERE id = $1", studentID)
	studentToken := env.login(t, studentCode, "читатель")

	env.request(t, "POST", "/api/book-requests", "", map[string]string{"title": "Holes"}, http.StatusUnauthorized)
	env.request(t, "POST", "/api/book-requests", studentToken, map[string]string{"title": "   "}, http.StatusBadRequest)
	env.request(t, "POST", "/api/book-requests", studentToken,
		map[string]string{"title": strings.Repeat("я", maxBookRequestTitle+1)}, http.StatusBadRequest)
	env.request(t, "POST", "/api/book-requests", studentToken, map[string]string{"title": "  Holes "}, http.StatusNoContent)
	env.request(t, "POST", "/api/book-requests", studentToken, map[string]string{"title": "Wonder"}, http.StatusNoContent)

	// Admin routes stay hidden from students.
	env.request(t, "GET", "/api/admin/book-requests", studentToken, nil, http.StatusNotFound)

	mine := func() []map[string]any {
		t.Helper()
		data := env.request(t, "GET", "/api/admin/book-requests", adminToken, nil, http.StatusOK)
		var out []map[string]any
		for _, item := range data["requests"].([]any) {
			request := item.(map[string]any)
			if request["student"].(map[string]any)["id"] == studentID.String() {
				out = append(out, request)
			}
		}
		return out
	}
	requests := mine()
	if len(requests) != 2 || requests[0]["title"] != "Wonder" || requests[1]["title"] != "Holes" {
		t.Fatalf("requests = %v, want Wonder then Holes", requests)
	}
	if requests[0]["student"].(map[string]any)["display_name"] != "Аня" || requests[0]["purchased_at"] != nil {
		t.Fatalf("request = %v", requests[0])
	}

	wonder := requests[0]["id"].(string)
	holes := requests[1]["id"].(string)
	env.request(t, "PUT", "/api/admin/book-requests/"+wonder+"/purchased", adminToken,
		map[string]bool{"purchased": true}, http.StatusNoContent)
	requests = mine()
	if requests[0]["id"] != holes || requests[1]["purchased_at"] == nil {
		t.Fatalf("after purchase: %v, want Holes first and Wonder bought", requests)
	}
	env.request(t, "PUT", "/api/admin/book-requests/"+wonder+"/purchased", adminToken,
		map[string]bool{"purchased": false}, http.StatusNoContent)
	if requests = mine(); requests[0]["purchased_at"] != nil || requests[1]["purchased_at"] != nil {
		t.Fatalf("after unticking: %v", requests)
	}

	env.request(t, "DELETE", "/api/admin/book-requests/"+holes, adminToken, nil, http.StatusNoContent)
	env.request(t, "DELETE", "/api/admin/book-requests/"+holes, adminToken, nil, http.StatusNotFound)
	env.request(t, "PUT", "/api/admin/book-requests/"+holes+"/purchased", adminToken,
		map[string]bool{"purchased": true}, http.StatusNotFound)
	if requests = mine(); len(requests) != 1 || requests[0]["id"] != wonder {
		t.Fatalf("after delete: %v", requests)
	}

	// Deleting the student deletes what they asked for.
	env.exec(t, "DELETE FROM users WHERE id = $1", studentID)
	var left int
	if err := env.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM book_requests WHERE user_id = $1", studentID).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Fatalf("%d requests survived their author", left)
	}
}
