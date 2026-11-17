package create

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"time"

	"github.com/google/uuid"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
	api "github.com/fumoid/x-course-avito/internal/generated"
	usecase "github.com/fumoid/x-course-avito/internal/usecase/trip/position/create"
)

const maxCreatePositionBodyBytes = 1 << 20

func decodeCommand(w http.ResponseWriter, r *http.Request, tripID uuid.UUID) (usecase.Command, error) {
	body, err := decodePositionBody(w, r)
	if err != nil {
		return usecase.Command{}, err
	}
	return usecase.Command{
		TripID:     tripID,
		Latitude:   body.Coordinates.Latitude,
		Longitude:  body.Coordinates.Longitude,
		RecordedAt: body.RecordedAt,
	}, nil
}

func decodePositionBody(w http.ResponseWriter, r *http.Request) (api.PositionData, error) {
	if !hasJSONContentType(r) {
		return api.PositionData{}, domain.NewValidationError("Content-Type", "must be application/json")
	}

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCreatePositionBodyBytes))
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return api.PositionData{}, domain.NewValidationError("body", decodeReason(err))
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return api.PositionData{}, domain.NewValidationError("body", "must contain one JSON value")
	}

	fields, err := validateObjectFields(raw, "coordinates", "recorded_at")
	if err != nil {
		return api.PositionData{}, err
	}
	if _, err := validateObjectFields(fields["coordinates"], "latitude", "longitude"); err != nil {
		return api.PositionData{}, domain.NewValidationError("coordinates", err.Error())
	}

	strict := json.NewDecoder(bytes.NewReader(raw))
	strict.DisallowUnknownFields()
	var body api.PositionData
	if err := strict.Decode(&body); err != nil {
		if recordedAtInvalid(fields["recorded_at"]) {
			return api.PositionData{}, domain.NewValidationError("recorded_at", "must be an RFC3339 timestamp")
		}
		return api.PositionData{}, domain.NewValidationError("body", decodeReason(err))
	}
	if body.RecordedAt.IsZero() {
		return api.PositionData{}, domain.NewValidationError("recorded_at", "must be set")
	}
	return body, nil
}

func validateObjectFields(raw json.RawMessage, requiredFields ...string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, domain.NewValidationError("body", "JSON value must be an object")
	}
	if object == nil {
		return nil, domain.NewValidationError("body", "JSON value must be a non-null object")
	}

	allowed := make(map[string]struct{}, len(requiredFields))
	for _, field := range requiredFields {
		allowed[field] = struct{}{}
	}
	for field := range object {
		if _, ok := allowed[field]; !ok {
			return nil, domain.NewValidationError(field, "unknown field")
		}
	}
	for _, field := range requiredFields {
		value, ok := object[field]
		if !ok {
			return nil, domain.NewValidationError(field, "required field is missing")
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, domain.NewValidationError(field, "must not be null")
		}
	}
	return object, nil
}

func hasJSONContentType(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && mediaType == "application/json"
}

func decodeReason(err error) string {
	var syntax *json.SyntaxError
	var unmarshalType *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syntax):
		return "malformed JSON"
	case errors.As(err, &unmarshalType):
		if unmarshalType.Field != "" {
			return fmt.Sprintf("field %s has invalid type", unmarshalType.Field)
		}
		return "invalid JSON types"
	default:
		return "malformed JSON"
	}
}

func recordedAtInvalid(raw json.RawMessage) bool {
	var ts time.Time
	return json.Unmarshal(raw, &ts) != nil
}
