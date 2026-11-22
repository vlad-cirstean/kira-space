package ade

import (
	"reflect"
	"strings"
	"testing"
)

func TestExtractLinks(t *testing.T) {
	long := "https://a.b/" + strings.Repeat("x", 2100)
	cases := []struct {
		name, body string
		want       []foundURL
	}{
		{"sentence period", "See https://a.b/c.", []foundURL{{URL: "https://a.b/c"}}},
		{"comma list", "https://a.b/1, https://a.b/2, done", []foundURL{{URL: "https://a.b/1"}, {URL: "https://a.b/2"}}},
		{"paren wrap", "(see https://a.b/c)", []foundURL{{URL: "https://a.b/c"}}},
		{"balanced parens", "https://en.wikipedia.org/wiki/Foo_(bar).", []foundURL{{URL: "https://en.wikipedia.org/wiki/Foo_(bar)"}}},
		{"markdown", "[Design doc](https://a.b/d)", []foundURL{{URL: "https://a.b/d", Title: "Design doc"}}},
		{"markdown parens in url", "[w](https://a.b/Foo_(bar))", []foundURL{{URL: "https://a.b/Foo_(bar)", Title: "w"}}},
		{"markdown title", `[T](https://a.b/d "hover")`, []foundURL{{URL: "https://a.b/d", Title: "T"}}},
		{"image", "![shot](https://a.b/i.png)", []foundURL{{URL: "https://a.b/i.png", Title: "shot"}}},
		{"autolink", "<https://a.b>", []foundURL{{URL: "https://a.b"}}},
		{"upper scheme", "HTTPS://A.B/x", []foundURL{{URL: "HTTPS://A.B/x"}}},
		{"port", "http://a.b:8080/x", []foundURL{{URL: "http://a.b:8080/x"}}},
		{"ipv6", "http://[2001:db8::1]:80/x", []foundURL{{URL: "http://[2001:db8::1]:80/x"}}},
		{"bad schemes", "javascript:alert(1) data:text/html,x mailto:a@b.c file:///etc ftp://a.b //a.b/x", nil},
		{"userinfo", "https://user:pass@a.b/x", nil},
		{"loopback", "http://localhost:3000 http://x.localhost http://127.0.0.1:3000 http://[::1]/ http://0.0.0.0/", nil},
		{"lan kept", "http://192.168.1.5/x", []foundURL{{URL: "http://192.168.1.5/x"}}},
		{"fenced", "a\n```\nhttps://a.b/in\n```\nhttps://a.b/out\n~~~\nhttps://a.b/in2\n~~~\n", []foundURL{{URL: "https://a.b/out"}}},
		{"inline code", "run `curl https://a.b/c` then https://a.b/d", []foundURL{{URL: "https://a.b/d"}}},
		{"bold", "**https://a.b/c**", []foundURL{{URL: "https://a.b/c"}}},
		{"too long", long, nil},
		{"dup", "https://a.b/c and https://a.b/c", []foundURL{{URL: "https://a.b/c"}}},
		{"markdown also bare", "[T](https://a.b/c) https://a.b/c", []foundURL{{URL: "https://a.b/c", Title: "T"}}},
		{"empty text", "[](https://a.b/c)", []foundURL{{URL: "https://a.b/c"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractLinks(c.body); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %+v want %+v", got, c.want)
			}
		})
	}
}
