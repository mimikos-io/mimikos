package generator

import (
	"fmt"
	"math"
	"strings"

	"github.com/brianvoe/gofakeit/v7"
)

// Func produces a typed value from a seeded Faker instance.
// Each call with the same Faker seed produces identical output.
//
// The returned value's Go type reflects the semantic meaning of the field
// (e.g., string for emails, float64 for prices, bool for flags). It is
// NOT guaranteed to match the JSON Schema type declared for that field.
// The caller (typically the DataGenerator) is responsible for coercing the
// value to the schema-declared type when they differ.
type Func func(f *gofakeit.Faker) any

// SemanticMapper maps JSON field names to contextually appropriate faker
// functions. It normalizes field names to handle snake_case, camelCase,
// and kebab-case variations transparently.
type SemanticMapper struct {
	table map[string]Func
}

// NewSemanticMapper creates a mapper with the default field-name patterns.
// The lookup table is built once and is read-only at runtime.
func NewSemanticMapper() *SemanticMapper {
	return &SemanticMapper{table: defaultMappings()}
}

// Match returns the generator function for a field name, if one exists.
// The lookup is case-insensitive and ignores underscores and hyphens,
// so "firstName", "first_name", "first-name", and "FIRST_NAME" all
// resolve to the same generator.
func (m *SemanticMapper) Match(fieldName string) (Func, bool) {
	fn, ok := m.table[normalize(fieldName)]
	if !ok {
		return nil, false
	}

	return fn, true
}

// Len returns the number of field-name entries in the mapper.
func (m *SemanticMapper) Len() int {
	return len(m.table)
}

// normalize converts a field name to its canonical lookup key by
// lowercasing and stripping underscores and hyphens.
func normalize(name string) string {
	lower := strings.ToLower(name)
	lower = strings.ReplaceAll(lower, "_", "")
	lower = strings.ReplaceAll(lower, "-", "")

	return lower
}

// roundTo2 rounds a float64 to 2 decimal places.
func roundTo2(f float64) float64 {
	return math.Round(f*100) / 100 //nolint:mnd
}

// stripNonAlphanumeric removes any character that is not a lowercase letter or digit.
func stripNonAlphanumeric(s string) string {
	var b strings.Builder

	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}

	return b.String()
}

