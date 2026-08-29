package scraper

import (
	"bytes"
	"log/slog"
	"net"
	"net/http"
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
var amazonPageURL = mustParsePageURL("https://www.amazon.fr/dp/B0EXAMPLE")

func mustPageURL() *url.URL {
	return mustParsePageURL("https://shop.example.com/products/widget")
}

func mustParsePageURL(raw string) *url.URL {
	u, err := url.Parse(raw)
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

func TestIsAmazonHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"www.amazon.fr", true},
		{"amazon.com", true},
		{"amazon.co.uk", true},
		{"smile.amazon.de", true},
		{"amzn.to", true},
		{"amzn.eu", true},
		{"www.amzn.to", true},
		{"shop.example.com", false},
		{"notamazon.com", false},
		{"amazon-fake.com", false},
	}
	for _, tc := range cases {
		if got := isAmazonHost(tc.host); got != tc.want {
			t.Errorf("isAmazonHost(%q) = %v, want %v", tc.host, got, tc.want)
		}
	}
}

func TestExtractImageAmazonPrefersDataOldHires(t *testing.T) {
	html := `<html><head>
		<meta property="og:image" content="https://images-eu.ssl-images-amazon.com/images/G/amazon-logo.png">
	</head><body>
		<img id="landingImage" src="https://images-eu.ssl-images-amazon.com/images/I/placeholder.jpg"
			data-old-hires="https://images-eu.ssl-images-amazon.com/images/I/71RealProduct._SL1500_.jpg">
	</body></html>`

	info, err := extractProductInfo(strings.NewReader(html), amazonPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.ImageURL != "https://images-eu.ssl-images-amazon.com/images/I/71RealProduct._SL1500_.jpg" {
		t.Fatalf("expected data-old-hires image, got %q", info.ImageURL)
	}
}

func TestExtractImageAmazonDynamicImagePicksLargest(t *testing.T) {
	html := `<html><body>
		<img id="imgBlkFront" data-a-dynamic-image='{"https://m.media-amazon.com/images/I/small.jpg":[300,300],"https://m.media-amazon.com/images/I/large.jpg":[1500,1500]}'>
	</body></html>`

	info, err := extractProductInfo(strings.NewReader(html), amazonPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.ImageURL != "https://m.media-amazon.com/images/I/large.jpg" {
		t.Fatalf("expected the largest dynamic image, got %q", info.ImageURL)
	}
}

func TestExtractImageAmazonSkipsGenericOgImage(t *testing.T) {
	html := `<html><head>
		<meta property="og:image" content="https://images-eu.ssl-images-amazon.com/images/G/amazon-logo.png">
	</head><body></body></html>`

	info, err := extractProductInfo(strings.NewReader(html), amazonPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.ImageURL != "" {
		t.Fatalf("expected the generic Amazon logo image to be rejected, got %q", info.ImageURL)
	}
}

func TestExtractImageAmazonFallsBackToOgImageWhenNotGeneric(t *testing.T) {
	html := `<html><head>
		<meta property="og:image" content="https://m.media-amazon.com/images/I/71RealProduct._SL1500_.jpg">
	</head><body></body></html>`

	info, err := extractProductInfo(strings.NewReader(html), amazonPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.ImageURL != "https://m.media-amazon.com/images/I/71RealProduct._SL1500_.jpg" {
		t.Fatalf("expected the og:image fallback, got %q", info.ImageURL)
	}
}

func TestExtractPriceAmazonApexPriceToPay(t *testing.T) {
	html := `<html><body>
		<span class="a-price aok-align-center apexPriceToPay" data-a-size="xl">
			<span class="a-offscreen">14,99&nbsp;€</span>
			<span aria-hidden="true"><span class="a-price-whole">14</span></span>
		</span>
	</body></html>`

	info, err := extractProductInfo(strings.NewReader(html), amazonPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil || *info.Price != 14.99 {
		t.Fatalf("expected 14.99, got %v", info.Price)
	}
}

func TestExtractPriceAmazonCorePriceDesktop(t *testing.T) {
	html := `<html><body>
		<div id="corePrice_desktop">
			<table>
				<tr><td class="a-span12">
					<span class="a-price">
						<span class="a-offscreen">27,50 €</span>
					</span>
				</td></tr>
			</table>
		</div>
	</body></html>`

	info, err := extractProductInfo(strings.NewReader(html), amazonPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil || *info.Price != 27.50 {
		t.Fatalf("expected 27.50, got %v", info.Price)
	}
}

func TestExtractPriceAmazonAPriceOffscreenFallback(t *testing.T) {
	html := `<html><body>
		<span class="a-price">
			<span class="a-offscreen">9,90 €</span>
		</span>
	</body></html>`

	info, err := extractProductInfo(strings.NewReader(html), amazonPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil || *info.Price != 9.90 {
		t.Fatalf("expected 9.90, got %v", info.Price)
	}
}

func TestExtractPriceAmazonPriceblockID(t *testing.T) {
	html := `<html><body>
		<span id="priceblock_ourprice">33,00 €</span>
	</body></html>`

	info, err := extractProductInfo(strings.NewReader(html), amazonPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil || *info.Price != 33.00 {
		t.Fatalf("expected 33.00, got %v", info.Price)
	}
}

func TestExtractPriceAmazonColorPriceClass(t *testing.T) {
	html := `<html><body>
		<span class="a-color-price">5,49 €</span>
	</body></html>`

	info, err := extractProductInfo(strings.NewReader(html), amazonPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil || *info.Price != 5.49 {
		t.Fatalf("expected 5.49, got %v", info.Price)
	}
}

func TestExtractPriceAmazonBuyboxID(t *testing.T) {
	html := `<html><body>
		<div id="price_inside_buybox">18,20 €</div>
	</body></html>`

	info, err := extractProductInfo(strings.NewReader(html), amazonPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil || *info.Price != 18.20 {
		t.Fatalf("expected 18.20, got %v", info.Price)
	}
}

func TestExtractPriceAmazonSelectorPriorityOverJSONLD(t *testing.T) {
	// Amazon selectors must win over JSON-LD/OpenGraph, since Amazon pages
	// frequently ship a stale or entirely absent JSON-LD price relative to
	// the actually-displayed one.
	html := `<html><head>
		<meta property="og:price:amount" content="99.99">
		<script type="application/ld+json">
		{"@type": "Product", "offers": {"@type": "Offer", "price": "50.00"}}
		</script>
	</head><body>
		<span class="a-price apexPriceToPay">
			<span class="a-offscreen">14,99 €</span>
		</span>
	</body></html>`

	info, err := extractProductInfo(strings.NewReader(html), amazonPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil || *info.Price != 14.99 {
		t.Fatalf("expected the Amazon selector price 14.99 to win, got %v", info.Price)
	}
}

func TestExtractPriceAmazonFallsBackToJSONLDWhenNoSelectorMatches(t *testing.T) {
	html := `<html><head>
		<script type="application/ld+json">
		{"@type": "Product", "offers": {"@type": "Offer", "price": "50.00"}}
		</script>
	</head><body></body></html>`

	info, err := extractProductInfo(strings.NewReader(html), amazonPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil || *info.Price != 50.00 {
		t.Fatalf("expected the JSON-LD fallback 50.00, got %v", info.Price)
	}
}

func TestExtractPriceAmazonHighPriceFallback(t *testing.T) {
	html := `<html><head>
		<script type="application/ld+json">
		{"@type": "Product", "offers": {"@type": "AggregateOffer", "highPrice": "22.10", "lowPrice": "19.90"}}
		</script>
	</head><body></body></html>`

	info, err := extractProductInfo(strings.NewReader(html), amazonPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil {
		t.Fatal("expected a price, got nil")
	}
}

func TestExtractPriceAmazonDataAttributeFallback(t *testing.T) {
	html := `<html><body>
		<div data-asin-price="12.40"></div>
	</body></html>`

	info, err := extractProductInfo(strings.NewReader(html), amazonPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil || *info.Price != 12.40 {
		t.Fatalf("expected 12.40, got %v", info.Price)
	}
}

func TestExtractImageMicrodata(t *testing.T) {
	html := `<html><body>
		<img itemprop="image" src="/img/widget.jpg">
	</body></html>`

	info, err := extractProductInfo(strings.NewReader(html), testPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.ImageURL != "https://shop.example.com/img/widget.jpg" {
		t.Fatalf("expected the microdata image URL, got %q", info.ImageURL)
	}
}

func TestExtractPriceMicrodataMetaContentAttr(t *testing.T) {
	html := `<html><head>
		<meta itemprop="price" content="12.34">
	</head><body></body></html>`

	info, err := extractProductInfo(strings.NewReader(html), testPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil || *info.Price != 12.34 {
		t.Fatalf("expected 12.34, got %v", info.Price)
	}
}

func TestExtractPriceTwitterCard(t *testing.T) {
	html := `<html><head>
		<meta name="twitter:label1" content="Price">
		<meta name="twitter:data1" content="6,12 €">
	</head><body></body></html>`

	info, err := extractProductInfo(strings.NewReader(html), testPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil || *info.Price != 6.12 {
		t.Fatalf("expected 6.12, got %v", info.Price)
	}
}

func TestExtractPricePriorityJSONLDOverOpenGraph(t *testing.T) {
	html := `<html><head>
		<meta property="og:price:amount" content="99.99">
		<script type="application/ld+json">
		{"@type": "Product", "offers": {"@type": "Offer", "price": "15.90"}}
		</script>
	</head><body></body></html>`

	info, err := extractProductInfo(strings.NewReader(html), testPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil || *info.Price != 15.90 {
		t.Fatalf("expected JSON-LD price 15.90 to win over og:price:amount, got %v", info.Price)
	}
}

func TestExtractTitleJSONLDPrefersProductType(t *testing.T) {
	html := `<html><head>
		<script type="application/ld+json">
		{"@context": "https://schema.org", "@type": "WebSite", "name": "Shop Example"}
		</script>
		<script type="application/ld+json">
		{"@context": "https://schema.org", "@type": "Product", "name": "Widget Deluxe"}
		</script>
	</head><body></body></html>`

	info, err := extractProductInfo(strings.NewReader(html), testPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Title != "Widget Deluxe" {
		t.Fatalf("expected the Product's own name, got %q", info.Title)
	}
}

func TestExtractTitleOpenGraphFallback(t *testing.T) {
	html := `<html><head>
		<meta property="og:title" content="Widget Deluxe">
	</head><body></body></html>`

	info, err := extractProductInfo(strings.NewReader(html), testPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Title != "Widget Deluxe" {
		t.Fatalf("expected the og:title fallback, got %q", info.Title)
	}
}

func TestExtractTitlePageTitleFallback(t *testing.T) {
	html := `<html><head>
		<title>Widget Deluxe - Shop Example</title>
	</head><body></body></html>`

	info, err := extractProductInfo(strings.NewReader(html), testPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Title != "Widget Deluxe - Shop Example" {
		t.Fatalf("expected the <title> fallback, got %q", info.Title)
	}
}

func TestSetBrowserHeaders(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://www.amazon.fr/dp/B0EXAMPLE", nil)
	if err != nil {
		t.Fatalf("http.NewRequest returned error: %v", err)
	}
	setBrowserHeaders(req)

	want := map[string]string{
		"User-Agent":         userAgent,
		"Accept":             acceptHeader,
		"Accept-Language":    acceptLanguage,
		"Device-Memory":      "8",
		"Sec-Ch-Ua":          secChUa,
		"Sec-Ch-Ua-Mobile":   "?0",
		"Sec-Ch-Ua-Platform": `"Windows"`,
		"Sec-Fetch-Dest":     "document",
		"Sec-Fetch-Mode":     "navigate",
		"Sec-Fetch-Site":     "none",
		"Sec-Fetch-User":     "?1",
	}
	for header, expected := range want {
		if got := req.Header.Get(header); got != expected {
			t.Errorf("header %q = %q, want %q", header, got, expected)
		}
	}
	if !strings.Contains(userAgent, "Chrome/124.") {
		t.Errorf("expected userAgent's Chrome version to match secChUa's (\"124\"), got %q", userAgent)
	}
}

// TestExtractPriceAmazonScriptState covers a page whose price is only
// present in an inline JS state script (e.g. a <script type="a-state"
// data-a-state='{"key":"..."}'> widget payload) rather than in any of the
// static DOM selectors or JSON-LD block — the scenario that motivated this
// fallback tier, since Amazon sometimes renders the buy-box price this way.
func TestExtractPriceAmazonScriptState(t *testing.T) {
	html := `<html><body>
		<script type="a-state" data-a-state='{"key":"desktop-buybox_feature_div"}'>
			{"asin":"B0EXAMPLE","priceAmount":23.45,"currencySymbol":"€"}
		</script>
	</body></html>`

	info, err := extractProductInfo(strings.NewReader(html), amazonPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil || *info.Price != 23.45 {
		t.Fatalf("expected 23.45 from the inline JS state script, got %v", info.Price)
	}
}

func TestExtractPriceAmazonScriptStateDisplayPrice(t *testing.T) {
	html := `<html><body>
		<script>window.P.register('data', function(){return {"displayPrice":"14,99 €","buyingPrice":14.99}});</script>
	</body></html>`

	info, err := extractProductInfo(strings.NewReader(html), amazonPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil || *info.Price != 14.99 {
		t.Fatalf("expected 14.99, got %v", info.Price)
	}
}

func TestExtractPriceAmazonScriptStateOnlyOnAmazonHost(t *testing.T) {
	// The same script content on a non-Amazon page must never be scanned
	// for a price this way — amazonScriptStatePrice is Amazon-only.
	html := `<html><body>
		<script>{"priceAmount":23.45}</script>
	</body></html>`

	info, err := extractProductInfo(strings.NewReader(html), testPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price != nil {
		t.Fatalf("expected no price on a non-Amazon host, got %v", *info.Price)
	}
}

// TestExtractPriceAmazonRawBodyLastResort exercises the absolute
// last-resort raw-body regex: the price text is placed inside an HTML
// comment, so it never becomes part of the parsed DOM tree (no element, no
// script, nothing extractProductInfo's structured sources can see) but is
// still present in the raw response bytes.
func TestExtractPriceAmazonRawBodyLastResort(t *testing.T) {
	html := `<html><body>
		<!-- fallback rendering: <span class="a-offscreen">19,99 €</span> -->
	</body></html>`

	info, err := extractProductInfo(strings.NewReader(html), amazonPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil || *info.Price != 19.99 {
		t.Fatalf("expected 19.99 from the raw-body last resort, got %v", info.Price)
	}
}

func TestExtractPriceAmazonRawBodyPriceAmountPattern(t *testing.T) {
	html := `<html><body>
		<!-- "priceAmount": 27.5 -->
	</body></html>`

	info, err := extractProductInfo(strings.NewReader(html), amazonPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil || *info.Price != 27.5 {
		t.Fatalf("expected 27.5, got %v", info.Price)
	}
}

func TestExtractPriceAmazonRawBodyNotUsedOnNonAmazonHost(t *testing.T) {
	html := `<html><body>
		<!-- "priceAmount": 27.5 -->
	</body></html>`

	info, err := extractProductInfo(strings.NewReader(html), testPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price != nil {
		t.Fatalf("expected no price on a non-Amazon host, got %v", *info.Price)
	}
}

// TestExtractPriceAmazonDOMSelectorBeatsScriptStateAndRawBody confirms the
// overall priority order still holds when several tiers could all match:
// the DOM selector (highest priority) must win over both the script-state
// and raw-body fallbacks even though all three are present.
func TestExtractPriceAmazonDOMSelectorBeatsScriptStateAndRawBody(t *testing.T) {
	html := `<html><body>
		<span class="a-price apexPriceToPay"><span class="a-offscreen">1,11 €</span></span>
		<script>{"priceAmount":2.22}</script>
		<!-- "priceAmount": 3.33 -->
	</body></html>`

	info, err := extractProductInfo(strings.NewReader(html), amazonPageURL)
	if err != nil {
		t.Fatalf("extractProductInfo returned error: %v", err)
	}
	if info.Price == nil || *info.Price != 1.11 {
		t.Fatalf("expected the DOM selector's 1.11 to win, got %v", info.Price)
	}
}

func TestLogIfAmazonBlockedLogsOnCaptchaMarker(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	logIfAmazonBlocked(logger, "https://www.amazon.fr/dp/B0EXAMPLE", true, []byte("Type the characters you see in this image"))

	if !strings.Contains(buf.String(), "blocked") {
		t.Fatalf("expected a debug log mentioning the block, got %q", buf.String())
	}
}

func TestLogIfAmazonBlockedSilentWhenNotBlocked(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	logIfAmazonBlocked(logger, "https://www.amazon.fr/dp/B0EXAMPLE", true, []byte("<html><body>ordinary product page</body></html>"))

	if buf.Len() != 0 {
		t.Fatalf("expected no log output, got %q", buf.String())
	}
}

func TestLogIfAmazonBlockedIgnoresNonAmazonHost(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	logIfAmazonBlocked(logger, "https://shop.example.com/product", false, []byte("Server Busy"))

	if buf.Len() != 0 {
		t.Fatalf("expected no log output for a non-Amazon host, got %q", buf.String())
	}
}

func TestLogIfAmazonBlockedNilLoggerNoPanic(t *testing.T) {
	logIfAmazonBlocked(nil, "https://www.amazon.fr/dp/B0EXAMPLE", true, []byte("Server Busy"))
}
