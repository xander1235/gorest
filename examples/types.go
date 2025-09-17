package main

// User represents a user in the API
type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// CreateUserRequest represents the request payload for creating a user
type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// ChargeRequest represents a payment charge request
type ChargeRequest struct {
	Amount   int    `json:"amount"`   // Amount in cents
	Currency string `json:"currency"` // Currency code (USD, EUR, etc.)
	Token    string `json:"token"`    // Payment token from frontend
}
