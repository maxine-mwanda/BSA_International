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
	var err error
	db, err = sql.Open("mysql", dsn) // Assigning to global 'db'
	if err != nil {
		log.Fatalf("Database connection error: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("Database unreachable: %v", err)
	}

	// Initialize PayPal client
	paypalClient, err = paypal.NewClient("clientID", "secretID", paypal.APIBaseSandBox)
	if err != nil {
		log.Fatalf("PayPal initialization failed: %v", err)
	}

	// Initialize Stripe
	stripe.Key = "YOUR_STRIPE_SECRET_KEY"

	// Setup router
	r := mux.NewRouter()
	r.HandleFunc("/", homeHandler).Methods("GET")
	r.HandleFunc("/donate", donateHandler).Methods("POST")
	r.HandleFunc("/report", reportHandler).Methods("POST")

	// Serve static files (important for CSS, images, etc.)
	r.PathPrefix("/templates/").Handler(http.StripPrefix("/templates/", http.FileServer(http.Dir("templates"))))

	// Get and validate port from environment
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Default to 8080 if PORT is not set
	}

	fmt.Printf("🚀 Server started at port %s\n", port)
	log.Fatal(http.ListenAndServe("0.0.0.0:"+port, r)) // Bind router
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("templates/index.html"))
	tmpl.Execute(w, nil)
}

func donateHandler(w http.ResponseWriter, r *http.Request) {
	amount := r.FormValue("amount")
	paymentMethod := r.FormValue("payment_method")

	if amount < "1" {
		http.Error(w, "Minimum donation amount is $1", http.StatusBadRequest)
		return
	}

	// Process payment
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

	// Save donation
	_, err := db.Exec("INSERT INTO donations (amount, payment_method) VALUES (?, ?)", amount, paymentMethod)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func processStripePayment(amount string) error {
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

	for _, link := range createdOrder.Links {
		if link.Rel == "approve" {
			return link.Href, nil
		}
	}
	return "", fmt.Errorf("no approval URL found")
}

func processMPesaPayment(amount string) error {
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
