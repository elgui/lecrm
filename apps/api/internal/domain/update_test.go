package domain

import (
	"testing"
)

func TestUpdateContactInput_Validate(t *testing.T) {
	tests := []struct {
		name    string
		input   UpdateContactInput
		wantErr bool
	}{
		// All fields empty: valid partial update — nothing to validate.
		{"all empty", UpdateContactInput{}, false},

		// Individual valid fields.
		{"valid first_name only", UpdateContactInput{FirstName: "Alice"}, false},
		{"valid last_name only", UpdateContactInput{LastName: "Smith"}, false},
		{"valid email only", UpdateContactInput{Email: "alice@example.com"}, false},

		// All fields provided and valid.
		{"all fields valid", UpdateContactInput{FirstName: "Alice", LastName: "Smith", Email: "alice@example.com"}, false},

		// Whitespace-only strings are rejected (non-empty but blank).
		{"whitespace-only first_name", UpdateContactInput{FirstName: "   "}, true},
		{"whitespace-only last_name", UpdateContactInput{LastName: "\t"}, true},

		// Invalid email formats.
		{"invalid email plain string", UpdateContactInput{Email: "not-an-email"}, true},
		{"invalid email missing domain", UpdateContactInput{Email: "user@"}, true},
		{"invalid email missing local", UpdateContactInput{Email: "@example.com"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.input.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
