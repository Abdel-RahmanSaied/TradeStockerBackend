package contracts

import "fmt"

// Event error codes. A consumer quarantines a message with any of them; it
// never guesses a meaning for an unknown type, version or value.
const (
	MalformedJSON            = "MALFORMED_JSON"
	UnsupportedSchemaVersion = "UNSUPPORTED_SCHEMA_VERSION"
	UnknownEventType         = "UNKNOWN_EVENT_TYPE"
	SchemaViolation          = "SCHEMA_VIOLATION"
)

// EventError explains why a message is not an acceptable v1 event.
type EventError struct {
	Code      string
	Violation *Violation // set for SCHEMA_VIOLATION
	Message   string
}

func (e *EventError) Error() string { return e.Code + ": " + e.Message }

// EventEnvelope is the envelope schema every event is checked against when its
// type is missing or not a string.
const EventEnvelope = "events/v1/envelope.schema.json"

// EventSchema is the schema document of a v1 event type.
func EventSchema(eventType string) string { return "events/v1/" + eventType + ".v1.schema.json" }

// CheckEvent parses and validates one v1 event and returns the parsed object.
// The order is fixed and matches Python's check_event: malformed JSON; a
// schema_version that is an integer other than 1; an event_type that is a
// string outside the v1 catalog; then the first rule the envelope or the
// event's schema breaks.
func CheckEvent(r *Registry, raw []byte) (map[string]any, error) {
	v, err := Parse(raw)
	if err != nil {
		return nil, &EventError{Code: MalformedJSON, Message: err.Error()}
	}
	ref := EventEnvelope
	if obj, ok := v.(map[string]any); ok {
		if n, ok := obj["schema_version"].(Number); ok {
			if i, isInt := n.Integer(); isInt && (!i.IsInt64() || i.Int64() != 1) {
				return nil, &EventError{Code: UnsupportedSchemaVersion, Message: fmt.Sprintf("schema_version %s is not 1", n)}
			}
		}
		if t, ok := obj["event_type"].(string); ok {
			if _, known := r.docs[EventSchema(t)]; !known {
				return nil, &EventError{Code: UnknownEventType, Message: "event_type is not in the v1 catalog"}
			}
			ref = EventSchema(t)
		}
	}
	if violation := r.Validate(ref, v); violation != nil {
		return nil, &EventError{Code: SchemaViolation, Violation: violation, Message: violation.Message}
	}
	return v.(map[string]any), nil
}
