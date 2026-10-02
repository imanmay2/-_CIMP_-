package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"net/mail"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Sasank-V/CIMP-Golang-Backend/api/utils"
	"github.com/Sasank-V/CIMP-Golang-Backend/database/schemas"
	"github.com/Sasank-V/CIMP-Golang-Backend/lib"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/term"
)

const clubID = "codechefvitc"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("run this script in an interactive terminal")
	}

	if err := godotenv.Load(); err != nil {
		log.Printf(".env was not loaded: %v", err)
	}
	connectionString := os.Getenv("CONNECTION_STRING")
	databaseName := os.Getenv("DATABASE_NAME")
	if connectionString == "" || databaseName == "" {
		return fmt.Errorf("CONNECTION_STRING and DATABASE_NAME must be set in Server/.env")
	}

	reader := bufio.NewReader(os.Stdin)
	registrationNumber, err := readRequired(reader, "Registration number")
	if err != nil {
		return err
	}
	registrationNumber = strings.ToUpper(registrationNumber)

	email, err := readRequired(reader, "Email")
	if err != nil {
		return err
	}
	email = strings.ToLower(email)
	parsedEmail, err := mail.ParseAddress(email)
	if err != nil || parsedEmail.Address != email {
		return fmt.Errorf("enter a valid email address")
	}

	firstName, err := readRequired(reader, "First name")
	if err != nil {
		return err
	}
	lastName, err := readRequired(reader, "Last name")
	if err != nil {
		return err
	}

	fmt.Print("Password (input hidden): ")
	passwordBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	if len(passwordBytes) == 0 {
		return fmt.Errorf("password cannot be empty")
	}
	passwordHash, err := utils.HashPassword(string(passwordBytes))
	if err != nil {
		return fmt.Errorf("invalid password: %w", err)
	}

	isLead, err := readIsLead(reader)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(options.Client().ApplyURI(connectionString))
	if err != nil {
		return fmt.Errorf("connect to MongoDB: %w", err)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			log.Printf("MongoDB disconnect error: %v", err)
		}
	}()
	if err := client.Ping(ctx, nil); err != nil {
		return fmt.Errorf("MongoDB connection test failed: %w", err)
	}

	db := client.Database(databaseName)
	schemas.CreateUserCollection(db)
	users := db.Collection(lib.UserCollName)
	userID := utils.GetUserIDFromRegNumber(registrationNumber)
	emailPattern := "^" + regexp.QuoteMeta(email) + "$"
	duplicateCount, err := users.CountDocuments(ctx, bson.M{
		"$or": bson.A{
			bson.M{"id": userID},
			bson.M{"reg_number": registrationNumber},
			bson.M{"email": bson.M{"$regex": emailPattern, "$options": "i"}},
		},
	})
	if err != nil {
		return fmt.Errorf("check for an existing user: %w", err)
	}
	if duplicateCount > 0 {
		return fmt.Errorf("a user with this registration number or email already exists")
	}

	user := schemas.User{
		ID:          userID,
		RegNumber:   registrationNumber,
		FirstName:   firstName,
		LastName:    lastName,
		Email:       email,
		Password:    passwordHash,
		IsLead:      isLead,
		Clubs:       []string{clubID},
		TotalPoints: 0,
		LastUpdated: time.Now(),
	}
	if _, err := users.InsertOne(ctx, user); err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	fmt.Printf("Created user %s (%s), isLead=%t.\n", user.ID, user.Email, user.IsLead)
	return nil
}

func readRequired(reader *bufio.Reader, label string) (string, error) {
	fmt.Printf("%s: ", label)
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read %s: %w", strings.ToLower(label), err)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s cannot be empty", label)
	}
	return value, nil
}

func readIsLead(reader *bufio.Reader) (bool, error) {
	fmt.Print("Is this user a lead? [y/N]: ")
	answer, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("read lead status: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "", "n", "no":
		return false, nil
	case "y", "yes":
		return true, nil
	default:
		return false, fmt.Errorf("enter y or n for lead status")
	}
}
