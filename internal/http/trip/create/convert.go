package create

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"

	"github.com/google/uuid"

	domain "github.com/fumoid/x-course-avito/internal/domain/trip"
	api "github.com/fumoid/x-course-avito/internal/generated"
	usecase "github.com/fumoid/x-course-avito/internal/usecase/trip/create"
)

const maxCreateTripBodyBytes = 1 << 20

func decodeCommand(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) (usecase.Command, error) {
	body, err := decodeCreateTripBody(w, r)
	if err != nil {
		return usecase.Command{}, err
	}
	userID := body.UserId
	driverID := body.DriverId
	if userID == uuid.Nil {
		return usecase.Command{}, domain.NewValidationError("user_id", "must be a non-zero UUID")
	}
	if driverID == uuid.Nil {
		return usecase.Command{}, domain.NewValidationError("driver_id", "must be a non-zero UUID")
	}

	cmd := usecase.Command{
		UserID:   userID,
		DriverID: driverID,
		StartLat: body.StartPoint.Latitude,
		StartLon: body.StartPoint.Longitude,
		EndLat:   body.EndPoint.Latitude,
		EndLon:   body.EndPoint.Longitude,
		Price:    body.Price,
	}
	if params.IdempotencyKey != nil {
		key := *params.IdempotencyKey
		cmd.IdempotencyKey = &key
	}
	return cmd, nil
}

func decodeCreateTripBody(w http.ResponseWriter, r *http.Request) (api.TripData, error) {
	if !hasJSONContentType(r) {
		return api.TripData{}, domain.NewValidationError("Content-Type", "must be application/json")
	}

	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCreateTripBodyBytes))
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return api.TripData{}, domain.NewValidationError("body", decodeReason(err))
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return api.TripData{}, domain.NewValidationError("body", "must contain one JSON value")
	}

	fields, err := validateObjectFields(raw, "user_id", "driver_id", "start_point", "end_point", "price")
	if err != nil {
		return api.TripData{}, err
	}
	if _, err := validateObjectFields(fields["start_point"], "latitude", "longitude"); err != nil {
		return api.TripData{}, domain.NewValidationError("start_point", err.Error())
	}
	if _, err := validateObjectFields(fields["end_point"], "latitude", "longitude"); err != nil {
		return api.TripData{}, domain.NewValidationError("end_point", err.Error())
	}

	strict := json.NewDecoder(bytes.NewReader(raw))
	strict.DisallowUnknownFields()
	var body api.TripData
	if err := strict.Decode(&body); err != nil {
		if field := invalidUUIDField(fields, "user_id", "driver_id"); field != "" {
			return api.TripData{}, domain.NewValidationError(field, "must be a UUID")
		}
		return api.TripData{}, domain.NewValidationError("body", decodeReason(err))
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

func invalidUUIDField(fields map[string]json.RawMessage, names ...string) string {
	for _, name := range names {
		raw, ok := fields[name]
		if !ok {
			continue
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			continue
		}
		if _, err := uuid.Parse(value); err != nil {
			return name
		}
	}
	return ""
}
