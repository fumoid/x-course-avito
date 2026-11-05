package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"

	spec "github.com/fumoid/x-course-avito/internal/generated"
)

const maxCreateTripBodyBytes = 1 << 20

func decodeCreateTripBody(w http.ResponseWriter, r *http.Request) (spec.TripData, error) {
	if !hasJSONContentType(r) {
		return spec.TripData{}, errors.New("Content-Type must be application/json")
	}

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCreateTripBodyBytes))
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return spec.TripData{}, fmt.Errorf("decode request body: %w", err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return spec.TripData{}, errors.New("request body must contain one JSON value")
		}
		return spec.TripData{}, fmt.Errorf("decode trailing request data: %w", err)
	}

	fields, err := validateObjectFields(raw, "user_id", "driver_id", "start_point", "end_point", "price")
	if err != nil {
		return spec.TripData{}, err
	}
	if _, err := validateObjectFields(fields["start_point"], "latitude", "longitude"); err != nil {
		return spec.TripData{}, fmt.Errorf("validate start_point: %w", err)
	}
	if _, err := validateObjectFields(fields["end_point"], "latitude", "longitude"); err != nil {
		return spec.TripData{}, fmt.Errorf("validate end_point: %w", err)
	}

	strict := json.NewDecoder(bytes.NewReader(raw))
	strict.DisallowUnknownFields()
	var body spec.TripData
	if err := strict.Decode(&body); err != nil {
		return spec.TripData{}, fmt.Errorf("decode typed request body: %w", err)
	}
	return body, nil
}

func validateObjectFields(raw json.RawMessage, requiredFields ...string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("decode JSON object: %w", err)
	}
	if object == nil {
		return nil, errors.New("JSON value must be a non-null object")
	}

	allowed := make(map[string]struct{}, len(requiredFields))
	for _, field := range requiredFields {
		allowed[field] = struct{}{}
	}
	for field := range object {
		if _, ok := allowed[field]; !ok {
			return nil, fmt.Errorf("unknown field %q", field)
		}
	}
	for _, field := range requiredFields {
		value, ok := object[field]
		if !ok {
			return nil, fmt.Errorf("required field %q is missing", field)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("required field %q must not be null", field)
		}
	}
	return object, nil
}

func hasJSONContentType(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mediaType == "application/json"
}
