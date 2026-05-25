package domain

import (
	"fmt"
	"net/mail"
	"strings"
)

var validCompanySizes = map[string]bool{
	"1-10":    true,
	"11-50":   true,
	"51-200":  true,
	"201-1000": true,
	"1000+":   true,
}

type CreateContactInput struct {
	FirstName string
	LastName  string
	Email     string
	Phone     string
}

func (c CreateContactInput) Validate() error {
	if strings.TrimSpace(c.FirstName) == "" {
		return fmt.Errorf("first_name is required")
	}
	if strings.TrimSpace(c.LastName) == "" {
		return fmt.Errorf("last_name is required")
	}
	if c.Email != "" {
		if _, err := mail.ParseAddress(c.Email); err != nil {
			return fmt.Errorf("invalid email: %w", err)
		}
	}
	return nil
}

type CreateCompanyInput struct {
	Name string
	Size string
}

func (c CreateCompanyInput) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if c.Size != "" && !validCompanySizes[c.Size] {
		return fmt.Errorf("invalid size %q: must be one of 1-10, 11-50, 51-200, 201-1000, 1000+", c.Size)
	}
	return nil
}

type CreateDealInput struct {
	Title    string
	Currency string
}

func (d CreateDealInput) Validate() error {
	if strings.TrimSpace(d.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if d.Currency != "" && len(d.Currency) != 3 {
		return fmt.Errorf("currency must be a 3-letter ISO code, got %q", d.Currency)
	}
	return nil
}
