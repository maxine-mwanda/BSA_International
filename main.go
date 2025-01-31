package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"text/template"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/mux"
	"github.com/plutov/paypal/v4"
	"github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/paymentintent"
)

var db *sql.DB
var paypalClient *paypal.Client

func main() {
	// Initialize MySQL database
	dsn := os.Getenv("DB_CONNECTION")
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return
	}
	if err := db.Ping(); err != nil {
		return
	}

	// Initialize PayPal client
	paypalClient, err = paypal.NewClient("clientID", "secretID", paypal.APIBaseSandBox)
	if err != nil {
		log.Fatalln(err)
	}

	// Initialize Stripe
	stripe.Key = "YOUR_STRIPE_SECRET_KEY"

	r := mux.NewRouter()

	r.HandleFunc("/", homeHandler).Methods("GET")
	r.HandleFunc("/donate", donateHandler).Methods("POST")
	r.HandleFunc("/report", reportHandler).Methods("POST")

	fmt.Println("Server started at :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("templates/index.html"))
	tmpl.Execute(w, nil)
}

func donateHandler(w http.ResponseWriter, r *http.Request) {
	amount := r.FormValue("amount")
	paymentMethod := r.FormValue("payment_method")

	// Validate minimum donation amount
	if amount < "1" {
		http.Error(w, "Minimum donation amount is $1", http.StatusBadRequest)
		return
	}

	// Process payment based on the selected method
	switch paymentMethod {
	case "Visa":
		err := processStripePayment(amount)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case "PayPal":
		_, err := createPayPalDonation(amount)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case "MPesa":
		err := processMPesaPayment(amount)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, "Invalid payment method", http.StatusBadRequest)
		return
	}

	// Save donation to the database
	_, err := db.Exec("INSERT INTO donations (amount, payment_method) VALUES (?, ?)", amount, paymentMethod)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func processStripePayment(amount string) error {
	// Convert amount to cents (Stripe requires amount in smallest currency unit)
	amountFloat, err := strconv.ParseFloat(amount, 64)
	if err != nil {
		return fmt.Errorf("invalid amount: %v", err)
	}
	amountInCents := int64(amountFloat * 100)

	params := &stripe.PaymentIntentParams{
		Amount:   stripe.Int64(amountInCents),
		Currency: stripe.String("usd"),
	}

	_, err = paymentintent.New(params)
	return err
}

func createPayPalDonation(amount string) (string, error) {
	// Create the payment
	ctx := context.Background()
	purchaseUnits := []paypal.PurchaseUnitRequest{
		{
			Amount: &paypal.PurchaseUnitAmount{
				Value:    amount,
				Currency: "USD",
			},
			Description: "Donation to BSA International",
		},
	}
	createdOrder, err := paypalClient.CreateOrder(ctx, "CAPTURE", purchaseUnits, nil, nil)
	if err != nil {
		return "", err
	}

	// Get the approval URL
	for _, link := range createdOrder.Links {
		if link.Rel == "approve" {
			return link.Href, nil
		}
	}

	return "", fmt.Errorf("no approval URL found")
}

func processMPesaPayment(amount string) error {
	// Implement M-Pesa API call here
	// Use Safaricom's API to initiate a payment request
	return nil
}

func reportHandler(w http.ResponseWriter, r *http.Request) {
	incidentDescription := r.FormValue("incident_description")

	_, err := db.Exec("INSERT INTO reports (incident_description) VALUES (?)", incidentDescription)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}
