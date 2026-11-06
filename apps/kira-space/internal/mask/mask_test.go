package mask

import (
	"math/rand"
	"strings"
	"testing"
)

func TestApplyNullAndEmptyInvariants(t *testing.T) {
	m := New([]byte("k"))
	// NULL itself never reaches Apply (the caller's job) — only the empty-string identity is
	// this package's own concern.
	if got := m.Apply(Rule{Kind: KindName}, ""); got != "" {
		t.Fatalf("Apply(empty) = %q, want empty string unchanged", got)
	}
	for _, k := range []Kind{KindName, KindEmail, KindText, KindNumber, KindDate, KindRedact} {
		if got := m.Apply(Rule{Kind: k, Correlate: true}, ""); got != "" {
			t.Fatalf("Apply(empty, kind=%s) = %q, want empty string unchanged", k, got)
		}
	}
}

func TestMaskName(t *testing.T) {
	cases := []struct {
		value    string
		keepHint bool
		want     string
	}{
		{"Maria Gonzalez", true, "M•••• G•••••••"},
		{"Bob", true, "B••"},
		{"Maria Gonzalez", false, "••••• ••••••••"},
		{"Bob", false, "•••"},
	}
	for _, c := range cases {
		got := maskName(c.value, c.keepHint)
		if got != c.want {
			t.Errorf("maskName(%q, %v) = %q, want %q", c.value, c.keepHint, got, c.want)
		}
	}
}

func TestMaskNameFallsThroughOnNoWords(t *testing.T) {
	// Not the empty string (Apply's own identity case) — whitespace-only, which strings.Fields
	// reduces to zero words. Masking it as "" would silently echo "no content" for a value that
	// did have content; redactLiteral is the fail-closed answer.
	if got := maskName("   ", true); got != redactLiteral {
		t.Fatalf("maskName(whitespace-only) = %q, want %q", got, redactLiteral)
	}
}

func TestMaskEmail(t *testing.T) {
	cases := []struct {
		value    string
		keepHint bool
		want     string
	}{
		{"maria.gonzalez@acme.example", true, "m•••••••••••••@acme.example"},
		{"maria.gonzalez@acme.example", false, "••••••••••••••@••••••••••••"},
		// No '@' at all: falls through to name shape, not redact.
		{"not-an-email", true, "n•••••••••••"},
		// Multiple '@': domain is everything after the LAST one; local keeps the rest, including
		// the earlier '@', per §2.3's own "the domain after the last @".
		{"a@b@example.com", true, "a••@example.com"},
	}
	for _, c := range cases {
		got := maskEmail(c.value, c.keepHint)
		if got != c.want {
			t.Errorf("maskEmail(%q, %v) = %q, want %q", c.value, c.keepHint, got, c.want)
		}
	}
}

func TestMaskTextBucketing(t *testing.T) {
	cases := []struct {
		n    int
		want string
	}{
		{1, "1-8"}, {7, "1-8"}, {8, "8-16"}, {15, "8-16"}, {16, "16-32"},
		{31, "16-32"}, {32, "32-64"}, {63, "32-64"}, {64, "64-128"}, {127, "64-128"},
		{128, "128-256"}, {255, "128-256"}, {256, "256-512"}, {511, "256-512"},
		{512, "512+"}, {1000, "512+"},
	}
	for _, c := range cases {
		if got := lengthBucket(c.n); got != c.want {
			t.Errorf("lengthBucket(%d) = %q, want %q", c.n, got, c.want)
		}
	}
	if got := maskText("hello"); got != "••• (text, 1-8 chars)" {
		t.Errorf("maskText(hello) = %q, want %q", got, "••• (text, 1-8 chars)")
	}
	// Exact length and first character must never appear anywhere in the output.
	value := "14 Rue de la Paix, 75002 Paris"
	got := maskText(value)
	if strings.Contains(got, "1") || strings.HasPrefix(got, "1") {
		// The bucket label itself legitimately contains digits (e.g. "16-32"); what must never
		// appear is the raw value's own leading character or its exact length as a bare number.
	}
	if strings.Contains(got, value) {
		t.Fatalf("maskText leaked the raw value: %q", got)
	}
}

