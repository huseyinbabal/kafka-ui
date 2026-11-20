// Package api contains the HTTP handlers and wire types.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/twmb/franz-go/pkg/kerr"

	"github.com/huseyinbabal/kafka-ui/internal/apierr"
	"github.com/huseyinbabal/kafka-ui/internal/kafka"
)

const maxBodyBytes = 1 << 20

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type listBody[T any] struct {
	Items []T `json:"items"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeList[T any](w http.ResponseWriter, items []T) {
	if items == nil {
		items = []T{}
	}
	writeJSON(w, http.StatusOK, listBody[T]{Items: items})
}

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	e := apierr.From(mapKafkaError(err))
	if e.Status >= 500 {
		slog.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	}
	var b errorBody
	b.Error.Code, b.Error.Message = e.Code, e.Message
	if e.Status >= 500 && e.Err != nil && e.Code != "INTERNAL" {
		b.Error.Message = e.Error()
	}
	writeJSON(w, e.Status, b)
}

// mapKafkaError turns well-known Kafka/protocol errors into API errors.
func mapKafkaError(err error) error {
	var ae *apierr.Error
	if errors.As(err, &ae) {
		return err
	}
	var ke *kerr.Error
	if errors.As(err, &ke) {
		msg := ke.Description
		if detail := kafka.BrokerMessage(err); detail != "" {
			msg = detail
		}
		switch {
		case errors.Is(err, kerr.UnknownTopicOrPartition):
			return &apierr.Error{Status: http.StatusNotFound, Code: "TOPIC_NOT_FOUND", Message: ke.Description, Err: err}
		case errors.Is(err, kerr.TopicAlreadyExists):
			return &apierr.Error{Status: http.StatusConflict, Code: "TOPIC_EXISTS", Message: msg, Err: err}
		case errors.Is(err, kerr.NonEmptyGroup):
			return &apierr.Error{Status: http.StatusConflict, Code: "GROUP_NOT_EMPTY", Message: msg, Err: err}
		case errors.Is(err, kerr.SecurityDisabled):
			return &apierr.Error{Status: http.StatusConflict, Code: "SECURITY_DISABLED", Message: "no authorizer is configured on this cluster", Err: err}
		case errors.Is(err, kerr.TopicAuthorizationFailed), errors.Is(err, kerr.GroupAuthorizationFailed),
			errors.Is(err, kerr.ClusterAuthorizationFailed):
			return &apierr.Error{Status: http.StatusForbidden, Code: "KAFKA_AUTHORIZATION_FAILED", Message: ke.Description, Err: err}
		default:
			return &apierr.Error{Status: http.StatusBadRequest, Code: ke.Message, Message: msg, Err: err}
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return &apierr.Error{Status: http.StatusGatewayTimeout, Code: "KAFKA_TIMEOUT", Message: "kafka did not respond in time", Err: err}
	}
	return &apierr.Error{Status: http.StatusBadGateway, Code: "KAFKA_ERROR", Message: "kafka request failed", Err: err}
}

func decodeJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return apierr.BadRequest("invalid JSON body", err)
	}
	return nil
}
