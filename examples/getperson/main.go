// Command getperson is a minimal smoke example that constructs a go-familio
// Client from a session cookie and fetches a single person's basic record.
//
// Run:
//
//	FAMILIO_COOKIES='t=eyJ…; …' go run ./examples/getperson <person-uuid>
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	familio "github.com/dmalch/go-familio"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("usage: %s <person-uuid>", os.Args[0])
	}
	personUUID := os.Args[1]

	cookies := os.Getenv("FAMILIO_COOKIES")
	if cookies == "" {
		log.Fatal("set FAMILIO_COOKIES to a logged-in familio.org cookie header")
	}

	client, err := familio.NewClient(familio.Options{
		Cookies: familio.CookiesFromHeader(cookies),
	})
	if err != nil {
		log.Fatalf("NewClient: %v", err)
	}

	person, err := client.GetPersonBasic(context.Background(), personUUID)
	if err != nil {
		log.Fatalf("GetPersonBasic(%q): %v", personUUID, err)
	}

	fmt.Printf("uuid:    %s\n", person.UUID)
	fmt.Printf("name:    %s\n", person.DisplayName)
	fmt.Printf("gender:  %s\n", person.Gender)
}