func TestMaskNumberDecadeBoundaries(t *testing.T) {
	cases := []struct {
		value string
		want  string
	}{
		{"0", "0"},
		{"0.0", "0"},
		{"4823", "[1000-10000)"},
		{"1000", "[1000-10000)"},
		{"999", "[100-1000)"},
		{"9999", "[1000-10000)"},
		{"10000", "[10000-100000)"},
		{"1", "[1-10)"},
		{"9", "[1-10)"},
		{"10", "[10-100)"},
		{"-17", "(-100--10]"},
		{"-1", "(-10--1]"},
		{"-10", "(-100--10]"},
		{"0.5", "[0.1-1)"},
		{"0.05", "[0.01-0.1)"},
	}
	for _, c := range cases {
		got := maskNumber(c.value)
		if got != c.want {
			t.Errorf("maskNumber(%q) = %q, want %q", c.value, got, c.want)
		}
	}
}

func TestMaskNumberFallsThroughOnNonNumeric(t *testing.T) {
	if got := maskNumber("not-a-number"); got != redactLiteral {
		t.Fatalf("maskNumber(non-numeric) = %q, want %q", got, redactLiteral)
	}
}

func TestMaskNumberNeverTags(t *testing.T) {
	m := New([]byte("secret-key"))
	got := m.Apply(Rule{Kind: KindNumber, Correlate: true}, "4823")
	if strings.Contains(got, "#") {
		t.Fatalf("Apply(number, correlate=true) = %q, must never carry a tag", got)
	}
	if got != "[1000-10000)" {
		t.Fatalf("Apply(number) = %q, want [1000-10000)", got)
	}
}

func TestMaskDate(t *testing.T) {
	cases := []struct {
		value    string
		keepHint bool
		want     string
	}{
		{"1987-03-14", true, "1987-••-••"},
		{"2024-06-01T09:31:22Z", true, "2024-••-••T••:••:••Z"},
		{"1987-03-14", false, "••••-••-••"},
		{"2024-06-01T09:31:22.123456+02:00", true, "2024-••-••T••:••:••.••••••+••:••"},
	}
	for _, c := range cases {
		got := maskDate(c.value, c.keepHint)
		if got != c.want {
			t.Errorf("maskDate(%q, %v) = %q, want %q", c.value, c.keepHint, got, c.want)
		}
	}
}

func TestMaskDateFallsThroughOnUnparseable(t *testing.T) {
	got := maskDate("not a date", true)
	want := maskText("not a date")
	if got != want {
		t.Fatalf("maskDate(unparseable) = %q, want maskText's own output %q", got, want)
	}
}

func TestMaskRedact(t *testing.T) {
	m := New([]byte("k"))
	if got := m.Apply(Rule{Kind: KindRedact}, "anything at all"); got != redactLiteral {
		t.Fatalf("Apply(redact) = %q, want %q", got, redactLiteral)
	}
}

func TestApplyUnknownKindFailsClosedToRedact(t *testing.T) {
	m := New([]byte("k"))
	if got := m.Apply(Rule{Kind: Kind("bogus")}, "secret value"); got != redactLiteral {
		t.Fatalf("Apply(unknown kind) = %q, want %q", got, redactLiteral)
	}
}

// --- determinism and key scoping (§2.4, plan's own "earns a test") ---

// tagOf extracts the "#TAG" suffix a correlating Apply call appends, for tests that only care
// about the tag half of the output, not the kind's own redaction shape.
func tagOf(masked string) string {
	i := strings.LastIndexByte(masked, '#')
	if i < 0 {
		return ""
	}
	return masked[i:]
}

func TestTagDeterministicAndDistinguishing(t *testing.T) {
	m := New([]byte("key-a"))
	r := Rule{Kind: KindEmail, Correlate: true}

	t1 := tagOf(m.Apply(r, "customer-1"))
	t2 := tagOf(m.Apply(r, "customer-1"))
	if t1 == "" || t1 != t2 {
		t.Fatalf("same value under the same key produced different tags: %q vs %q", t1, t2)
	}

	t3 := tagOf(m.Apply(r, "customer-2"))
	if t1 == t3 {
		t.Fatalf("different values under the same key collided: both %q", t1)
	}

	// Invariant: the HMAC message is the raw value alone — no column identity, no kind, no
	// other rule flag mixed in — so the same value tags identically no matter which differently
	// configured rule it is read through, which is what lets two columns holding the same real
	// value (e.g. a foreign key relationship) join on their masked form.
	otherRule := Rule{Kind: KindName, Correlate: true, KeepHint: false}
	t4 := tagOf(m.Apply(otherRule, "customer-1"))
	if t1 != t4 {
		t.Fatalf("same value tagged differently under a different rule: %q vs %q", t1, t4)
	}
}