// defaultMappings builds the ~100-entry field-name-to-faker lookup table.
// Keys are pre-normalized (lowercase, no separators).
//
//nolint:funlen,maintidx // lookup table is intentionally large
func defaultMappings() map[string]Func {
	m := make(map[string]Func, 120) //nolint:mnd

	// --- Identity ---
	uuidGen := func(f *gofakeit.Faker) any { return f.UUID() }
	for _, k := range []string{
		"id", "uuid", "guid", "userid", "orderid", "accountid",
		"transactionid", "sessionid", "requestid", "correlationid",
		"traceid", "parentid", "externalid",
	} {
		m[k] = uuidGen
	}

	// --- Person ---
	m["firstname"] = func(f *gofakeit.Faker) any { return f.FirstName() }
	m["lastname"] = func(f *gofakeit.Faker) any { return f.LastName() }
	m["name"] = func(f *gofakeit.Faker) any { return f.Name() }
	m["fullname"] = func(f *gofakeit.Faker) any { return f.Name() }
	m["username"] = func(f *gofakeit.Faker) any { return f.Username() }
	m["displayname"] = func(f *gofakeit.Faker) any { return f.Name() }
	m["nickname"] = func(f *gofakeit.Faker) any { return f.FirstName() }
	m["middlename"] = func(f *gofakeit.Faker) any { return f.FirstName() }
	m["prefix"] = func(f *gofakeit.Faker) any { return f.NamePrefix() }
	m["suffix"] = func(f *gofakeit.Faker) any { return f.NameSuffix() }

	// --- Contact ---
	m["email"] = func(f *gofakeit.Faker) any { return f.Email() }
	m["emailaddress"] = func(f *gofakeit.Faker) any { return f.Email() }
	m["phone"] = func(f *gofakeit.Faker) any { return f.Phone() }
	m["phonenumber"] = func(f *gofakeit.Faker) any { return f.Phone() }
	m["fax"] = func(f *gofakeit.Faker) any { return f.Phone() }

	// --- Address ---
	m["street"] = func(f *gofakeit.Faker) any { return f.Street() }
	m["streetaddress"] = func(f *gofakeit.Faker) any { return f.Street() }
	m["address"] = func(f *gofakeit.Faker) any { return f.Street() }
	m["addressline1"] = func(f *gofakeit.Faker) any { return f.Street() }
	m["addressline2"] = func(f *gofakeit.Faker) any {
		return fmt.Sprintf("Apt %d", f.IntRange(1, 999)) //nolint:mnd
	}
	m["city"] = func(f *gofakeit.Faker) any { return f.City() }
	m["state"] = func(f *gofakeit.Faker) any { return f.State() }
	m["province"] = func(f *gofakeit.Faker) any { return f.State() }
	m["zip"] = func(f *gofakeit.Faker) any { return f.Zip() }
	m["zipcode"] = func(f *gofakeit.Faker) any { return f.Zip() }
	m["postalcode"] = func(f *gofakeit.Faker) any { return f.Zip() }
	m["country"] = func(f *gofakeit.Faker) any { return f.Country() }
	m["countrycode"] = func(f *gofakeit.Faker) any { return f.CountryAbr() }
	m["latitude"] = func(f *gofakeit.Faker) any { return f.Latitude() }
	m["longitude"] = func(f *gofakeit.Faker) any { return f.Longitude() }
	m["lat"] = func(f *gofakeit.Faker) any { return f.Latitude() }
	m["lng"] = func(f *gofakeit.Faker) any { return f.Longitude() }
	m["lon"] = func(f *gofakeit.Faker) any { return f.Longitude() }

	// --- Time (datetime) ---
	datetimeGen := func(f *gofakeit.Faker) any { return f.Date().Format("2006-01-02T15:04:05Z") }
	for _, k := range []string{
		"createdat", "updatedat", "deletedat", "timestamp", "datetime",
		"startat", "endat", "expiresat", "modifiedat", "occurredat",
	} {
		m[k] = datetimeGen
	}

	m["time"] = func(f *gofakeit.Faker) any { return f.Date().Format("15:04:05Z") }

	// --- Time (date only) ---
	dateGen := func(f *gofakeit.Faker) any { return f.Date().Format("2006-01-02") }
	for _, k := range []string{"date", "birthday", "dob", "duedate", "startdate", "enddate"} {
		m[k] = dateGen
	}

	// --- Financial ---
	financialGen := func(f *gofakeit.Faker) any { return roundTo2(f.Float64Range(0.01, 9999.99)) } //nolint:mnd
	for _, k := range []string{
		"price", "amount", "total", "subtotal", "tax", "discount",
		"balance", "cost", "fee", "rate",
	} {
		m[k] = financialGen
	}

	// --- Currency ---
	m["currency"] = func(f *gofakeit.Faker) any { return f.CurrencyShort() }
	m["currencycode"] = func(f *gofakeit.Faker) any { return f.CurrencyShort() }

	// --- Web ---
	urlGen := func(f *gofakeit.Faker) any { return f.URL() }
	for _, k := range []string{
		"url", "uri", "website", "homepage", "href", "link",
		"callback", "redirect", "webhookurl",
	} {
		m[k] = urlGen
	}

	m["imageurl"] = func(f *gofakeit.Faker) any {
		return fmt.Sprintf("https://picsum.photos/seed/%s/640/480", f.LetterN(8)) //nolint:mnd
	}
	m["avatarurl"] = func(f *gofakeit.Faker) any {
		return fmt.Sprintf("https://picsum.photos/seed/%s/128/128", f.LetterN(8)) //nolint:mnd
	}
	m["avatar"] = func(f *gofakeit.Faker) any {
		return fmt.Sprintf("https://picsum.photos/seed/%s/128/128", f.LetterN(8)) //nolint:mnd
	}

	// --- Text ---
	sentenceGen := func(f *gofakeit.Faker) any { return f.Sentence(10) }             //nolint:mnd
	paragraphGen := func(f *gofakeit.Faker) any { return f.Paragraph(2, 3, 5, " ") } //nolint:mnd

	m["title"] = func(f *gofakeit.Faker) any { return f.Sentence(4) }    //nolint:mnd
	m["subject"] = func(f *gofakeit.Faker) any { return f.Sentence(4) }  //nolint:mnd
	m["headline"] = func(f *gofakeit.Faker) any { return f.Sentence(5) } //nolint:mnd
	m["label"] = func(f *gofakeit.Faker) any { return f.Word() }
	m["caption"] = sentenceGen

	for _, k := range []string{"description", "summary", "bio", "about", "comment", "note", "notes"} {
		m[k] = paragraphGen
	}

	for _, k := range []string{"message", "body", "content", "text"} {
		m[k] = sentenceGen
	}

	// --- Boolean flags ---
	boolGen := func(f *gofakeit.Faker) any { return f.Bool() }
	for _, k := range []string{
		"isactive", "isdeleted", "isenabled", "isdisabled", "isverified",
		"isadmin", "ispublic", "isprivate", "enabled", "disabled",
		"active", "verified", "published", "archived", "deleted", "blocked",
	} {
		m[k] = boolGen
	}

	// --- Network ---
	m["ipaddress"] = func(f *gofakeit.Faker) any { return f.IPv4Address() }
	m["ip"] = func(f *gofakeit.Faker) any { return f.IPv4Address() }
	m["ipv4"] = func(f *gofakeit.Faker) any { return f.IPv4Address() }
	m["ipv6"] = func(f *gofakeit.Faker) any { return f.IPv6Address() }
	m["macaddress"] = func(f *gofakeit.Faker) any { return f.MacAddress() }
	m["mac"] = func(f *gofakeit.Faker) any { return f.MacAddress() }
	m["hostname"] = func(f *gofakeit.Faker) any { return f.DomainName() }
	m["domain"] = func(f *gofakeit.Faker) any { return f.DomainName() }
	m["domainname"] = func(f *gofakeit.Faker) any { return f.DomainName() }
	m["useragent"] = func(f *gofakeit.Faker) any { return f.UserAgent() }

	// --- Identifiers ---
	m["slug"] = func(f *gofakeit.Faker) any {
		words := strings.Fields(f.Sentence(3)) //nolint:mnd
		for i, w := range words {
			words[i] = stripNonAlphanumeric(strings.ToLower(w))
		}

		return strings.Join(words, "-")
	}
	m["sku"] = func(f *gofakeit.Faker) any { return f.LetterN(10) }    //nolint:mnd
	m["barcode"] = func(f *gofakeit.Faker) any { return f.DigitN(13) } //nolint:mnd
	m["isbn"] = func(f *gofakeit.Faker) any { return f.DigitN(13) }    //nolint:mnd
	m["ssn"] = func(f *gofakeit.Faker) any { return f.SSN() }
	m["ein"] = func(f *gofakeit.Faker) any { return f.DigitN(9) }   //nolint:mnd
	m["taxid"] = func(f *gofakeit.Faker) any { return f.DigitN(9) } //nolint:mnd

	// --- Counts ---
	countGen := func(f *gofakeit.Faker) any { return f.IntRange(1, 1000) } //nolint:mnd
	for _, k := range []string{
		"count", "quantity", "qty", "size", "page", "limit", "offset",
	} {
		m[k] = countGen
	}

	measureGen := func(f *gofakeit.Faker) any { return f.IntRange(1, 500) } //nolint:mnd
	for _, k := range []string{"length", "width", "height", "weight", "age"} {
		m[k] = measureGen
	}

	// --- Color ---
	m["color"] = func(f *gofakeit.Faker) any { return f.HexColor() }
	m["colour"] = func(f *gofakeit.Faker) any { return f.HexColor() }
	m["hexcolor"] = func(f *gofakeit.Faker) any { return f.HexColor() }
	m["bgcolor"] = func(f *gofakeit.Faker) any { return f.HexColor() }
	m["backgroundcolor"] = func(f *gofakeit.Faker) any { return f.HexColor() }

	// --- File ---
	m["filename"] = func(f *gofakeit.Faker) any {
		return f.Word() + "." + f.FileExtension()
	}
	m["filepath"] = func(f *gofakeit.Faker) any {
		return "/" + f.Word() + "/" + f.Word() + "." + f.FileExtension()
	}
	m["mimetype"] = func(f *gofakeit.Faker) any { return f.FileMimeType() }
	m["contenttype"] = func(f *gofakeit.Faker) any { return f.FileMimeType() }
	m["extension"] = func(f *gofakeit.Faker) any { return f.FileExtension() }
	m["filesize"] = func(f *gofakeit.Faker) any { return f.IntRange(1024, 10485760) } //nolint:mnd

	// --- Locale ---
	m["language"] = func(f *gofakeit.Faker) any { return f.Language() }
	m["languagecode"] = func(f *gofakeit.Faker) any { return f.LanguageAbbreviation() }
	m["locale"] = func(f *gofakeit.Faker) any { return f.LanguageAbbreviation() }
	m["timezone"] = func(f *gofakeit.Faker) any { return f.TimeZone() }
	m["tz"] = func(f *gofakeit.Faker) any { return f.TimeZoneAbv() }

	// --- Company ---
	m["company"] = func(f *gofakeit.Faker) any { return f.Company() }
	m["companyname"] = func(f *gofakeit.Faker) any { return f.Company() }
	m["organization"] = func(f *gofakeit.Faker) any { return f.Company() }
	m["employer"] = func(f *gofakeit.Faker) any { return f.Company() }
	m["brand"] = func(f *gofakeit.Faker) any { return f.Company() }

	// --- Job ---
	m["jobtitle"] = func(f *gofakeit.Faker) any { return f.JobTitle() }
	m["role"] = func(f *gofakeit.Faker) any { return f.JobTitle() }
	m["position"] = func(f *gofakeit.Faker) any { return f.JobTitle() }
	m["department"] = func(f *gofakeit.Faker) any { return f.Word() }
	m["team"] = func(f *gofakeit.Faker) any { return f.Word() }

	return m
}
