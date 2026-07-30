package familio_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"

	familio "github.com/dmalch/go-familio"
)

// Building a client from a session cookie and reading a person.
func Example() {
	client, err := familio.NewClient(familio.Options{
		// A raw "name=value; …" header copied from DevTools, or $FAMILIO_COOKIES.
		Cookies: familio.CookiesFromHeader(os.Getenv("FAMILIO_COOKIES")),
	})
	if err != nil {
		log.Fatal(err)
	}

	person, err := client.GetPersonBasic(context.Background(), "0e9bd6a4-…")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(person.FirstName, person.LastName)
}

// The public settlement list needs no credentials at all.
func ExampleClient_ListSettlementPersons() {
	client, err := familio.NewClient(familio.Options{})
	if err != nil {
		log.Fatal(err)
	}

	persons, err := client.ListSettlementPersons(context.Background(), "40d1b180-…")
	if err != nil {
		log.Fatal(err)
	}
	for _, p := range persons {
		// catalogKey is null for user-created profiles; the rest are catalog rows.
		if p.CatalogKey == nil {
			fmt.Println(p.UUID, p.DisplayName)
		}
	}
}

// Deriving a person's relations from their life events. familio has no
// relationship resource — kinship lives in events — so DeriveRelations does the
// normalizing.
func ExampleDeriveRelations() {
	client, err := familio.NewClient(familio.Options{
		Cookies: familio.CookiesFromHeader(os.Getenv("FAMILIO_COOKIES")),
	})
	if err != nil {
		log.Fatal(err)
	}

	const uuid = "0e9bd6a4-…"
	events, err := client.GetPersonEvents(context.Background(), uuid)
	if err != nil {
		log.Fatal(err)
	}

	relations := familio.DeriveRelations(events, uuid)
	for _, parent := range relations.Parents {
		fmt.Println("parent:", parent.UUID, parent.Name)
	}
	for _, spouse := range relations.Spouses {
		// MarriageUUID is the wedding event — what a marriage delete addresses.
		fmt.Println("spouse:", spouse.UUID, "marriage:", spouse.MarriageUUID)
	}
	if year, ok := familio.BirthYear(events, uuid); ok {
		fmt.Println("born:", year)
	}
}

// Editing an optimistically-locked resource: read the current version, write it
// back in X-Base-Version, and retry once on a conflict.
func ExampleClient_UpdatePersonBiography() {
	client, err := familio.NewClient(familio.Options{
		Cookies: familio.CookiesFromHeader(os.Getenv("FAMILIO_COOKIES")),
	})
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()
	const uuid = "0e9bd6a4-…"

	// The biography carries its own version, distinct from /basic's.
	bio, err := client.GetPersonBiography(ctx, uuid)
	if err != nil {
		log.Fatal(err)
	}

	updated, err := client.UpdatePersonBiography(ctx, uuid, bio.Text+"\n\nМК Журавкино 1914.", bio.UpdatedAt)
	if errors.Is(err, familio.ErrConflict) {
		// Someone else edited it in the meantime: re-read and try again.
		log.Fatal("biography changed since it was read; re-read and retry")
	}
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(updated.UpdatedAt)
}

// Reaching the HTTP status behind a failure.
func ExampleAPIError() {
	client, err := familio.NewClient(familio.Options{
		Cookies: familio.CookiesFromHeader(os.Getenv("FAMILIO_COOKIES")),
	})
	if err != nil {
		log.Fatal(err)
	}

	_, err = client.GetPersonBasic(context.Background(), "not-a-uuid")

	// The sentinels cover the common cases…
	if errors.Is(err, familio.ErrNotFound) {
		fmt.Println("no such person")
	}
	// …and APIError carries everything else.
	var apiErr *familio.APIError
	if errors.As(err, &apiErr) {
		fmt.Printf("%s %s failed with %d: %s\n", apiErr.Method, apiErr.Path, apiErr.StatusCode, apiErr.Body)
	}
}

// Tagging persons («метки»). Tag ids are integers, not uuids.
func ExampleClient_AssignPersonTags() {
	client, err := familio.NewClient(familio.Options{
		Cookies: familio.CookiesFromHeader(os.Getenv("FAMILIO_COOKIES")),
	})
	if err != nil {
		log.Fatal(err)
	}
	ctx := context.Background()

	tag, err := client.CreateTag(ctx, familio.TagInput{
		Name:        "Проверить в архиве",
		Color:       familio.TagColorMintMist,
		Description: "Нужен запрос в ЦГА",
	})
	if err != nil {
		log.Fatal(err)
	}

	// Assign adds to the person's set; it never replaces it.
	tags, err := client.AssignPersonTags(ctx, "0e9bd6a4-…", []int{tag.ID})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(len(tags), "tag(s) on the person")
}
