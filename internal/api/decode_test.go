package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateObjectFields(t *testing.T) {
	t.Parallel()

	raw := json.RawMessage(`{"user_id":"a","driver_id":"b","start_point":{"latitude":1},"end_point":{"longitude":2},"price":0}`)
	fields, err := validateObjectFields(raw, "user_id", "driver_id", "start_point", "end_point", "price")
	if err != nil {
		t.Fatalf("expected valid object, got %v", err)
	}
	if _, err := validateObjectFields(fields["start_point"], "latitude", "longitude"); err == nil {
		t.Fatal("expected missing longitude")
	}

	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "missing price", body: `{"user_id":"a","driver_id":"b","start_point":{"latitude":1,"longitude":2},"end_point":{"latitude":3,"longitude":4}}`, want: `required field "price" is missing`},
		{name: "null price", body: `{"user_id":"a","driver_id":"b","start_point":{"latitude":1,"longitude":2},"end_point":{"latitude":3,"longitude":4},"price":null}`, want: `required field "price" must not be null`},
		{name: "unknown field case", body: `{"user_id":"a","driver_id":"b","start_point":{"latitude":1,"longitude":2},"end_point":{"latitude":3,"longitude":4},"price":1,"Price":2}`, want: `unknown field "Price"`},
		{name: "null object", body: `null`, want: `JSON value must be a non-null object`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := validateObjectFields(json.RawMessage(tc.body), "user_id", "driver_id", "start_point", "end_point", "price")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("got %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestDecodeCreateTripBody(t *testing.T) {
	t.Parallel()

	valid := `{
		"user_id":"5cb72c04-7650-45c9-a79b-bcdba0631e0c",
		"driver_id":"8860b315-ec86-42eb-a17c-7c163d721ff5",
		"start_point":{"latitude":59.9398,"longitude":30.3146},
		"end_point":{"latitude":59.929,"longitude":30.3626},
		"price":0
	}`

	t.Run("explicit zero price is allowed", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(valid))
		req.Header.Set("Content-Type", "application/json")
		body, err := decodeCreateTripBody(httptest.NewRecorder(), req)
		if err != nil {
			t.Fatal(err)
		}
		if body.Price != 0 {
			t.Fatalf("price = %d", body.Price)
		}
	})

	t.Run("trailing json rejected", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", bytes.NewBufferString(valid+"{}"))
		req.Header.Set("Content-Type", "application/json")
		if _, err := decodeCreateTripBody(httptest.NewRecorder(), req); err == nil {
			t.Fatal("expected trailing JSON error")
		}
	})

	t.Run("content type required", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(valid))
		if _, err := decodeCreateTripBody(httptest.NewRecorder(), req); err == nil {
			t.Fatal("expected content-type error")
		}
	})
}
