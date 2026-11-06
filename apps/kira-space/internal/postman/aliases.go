package postman

import (
	"strings"

	"github.com/kirathecat/kira-studio/apps/kira-space/internal/studiostorage/model"
)

// Postman's own `{{$name}}` dynamic-variable spellings, mapped onto the `fake.`
// spellings this app now offers exclusively. Mirrors packages/api-core's own ALIAS_TO_FAKE
// (src/http/dynamic/catalog.ts) entry for entry; go-ts-api-parity.spec.ts reads this literal as
// plain text and fails if the two ever drift, the same technique that already guards validBodyModes
// and the code-language content types.
//
// The rewrite happens at import so a Postman collection's dynamic values keep working while this
// app teaches only one vocabulary. A `$name` with no faker equivalent (catalog.ts's own documented
// exclusions — the image family, the word-fragment families, names faker cannot produce) is absent
// here and is deliberately left verbatim: resolution still accepts either spelling, so an unmapped
// alias keeps generating exactly as it did.
var aliasToFake = map[string]string{
	"$guid":                 "fake.string.uuid",
	"$randomUUID":           "fake.string.uuid",
	"$timestamp":            "fake.date.timestamp",
	"$isoTimestamp":         "fake.date.iso",
	"$randomInt":            "fake.number.int",
	"$randomBoolean":        "fake.datatype.boolean",
	"$randomAlphaNumeric":   "fake.string.alphanumeric",
	"$randomColor":          "fake.color.human",
	"$randomHexColor":       "fake.color.rgbHex",
	"$randomFirstName":      "fake.person.firstName",
	"$randomLastName":       "fake.person.lastName",
	"$randomFullName":       "fake.person.fullName",
	"$randomNamePrefix":     "fake.person.prefix",
	"$randomNameSuffix":     "fake.person.suffix",
	"$randomJobTitle":       "fake.person.jobTitle",
	"$randomPhoneNumber":    "fake.phone.number",
	"$randomEmail":          "fake.internet.email",
	"$randomExampleEmail":   "fake.internet.exampleEmail",
	"$randomUserName":       "fake.internet.username",
	"$randomPassword":       "fake.internet.password",
	"$randomUrl":            "fake.internet.url",
	"$randomDomainName":     "fake.internet.domainName",
	"$randomDomainSuffix":   "fake.internet.domainSuffix",
	"$randomProtocol":       "fake.internet.protocol",
	"$randomIP":             "fake.internet.ipv4",
	"$randomIPV6":           "fake.internet.ipv6",
	"$randomMACAddress":     "fake.internet.mac",
	"$randomUserAgent":      "fake.internet.userAgent",
	"$randomSemver":         "fake.system.semver",
	"$randomCity":           "fake.location.city",
	"$randomCountry":        "fake.location.country",
	"$randomCountryCode":    "fake.location.countryCode",
	"$randomStreetAddress":  "fake.location.streetAddress",
	"$randomLatitude":       "fake.location.latitude",
	"$randomLongitude":      "fake.location.longitude",
	"$randomDatePast":       "fake.date.past",
	"$randomDateFuture":     "fake.date.future",
	"$randomDateRecent":     "fake.date.recent",
	"$randomMonth":          "fake.date.month",
	"$randomWeekday":        "fake.date.weekday",
	"$randomCompanyName":    "fake.company.name",
	"$randomCatchPhrase":    "fake.company.catchPhrase",
	"$randomProductName":    "fake.commerce.productName",
	"$randomDepartment":     "fake.commerce.department",
	"$randomPrice":          "fake.commerce.price",
	"$randomCurrencyCode":   "fake.finance.currencyCode",
	"$randomBankAccount":    "fake.finance.accountNumber",
	"$randomBitcoin":        "fake.finance.bitcoinAddress",
	"$randomWord":           "fake.word.sample",
	"$randomWords":          "fake.word.words",
	"$randomLoremWord":      "fake.lorem.word",
	"$randomLoremWords":     "fake.lorem.words",
	"$randomLoremSentence":  "fake.lorem.sentence",
	"$randomLoremParagraph": "fake.lorem.paragraph",
	"$randomLoremSlug":      "fake.lorem.slug",
	"$randomFileName":       "fake.system.fileName",
	"$randomFileExt":        "fake.system.fileExt",
	"$randomMimeType":       "fake.system.mimeType",
}

// rewriteAliases replaces every `{{$name}}` whose name is in aliasToFake with `{{fake.x.y}}`.
//
// Deliberately a scan for the exact `{{$` opener rather than a regexp over the whole string: a
// reference's name is matched whole against the table, so `{{$randomEmailX}}` and `{{ $guid }}`
// (Postman does not permit inner spaces) are left alone rather than partially rewritten, and a
// `{{` with no closing `}}` terminates the scan instead of consuming the rest of the buffer.
func rewriteAliases(s string) string {
	if !strings.Contains(s, "{{$") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		open := strings.Index(s[i:], "{{$")
		if open < 0 {
			b.WriteString(s[i:])
			break
		}
		open += i
		end := strings.Index(s[open:], "}}")
		if end < 0 {
			b.WriteString(s[i:])
			break
		}
		end += open
		name := s[open+2 : end]
		b.WriteString(s[i:open])
		if fake, ok := aliasToFake[name]; ok {
			b.WriteString("{{" + fake + "}}")
		} else {
			b.WriteString(s[open : end+2])
		}
		i = end + 2
	}
	return b.String()
}

// rewriteRequestAliases applies rewriteAliases to every field of an imported request that can
// carry a `{{reference}}`. Called from importRequest, which means ShedOrigin's own
// importRequest-vs-saved comparison sees the identical rewrite on both sides — so an imported
// request nobody has edited still exports byte-for-byte from its Origin, aliases intact.
//
// A header's or field's *name* is rewritten too: `X-{{$guid}}-Trace` is legal in Postman.
// Descriptions are not — they are prose about the request, not part of what gets sent.
// A collection-level variable's value is deliberately out of scope: unlike a request, it is
// re-emitted on export from Variables rather than from Origin (collection.go's own note), so
// rewriting one would change the exported bytes of an untouched collection.
func rewriteRequestAliases(r *model.SavedRequest) {
	r.URL = rewriteAliases(r.URL)
	r.Body = rewriteAliases(r.Body)
	r.Code = rewriteAliases(r.Code)
	for i := range r.Headers {
		r.Headers[i].Name = rewriteAliases(r.Headers[i].Name)
		r.Headers[i].Value = rewriteAliases(r.Headers[i].Value)
	}
	for i := range r.URLEncoded {
		r.URLEncoded[i].Name = rewriteAliases(r.URLEncoded[i].Name)
		r.URLEncoded[i].Value = rewriteAliases(r.URLEncoded[i].Value)
	}
	for i := range r.FormData {
		r.FormData[i].Name = rewriteAliases(r.FormData[i].Name)
		r.FormData[i].Value = rewriteAliases(r.FormData[i].Value)
	}
}
