package protocol_test

import (
	"encoding/json"
	"net/url"
	"strconv"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

func BenchmarkProjection(b *testing.B) {
	schemas := core.Schemas{
		core.NewSchema(core.SchemaUser).WithName("User").With(core.UserAttributes()...),
		core.NewSchema(core.SchemaEnterpriseUser).With(core.NewAttribute("employeeNumber", core.TypeString), core.NewAttribute("department", core.TypeString)),
	}
	users := make([]*core.User, 100)
	for i := range users {
		users[i] = benchUser(i)
	}
	for _, query := range []string{"", "attributes=userName,emails.value", "excludedAttributes=addresses,phoneNumbers"} {
		values, _ := url.ParseQuery(query)
		projection, err := protocol.ParseProjection(values, schemas)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(strconv.Quote(query), func(b *testing.B) { benchMarshal(b, projection.All(users)) })
	}
	b.Run("baseline without projection", func(b *testing.B) { benchMarshal(b, users) })
}

func benchMarshal(b *testing.B, value any) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := json.Marshal(value); err != nil {
			b.Fatal(err)
		}
	}
}

func benchUser(i int) *core.User {
	id := strconv.Itoa(i)
	primary := true
	user := &core.User{
		UserName:    "user" + id,
		DisplayName: "User " + id,
		Name:        core.Name{GivenName: "Given" + id, FamilyName: "Family" + id},
		Emails: []core.Email{
			{Value: "work" + id + "@example.com", Type: "work", Primary: &primary},
			{Value: "home" + id + "@example.com", Type: "home"},
		},
		PhoneNumbers:   []core.PhoneNumber{{Value: "555-01" + id, Type: "work"}},
		Addresses:      []core.Address{{StreetAddress: id + " Main St", Locality: "Springfield", Type: "work"}},
		EnterpriseUser: &core.EnterpriseUser{EmployeeNumber: "E" + id, Department: "Engineering"},
	}
	user.ID = id
	return user
}
