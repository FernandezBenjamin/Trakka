package scraper

import (
	"net"
	"net/url"
	"strings"
	"testing"
)

func mustParseIP(t *testing.T, s string) net.IP {
	t.Helper()
	ip := net.ParseIP(s)
	if ip == nil {
		t.Fatalf("net.ParseIP(%q) returned nil", s)
	}
	return ip
}

var testPageURL = mustPageURL()

func mustPageURL() *url.URL {
	u, err := url.Parse("https://shop.example.com/products/widget")
	if err != nil {
		panic(err)
	}
	return u
}

func TestExtractPriceOpenGraph(t *testing.T) {
	html := `<html><head>
		<meta property="og:title" content="Widget">
		<meta property="og:price:amount" content="19.99">
		<meta property="og:price:currency" content="EUR">
	</head><body></body></html>`

	info, err := extractProductInfo(strings.NewReader(html), testPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil {
		t.Fatal("expected a price, got nil")
	}
	if *info.Price != 19.99 {
		t.Fatalf("expected 19.99, got %v", *info.Price)
	}
}

func TestExtractPriceJSONLD(t *testing.T) {
	html := `<html><head>
		<script type="application/ld+json">
		{
			"@context": "https://schema.org/",
			"@type": "Product",
			"name": "Widget",
			"offers": {
				"@type": "Offer",
				"priceCurrency": "EUR",
				"price": "42.50"
			}
		}
		</script>
	</head><body></body></html>`

	info, err := extractProductInfo(strings.NewReader(html), testPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil || *info.Price != 42.50 {
		t.Fatalf("expected 42.50, got %v", info.Price)
	}
}

func TestExtractPriceMicrodataContentAttr(t *testing.T) {
	html := `<html><body>
		<div itemscope itemtype="https://schema.org/Product">
			<span itemprop="price" content="7.25">7,25&nbsp;€</span>
		</div>
	</body></html>`

	info, err := extractProductInfo(strings.NewReader(html), testPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil || *info.Price != 7.25 {
		t.Fatalf("expected 7.25, got %v", info.Price)
	}
}

func TestExtractPriceMicrodataText(t *testing.T) {
	html := `<html><body>
		<span itemprop="price">1 234,56 €</span>
	</body></html>`

	info, err := extractProductInfo(strings.NewReader(html), testPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil || *info.Price != 1234.56 {
		t.Fatalf("expected 1234.56, got %v", info.Price)
	}
}

func TestExtractPriceNoneFound(t *testing.T) {
	html := `<html><body><p>Rien à voir ici.</p></body></html>`

	info, err := extractProductInfo(strings.NewReader(html), testPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price != nil {
		t.Fatalf("expected no price, got %v", *info.Price)
	}
	if info.ImageURL != "" {
		t.Fatalf("expected no image, got %v", info.ImageURL)
	}
}

func TestExtractImageOpenGraph(t *testing.T) {
	html := `<html><head>
		<meta property="og:title" content="Widget">
		<meta property="og:image" content="https://cdn.example.com/widget.jpg">
	</head><body></body></html>`

	info, err := extractProductInfo(strings.NewReader(html), testPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.ImageURL != "https://cdn.example.com/widget.jpg" {
		t.Fatalf("expected the og:image URL, got %q", info.ImageURL)
	}
}

func TestExtractImageOpenGraphRelative(t *testing.T) {
	html := `<html><head>
		<meta property="og:image" content="/images/widget.jpg">
	</head><body></body></html>`

	info, err := extractProductInfo(strings.NewReader(html), testPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.ImageURL != "https://shop.example.com/images/widget.jpg" {
		t.Fatalf("expected the resolved absolute URL, got %q", info.ImageURL)
	}
}

func TestExtractImageJSONLDString(t *testing.T) {
	html := `<html><head>
		<script type="application/ld+json">
		{
			"@context": "https://schema.org/",
			"@type": "Product",
			"name": "Widget",
			"image": "https://cdn.example.com/from-jsonld.jpg"
		}
		</script>
	</head><body></body></html>`

	info, err := extractProductInfo(strings.NewReader(html), testPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.ImageURL != "https://cdn.example.com/from-jsonld.jpg" {
		t.Fatalf("expected the JSON-LD image URL, got %q", info.ImageURL)
	}
}

func TestExtractImageJSONLDArray(t *testing.T) {
	html := `<html><head>
		<script type="application/ld+json">
		{
			"@type": "Product",
			"image": ["https://cdn.example.com/first.jpg", "https://cdn.example.com/second.jpg"]
		}
		</script>
	</head><body></body></html>`

	info, err := extractProductInfo(strings.NewReader(html), testPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.ImageURL != "https://cdn.example.com/first.jpg" {
		t.Fatalf("expected the first array entry, got %q", info.ImageURL)
	}
}

func TestExtractImageTwitterCard(t *testing.T) {
	html := `<html><head>
		<meta name="twitter:card" content="summary_large_image">
		<meta name="twitter:image" content="https://cdn.example.com/twitter.jpg">
	</head><body></body></html>`

	info, err := extractProductInfo(strings.NewReader(html), testPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.ImageURL != "https://cdn.example.com/twitter.jpg" {
		t.Fatalf("expected the twitter:image URL, got %q", info.ImageURL)
	}
}

func TestExtractImagePriorityOpenGraphOverTwitter(t *testing.T) {
	html := `<html><head>
		<meta property="og:image" content="https://cdn.example.com/og.jpg">
		<meta name="twitter:image" content="https://cdn.example.com/twitter.jpg">
	</head><body></body></html>`

	info, err := extractProductInfo(strings.NewReader(html), testPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.ImageURL != "https://cdn.example.com/og.jpg" {
		t.Fatalf("expected og:image to win over twitter:image, got %q", info.ImageURL)
	}
}

func TestExtractImageRejectsUnsafeScheme(t *testing.T) {
	html := `<html><head>
		<meta property="og:image" content="javascript:alert(1)">
	</head><body></body></html>`

	info, err := extractProductInfo(strings.NewReader(html), testPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.ImageURL != "" {
		t.Fatalf("expected a javascript: image URL to be rejected, got %q", info.ImageURL)
	}
}

func TestResolveImageURL(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
		ok   bool
	}{
		{"absolute https", "https://cdn.example.com/a.jpg", "https://cdn.example.com/a.jpg", true},
		{"protocol-relative", "//cdn.example.com/a.jpg", "https://cdn.example.com/a.jpg", true},
		{"root-relative", "/img/a.jpg", "https://shop.example.com/img/a.jpg", true},
		{"empty", "", "", false},
		{"javascript scheme", "javascript:alert(1)", "", false},
		{"data scheme", "data:image/png;base64,AAAA", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := resolveImageURL(testPageURL, tc.raw)
			if ok != tc.ok {
				t.Fatalf("resolveImageURL(%q) ok = %v, want %v", tc.raw, ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("resolveImageURL(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestParsePriceString(t *testing.T) {
	cases := []struct {
		in   string
		want float64
		ok   bool
	}{
		{"19.99", 19.99, true},
		{"19,99", 19.99, true},
		{"1,234.56", 1234.56, true},
		{"1.234,56", 1234.56, true},
		{"€19,99", 19.99, true},
		{"1 299", 1299, true},
		{"", 0, false},
		{"free", 0, false},
	}
	for _, tc := range cases {
		got, ok := parsePriceString(tc.in)
		if ok != tc.ok {
			t.Errorf("parsePriceString(%q) ok = %v, want %v", tc.in, ok, tc.ok)
			continue
		}
		if ok && got != tc.want {
			t.Errorf("parsePriceString(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestIsPublicIP(t *testing.T) {
	blocked := []string{"127.0.0.1", "10.0.0.5", "192.168.1.1", "169.254.169.254", "::1", "fc00::1"}
	for _, ip := range blocked {
		if isPublicIP(mustParseIP(t, ip)) {
			t.Errorf("expected %s to be blocked", ip)
		}
	}

	allowed := []string{"8.8.8.8", "1.1.1.1"}
	for _, ip := range allowed {
		if !isPublicIP(mustParseIP(t, ip)) {
			t.Errorf("expected %s to be allowed", ip)
		}
	}
}
