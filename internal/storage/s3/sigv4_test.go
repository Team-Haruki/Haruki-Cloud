package s3

import (
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

// Vectors copied from the AWS aws-sig-v4-test-suite (v4, 2015-08-30), as
// shipped in aws-signing-test-suite/v4/<name>/{request.txt,
// header-canonical-request.txt, header-string-to-sign.txt,
// header-signature.txt}.
const (
	vectorAccessKey = "AKIDEXAMPLE"
	vectorSecretKey = "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"
	vectorRegion    = "us-east-1"
	vectorService   = "service"
)

var vectorTime = time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC)

type sigV4Vector struct {
	name      string
	method    string
	target    string
	headers   [][2]string
	canonical string
	toSign    string
	signature string
}

var sigV4Vectors = []sigV4Vector{
	{
		name: "get-vanilla", method: "GET", target: "/",
		canonical: "GET\n/\n\nhost:example.amazonaws.com\nx-amz-date:20150830T123600Z\n\nhost;x-amz-date\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		toSign:    "AWS4-HMAC-SHA256\n20150830T123600Z\n20150830/us-east-1/service/aws4_request\nbb579772317eb040ac9ed261061d46c1f17a8133879d6129b6e1c25292927e63",
		signature: "5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31",
	},
	{
		name: "get-vanilla-query-order-key-case", method: "GET", target: "/?Param2=value2&Param1=value1",
		canonical: "GET\n/\nParam1=value1&Param2=value2\nhost:example.amazonaws.com\nx-amz-date:20150830T123600Z\n\nhost;x-amz-date\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		toSign:    "AWS4-HMAC-SHA256\n20150830T123600Z\n20150830/us-east-1/service/aws4_request\n816cd5b414d056048ba4f7c5386d6e0533120fb1fcfa93762cf0fc39e2cf19e0",
		signature: "b97d918cfa904a5beff61c982a1b6f458b799221646efd99d3219ec94cdf2500",
	},
	{
		name: "post-header-key-case", method: "POST", target: "/",
		canonical: "POST\n/\n\nhost:example.amazonaws.com\nx-amz-date:20150830T123600Z\n\nhost;x-amz-date\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		toSign:    "AWS4-HMAC-SHA256\n20150830T123600Z\n20150830/us-east-1/service/aws4_request\n553f88c9e4d10fc9e109e2aeb65f030801b70c2f6468faca261d401ae622fc87",
		signature: "5da7c1a2acd57cee7505fc6676e4e544621c30862966e37dddb68e92efbe5d6b",
	},
	{
		name: "get-header-value-trim", method: "GET", target: "/",
		headers:   [][2]string{{"My-Header1", " value1"}, {"My-Header2", ` "a   b   c"`}},
		canonical: "GET\n/\n\nhost:example.amazonaws.com\nmy-header1:value1\nmy-header2:\"a b c\"\nx-amz-date:20150830T123600Z\n\nhost;my-header1;my-header2;x-amz-date\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		toSign:    "AWS4-HMAC-SHA256\n20150830T123600Z\n20150830/us-east-1/service/aws4_request\na726db9b0df21c14f559d0a978e563112acb1b9e05476f0a6a1c7d68f28605c7",
		signature: "acc3ed3afb60bb290fc8d2dd0098b9911fcaa05412b367055dee359757a9c736",
	},
	{
		name: "get-utf8", method: "GET", target: "/%E1%88%B4",
		canonical: "GET\n/%E1%88%B4\n\nhost:example.amazonaws.com\nx-amz-date:20150830T123600Z\n\nhost;x-amz-date\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		signature: "8318018e0b0f223aa2bbf98705b62bb787dc9c0e678f255a891fd03141be5d85",
	},
	{
		name: "get-unreserved", method: "GET", target: "/-._~0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz",
		canonical: "GET\n/-._~0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz\n\nhost:example.amazonaws.com\nx-amz-date:20150830T123600Z\n\nhost;x-amz-date\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		signature: "07ef7494c76fa4850883e2b006601f940f8a34d404d0cfa977f52a65bbf5f24f",
	},
	{
		name: "get-space-normalized", method: "GET", target: "/example%20space/",
		canonical: "GET\n/example%20space/\n\nhost:example.amazonaws.com\nx-amz-date:20150830T123600Z\n\nhost;x-amz-date\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		signature: "652487583200325589f1fba4c7e578f72c47cb61beeca81406b39ddec1366741",
	},
	{
		name: "get-vanilla-utf8-query", method: "GET", target: "/?%E1%88%B4=bar",
		canonical: "GET\n/\n%E1%88%B4=bar\nhost:example.amazonaws.com\nx-amz-date:20150830T123600Z\n\nhost;x-amz-date\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		toSign:    "AWS4-HMAC-SHA256\n20150830T123600Z\n20150830/us-east-1/service/aws4_request\neb30c5bed55734080471a834cc727ae56beb50e5f39d1bff6d0d38cb192a7073",
		signature: "2cdec8eed098649ff3a119c94853b13c643bcf08f8b0a1d91e12c9027818dd04",
	},
	{
		name: "get-vanilla-query-unreserved", method: "GET",
		target:    "/?-._~0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz=-._~0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz",
		canonical: "GET\n/\n-._~0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz=-._~0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz\nhost:example.amazonaws.com\nx-amz-date:20150830T123600Z\n\nhost;x-amz-date\ne3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		signature: "9c3e54bfcdf0b19771a7f523ee5669cdf59bc7cc0884027167c21bb143a40197",
	},
}

