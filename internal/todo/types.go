package todo

import (
	"bytes"
	"encoding/json"
	"time"
)

// Todo represents a single todo item returned by the API.
type Todo struct {
	ID          int64     `json:"id"`
	Title       string    `json:"title"`
	Description *string   `json:"description"`
	Completed   bool      `json:"completed"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// CreateTodoRequest defines the payload accepted by create and replace operations.
type CreateTodoRequest struct {
	Title       string  `json:"title"`
	Description *string `json:"description"`
	Completed   bool    `json:"completed"`
}

// UpdateTodoRequest defines the payload accepted by partial update operations.
type UpdateTodoRequest struct {
	Title       *string             `json:"title,omitempty"`
	Description NullableStringPatch `json:"description"`
	Completed   *bool               `json:"completed,omitempty"`
}

// NullableStringPatch tracks whether a JSON field was omitted, null, or set.
type NullableStringPatch struct {
	Set   bool
	Value *string
}

// UnmarshalJSON marks the field as present and keeps null distinct from omission.
func (p *NullableStringPatch) UnmarshalJSON(data []byte) error {
	p.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		p.Value = nil
		return nil
	}

	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}

	p.Value = &value
	return nil
}
