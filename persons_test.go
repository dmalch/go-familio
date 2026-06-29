package familio

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/gomega"
)

func TestListSettlementPersonsPaginates(t *testing.T) {
	RegisterTestingT(t)
	// Two full pages of settlementPageSize, then a short final page.
	total := settlementPageSize*2 + 5
	var gotCookie string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		Expect(r.URL.Path).To(Equal("/api/v2/persons"))
		gotCookie = r.Header.Get("Cookie")

		page := 1
		if _, err := fmt.Sscanf(r.URL.Query().Get("page"), "%d", &page); err != nil {
			page = 1
		}

		start := (page - 1) * settlementPageSize
		end := start + settlementPageSize
		if end > total {
			end = total
		}

		var data []Person
		for i := start; i < end; i++ {
			data = append(data, Person{UUID: fmt.Sprintf("u-%d", i)})
		}
		resp := personsPage{Data: data}
		resp.Pager.TotalItems = total
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	client, err := NewClient(Options{
		BaseURL:   srv.URL + "/",
		Cookies:   CookiesFromHeader("t=secret"),
		RateLimit: 1000,
	})
	Expect(err).ToNot(HaveOccurred())

	persons, err := client.ListSettlementPersons(context.Background(), "settlement-uuid")
	Expect(err).ToNot(HaveOccurred())
	Expect(persons).To(HaveLen(total))
	Expect(gotCookie).To(ContainSubstring("t=secret"), "session cookie not sent")
}

func TestDoReturnsErrNotFound(t *testing.T) {
	RegisterTestingT(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	client, _ := NewClient(Options{BaseURL: srv.URL + "/", RateLimit: 1000})
	// ListSettlementPersons uses the public (no-bearer) path, so the 404 maps
	// straight through the transport to ErrNotFound.
	_, err := client.ListSettlementPersons(context.Background(), "missing")
	Expect(err).To(MatchError(ErrNotFound))
}

func TestFlexDateUnmarshal(t *testing.T) {
	cases := []struct {
		name        string
		json        string
		wantPresent bool
		wantValue   string
	}{
		{"null", `{"birthDate":null}`, false, ""},
		{"string", `{"birthDate":"1890"}`, true, "1890"},
		{"object", `{"birthDate":{"type":"equal","calendar":"gregorian","first":null,"second":null,"formatted":"Неизвестно"}}`, true, "Неизвестно"},
		{"missing", `{}`, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			var p Person
			Expect(json.Unmarshal([]byte(tc.json), &p)).To(Succeed())
			v, ok := p.BirthDate.Value()
			Expect(ok).To(Equal(tc.wantPresent))
			if ok {
				Expect(v).To(Equal(tc.wantValue))
			}
		})
	}
}
