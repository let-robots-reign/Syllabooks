package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSON(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "one object", body: `{"code":"K7F2MX"}`, want: true},
		{name: "unknown field", body: `{"code":"K7F2MX","admin":true}`, want: false},
		{name: "second value", body: `{"code":"K7F2MX"} {"code":"OTHER"}`, want: false},
		{name: "trailing whitespace", body: "{\"code\":\"K7F2MX\"}\n\t", want: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body))
			var value struct {
				Code string `json:"code"`
			}
			if got := decodeJSON(recorder, request, &value); got != test.want {
				t.Fatalf("decodeJSON() = %v, want %v", got, test.want)
			}
			if test.want && value.Code != "K7F2MX" {
				t.Fatalf("decoded code = %q", value.Code)
			}
			if !test.want && recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
			}
		})
	}
}
