package domain

import (
	"testing"
)

func TestCreateContactInput_Validate(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateContactInput
		wantErr bool
	}{
		{"valid", CreateContactInput{FirstName: "John", LastName: "Doe"}, false},
		{"valid with email", CreateContactInput{FirstName: "John", LastName: "Doe", Email: "john@example.com"}, false},
		{"missing first_name", CreateContactInput{LastName: "Doe"}, true},
		{"missing last_name", CreateContactInput{FirstName: "John"}, true},
		{"whitespace first_name", CreateContactInput{FirstName: "  ", LastName: "Doe"}, true},
		{"invalid email", CreateContactInput{FirstName: "John", LastName: "Doe", Email: "not-an-email"}, true},
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

func TestCreateCompanyInput_Validate(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateCompanyInput
		wantErr bool
	}{
		{"valid", CreateCompanyInput{Name: "Acme Corp"}, false},
		{"valid with size", CreateCompanyInput{Name: "Acme Corp", Size: "11-50"}, false},
		{"missing name", CreateCompanyInput{}, true},
		{"invalid size", CreateCompanyInput{Name: "Acme Corp", Size: "huge"}, true},
		{"all sizes", CreateCompanyInput{Name: "X", Size: "1000+"}, false},
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

func TestCreateDealInput_Validate(t *testing.T) {
	tests := []struct {
		name    string
		input   CreateDealInput
		wantErr bool
	}{
		{"valid", CreateDealInput{Title: "Big Deal"}, false},
		{"valid with currency", CreateDealInput{Title: "Big Deal", Currency: "USD"}, false},
		{"missing title", CreateDealInput{}, true},
		{"invalid currency length", CreateDealInput{Title: "Deal", Currency: "US"}, true},
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