func TestSignV4PublishedVectors(t *testing.T) {
	for _, vector := range sigV4Vectors {
		t.Run(vector.name, func(t *testing.T) {
			req, err := http.NewRequest(vector.method, "http://example.amazonaws.com"+vector.target, nil)
			if err != nil {
				t.Fatal(err)
			}
			signed := []string{"host", "x-amz-date"}
			for _, header := range vector.headers {
				req.Header.Add(header[0], header[1])
				signed = append(signed, strings.ToLower(header[0]))
			}
			req.Header.Set(headerAmzDate, vectorTime.Format(amzDateLayout))
			slices.Sort(signed)
			canonical, toSign, signature := signV4(req, signed, EmptyPayloadSHA256, vectorSecretKey, vectorRegion, vectorService, vectorTime)
			if canonical != vector.canonical {
				t.Errorf("canonical request =\n%s\nwant\n%s", canonical, vector.canonical)
			}
			if vector.toSign != "" && toSign != vector.toSign {
				t.Errorf("string to sign =\n%s\nwant\n%s", toSign, vector.toSign)
			}
			if signature != vector.signature {
				t.Errorf("signature = %s, want %s", signature, vector.signature)
			}
		})
	}
}

func TestSignHeaderVectorAuthorization(t *testing.T) {
	// header-signed-request.txt of get-vanilla, reproduced through Sign's
	// header-selection path (minus x-amz-content-sha256, which Sign always adds
	// and the generic vector does not sign).
	req, _ := http.NewRequest(http.MethodGet, "http://example.amazonaws.com/", nil)
	req.Header.Set(headerAmzDate, vectorTime.Format(amzDateLayout))
	_, _, signature := signV4(req, []string{"host", "x-amz-date"}, EmptyPayloadSHA256, vectorSecretKey, vectorRegion, vectorService, vectorTime)
	want := "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, SignedHeaders=host;x-amz-date, Signature=5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31"
	got := sigV4Algorithm + " Credential=" + vectorAccessKey + "/" + credentialScope(vectorTime, vectorRegion, vectorService) +
		", SignedHeaders=host;x-amz-date, Signature=" + signature
	if got != want {
		t.Fatalf("authorization = %s, want %s", got, want)
	}
}