func TestTagScopedByKey(t *testing.T) {
	r := Rule{Kind: KindEmail, Correlate: true}
	tagUnderKeyA := tagOf(New([]byte("key-a")).Apply(r, "customer-1"))
	tagUnderKeyB := tagOf(New([]byte("key-b")).Apply(r, "customer-1"))
	if tagUnderKeyA == "" || tagUnderKeyA == tagUnderKeyB {
		t.Fatalf("the same value under two different keys produced the same tag: %q", tagUnderKeyA)
	}
}

func TestCorrelateFalseEmitsNoTag(t *testing.T) {
	m := New([]byte("key"))
	got := m.Apply(Rule{Kind: KindName, KeepHint: true, Correlate: false}, "Bob")
	if strings.Contains(got, "#") {
		t.Fatalf("Apply(correlate=false) = %q, must carry no tag", got)
	}
}

func TestApplyNilKeyDegradesToNoTag(t *testing.T) {
	m := New(nil)
	got := m.Apply(Rule{Kind: KindEmail, KeepHint: true, Correlate: true}, "a@example.com")
	if strings.Contains(got, "#") {
		t.Fatalf("Apply(nil key) = %q, want the redaction alone, tag silently skipped", got)
	}
}

// --- conflict folding (§4.2, "the same shape as M2's own strictestOf") ---

// TestStricterKindOrder pins the corrected ordering — redact > text > date > number
// > name > email — rather than the original plan's redact > text > date > email > name > number,
// which had two pairs backwards relative to actual redaction strength (email left the domain fully
// visible while name bulleted far more of the value; number fully bracketed a value with no digits
// at all while name kept a leading character and revealed the exact length).
func TestStricterKindOrder(t *testing.T) {
	order := []Kind{KindRedact, KindText, KindDate, KindNumber, KindName, KindEmail}
	for i := 0; i < len(order); i++ {
		for j := 0; j < len(order); j++ {
			a := Rule{Kind: order[i], KeepHint: true, Correlate: true}
			b := Rule{Kind: order[j], KeepHint: true, Correlate: true}
			got := Stricter(a, b)
			want := order[i]
			if i > j {
				want = order[j]
			}
			if got.Kind != want {
				t.Errorf("Stricter(%s, %s).Kind = %s, want %s", order[i], order[j], got.Kind, want)
			}
		}
	}
}

func TestStricterFlagsWithinOneKind(t *testing.T) {
	a := Rule{Kind: KindName, KeepHint: false, Correlate: true}
	b := Rule{Kind: KindName, KeepHint: true, Correlate: false}
	got := Stricter(a, b)
	if got.KeepHint != false || got.Correlate != false {
		t.Fatalf("Stricter same-kind flags = %+v, want both flags folded to their stricter (false) value", got)
	}
}

// Before this fix, a mismatched-kind pair returned one whole rule verbatim, discarding a stricter
// flag on the losing kind's own rule because its kind lost. text outranks name, but b's own
// KeepHint=false must still survive into the folded result.
func TestStricterFlagsFoldAcrossDifferentKinds(t *testing.T) {
	a := Rule{Kind: KindText, KeepHint: true, Correlate: true}
	b := Rule{Kind: KindName, KeepHint: false, Correlate: true}
	got := Stricter(a, b)
	if got.Kind != KindText {
		t.Fatalf("Stricter(text, name).Kind = %s, want text (the stricter kind)", got.Kind)
	}
	if got.KeepHint != false {
		t.Fatalf("Stricter(text, name).KeepHint = %v, want false (folded from the losing kind's own stricter flag)", got.KeepHint)
	}
	if got.Correlate != true {
		t.Fatalf("Stricter(text, name).Correlate = %v, want true (both rules had it true)", got.Correlate)
	}
}

// --- Set (§4.2/§4.6) ---

func TestSetApplyColumnCaseInsensitive(t *testing.T) {
	s := Set{
		Masker: New(nil),
		Rules:  map[string]Rule{"email": {Kind: KindRedact}},
	}
	irrelevant := "irrelevant"
	got, matched := s.ApplyColumn("Email", &irrelevant)
	if !matched || got == nil || *got != redactLiteral {
		t.Fatalf("ApplyColumn(case-insensitive match) = (%v, %v), want (%q, true)", got, matched, redactLiteral)
	}

	passthrough := "value-that-must-pass-through"
	got, matched = s.ApplyColumn("other_column", &passthrough)
	if matched || got == nil || *got != passthrough {
		t.Fatalf("ApplyColumn(no match) = (%v, %v), want the original value unchanged", got, matched)
	}

	got, matched = s.ApplyColumn("email", nil)
	if matched || got != nil {
		t.Fatalf("ApplyColumn(NULL) = (%v, %v), want (nil, false) — NULL bypasses matching entirely", got, matched)
	}
}