func TestSignPutGolden(t *testing.T) {
	body := []byte("hello garage")
	req, _ := http.NewRequest(http.MethodPut, "http://100.64.0.11:3900/image-cache/pjsk/a%20b/c+d.png", nil)
	req.Header.Set("Content-Type", "image/png")
	req.Header.Set("Cache-Control", "public, max-age=60")
	req.Header.Set("X-Amz-Acl", "public-read")
	req.Header.Set("User-Agent", "not-signed")
	at := time.Date(2026, 9, 13, 8, 30, 0, 0, time.FixedZone("CST", 8*3600))
	Sign(req, PayloadSHA256(body), "GKtest", "secret", "garage", "s3", at)

	if req.Host != "100.64.0.11:3900" {
		t.Fatalf("Host = %q", req.Host)
	}
	if got := req.Header.Get(headerAmzDate); got != "20260913T003000Z" {
		t.Fatalf("X-Amz-Date = %q", got)
	}
	if got := req.Header.Get(headerAmzContent); got != PayloadSHA256(body) {
		t.Fatalf("X-Amz-Content-Sha256 = %q", got)
	}
	signed := signedHeaderNames(req.Header)
	canonical, _, _ := signV4(req, signed, PayloadSHA256(body), "secret", "garage", "s3", at)
	wantCanonical := "PUT\n/image-cache/pjsk/a%20b/c%2Bd.png\n\n" +
		"cache-control:public, max-age=60\ncontent-type:image/png\nhost:100.64.0.11:3900\n" +
		"x-amz-acl:public-read\nx-amz-content-sha256:" + PayloadSHA256(body) + "\nx-amz-date:20260913T003000Z\n\n" +
		"cache-control;content-type;host;x-amz-acl;x-amz-content-sha256;x-amz-date\n" + PayloadSHA256(body)
	if canonical != wantCanonical {
		t.Fatalf("canonical =\n%s\nwant\n%s", canonical, wantCanonical)
	}
	want := "AWS4-HMAC-SHA256 Credential=GKtest/20260913/garage/s3/aws4_request, " +
		"SignedHeaders=cache-control;content-type;host;x-amz-acl;x-amz-content-sha256;x-amz-date, " +
		"Signature=d6d6d73341b8b53d8f349926564648987672812e4a23e206fa6e8a3306a60cb5"
	if got := req.Header.Get(headerAuth); got != want {
		t.Fatalf("Authorization =\n%s\nwant\n%s", got, want)
	}
}

func TestSignKeepsExplicitHost(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://10.0.0.1/b/k", nil)
	req.Host = "bucket.example"
	Sign(req, EmptyPayloadSHA256, "a", "s", "garage", "s3", vectorTime)
	if req.Host != "bucket.example" {
		t.Fatalf("Host = %q", req.Host)
	}
	req.Host = ""
	if got := canonicalHeaderValue(req, "host"); got != "10.0.0.1" {
		t.Fatalf("host fallback = %q", got)
	}
}

func TestCanonicalQueryAndEscaping(t *testing.T) {
	cases := map[string]string{
		"":                           "",
		"b=2&a=1&&a=0":               "a=0&a=1&b=2",
		"prefix=a%20b%2Fc&list-type": "list-type=&prefix=a%20b%2Fc",
		"k=a+b":                      "k=a%2Bb",
		"bad=%zz&tail=%4":            "bad=%25zz&tail=%254",
	}
	for raw, want := range cases {
		if got := canonicalQuery(raw); got != want {
			t.Errorf("canonicalQuery(%q) = %q, want %q", raw, got, want)
		}
	}
	if got := canonicalURI(""); got != "/" {
		t.Errorf("canonicalURI empty = %q", got)
	}
	if got := encodePath("/a b/ü+~"); got != "/a%20b/%C3%BC%2B~" {
		t.Errorf("encodePath = %q", got)
	}
	if got := encodeQueryComponent("a/b c"); got != "a%2Fb%20c" {
		t.Errorf("encodeQueryComponent = %q", got)
	}
}