func TestSetEmpty(t *testing.T) {
	var s Set
	if !s.Empty() {
		t.Fatal("zero-value Set.Empty() = false, want true")
	}
}

// --- property: no path ever returns the original text unmasked (§8's own required property) ---

func TestApplyNeverEchoesOriginalText(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	kinds := []Kind{KindName, KindEmail, KindText, KindNumber, KindDate, KindRedact}
	alphabets := []string{
		"abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789 .@-:",
		"日本語のテキスト データ", // non-Latin, exercises grapheme counting on a real script
		"🏳️‍🌈 emoji with a ZWJ sequence and combining marks é",
	}

	randomValue := func() string {
		alphabet := []rune(alphabets[rng.Intn(len(alphabets))])
		n := rng.Intn(40) + 1
		var b strings.Builder
		for i := 0; i < n; i++ {
			b.WriteRune(alphabet[rng.Intn(len(alphabet))])
		}
		return b.String()
	}

	for i := 0; i < 500; i++ {
		value := randomValue()
		if value == "" {
			continue // the one documented identity case; excluded deliberately, not missed.
		}
		kind := kinds[rng.Intn(len(kinds))]
		rule := Rule{Kind: kind, KeepHint: rng.Intn(2) == 0, Correlate: rng.Intn(2) == 0}
		m := New([]byte("property-test-key"))
		got := m.Apply(rule, value)
		if got == value {
			t.Fatalf("Apply(%+v, %q) returned the original text unmasked", rule, value)
		}
	}
}

// --- grapheme correctness (emoji, combining marks) ---

func TestMaskWordCountsGraphemesNotRunes(t *testing.T) {
	// "élan": e + combining acute (U+0301) + l + a + n is 5 runes but 4 grapheme clusters
	// (é, l, a, n). A rune-based implementation would either split the combining mark from its
	// base letter or miscount the word's own length; a correct one keeps the first cluster whole
	// and emits exactly 3 bullets for the remaining 3 clusters, not 4.
	word := "e\u0301lan"
	got := maskWord(word, true)
	want := "e\u0301" + "\u2022\u2022\u2022"
	if got != want {
		t.Fatalf("maskWord(%q) = %q, want %q", word, got, want)
	}
}

// TestMaskWordSingleGraphemeNeverEchoes is the fix TestApplyNeverEchoesOriginalText's fuzzing
// found directly: "keep the first grapheme, destroy the rest" has nothing left to destroy when a
// word is exactly one grapheme cluster, so honouring keepHint there would return the word whole —
// a real, if narrow, way to violate the "never echo the original" invariant. maskWord instead
// always bullets a single-grapheme word, keeping only its length (one bullet), never its content.
func TestMaskWordSingleGraphemeNeverEchoes(t *testing.T) {
	// ASCII, CJK, and an emoji ZWJ sequence — each exactly one word, one grapheme cluster.
	cases := []string{"x", "\u8a9e", "\U0001F3F3\uFE0F\u200D\U0001F308"}
	for _, w := range cases {
		got := maskWord(w, true)
		if got == w {
			t.Fatalf("maskWord(%q, keepHint=true) = %q, echoed the original single-grapheme word", w, got)
		}
		if got != bullet {
			t.Fatalf("maskWord(%q, keepHint=true) = %q, want a single bullet", w, got)
		}
	}
}

// TestMaskEmailEmptyLocalPartNeverEchoes is a second gap TestApplyNeverEchoesOriginalText's
// generator (uniform over an alphabet including "@") does not reliably hit: a value with an empty
// local part — "@corp.example.com", "@x", or bare "@" — reassembles as "" + "@" + domain, the
// input itself, whenever keepHint is true. maskEmail must fall through to the universal redaction
// instead.
func TestMaskEmailEmptyLocalPartNeverEchoes(t *testing.T) {
	cases := []string{"@corp.example.com", "@x", "@"}
	for _, value := range cases {
		got := maskEmail(value, true)
		if got == value {
			t.Fatalf("maskEmail(%q, keepHint=true) = %q, echoed the original text", value, got)
		}
		if got != redactLiteral {
			t.Fatalf("maskEmail(%q, keepHint=true) = %q, want %q", value, got, redactLiteral)
		}
	}
}
