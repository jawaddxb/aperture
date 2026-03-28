package profiles

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewYAMLProfileManager(t *testing.T) {
	mgr, err := NewYAMLProfileManager()
	require.NoError(t, err)
	require.NotNil(t, mgr)
	assert.GreaterOrEqual(t, len(mgr.Profiles()), 3, "should load at least 3 profiles")
}

func TestMatch_AmazonProductDetail(t *testing.T) {
	mgr, err := NewYAMLProfileManager()
	require.NoError(t, err)

	match := mgr.Match("https://www.amazon.com/dp/B0D1XD1ZV3")
	require.NotNil(t, match)
	assert.Equal(t, "*.amazon.com", match.ProfileDomain)
	assert.Equal(t, "product_detail", match.PageType)
	assert.NotNil(t, match.Profile)
}

func TestMatch_AmazonSearchResults(t *testing.T) {
	mgr, err := NewYAMLProfileManager()
	require.NoError(t, err)

	match := mgr.Match("https://www.amazon.com/s?k=laptop")
	require.NotNil(t, match)
	assert.Equal(t, "search_results", match.PageType)
}

func TestMatch_GoogleSearch(t *testing.T) {
	mgr, err := NewYAMLProfileManager()
	require.NoError(t, err)

	match := mgr.Match("https://www.google.com/search?q=test")
	require.NotNil(t, match)
	assert.Equal(t, "*.google.com", match.ProfileDomain)
	assert.Equal(t, "search", match.PageType)
}

func TestMatch_ShopifyProduct(t *testing.T) {
	mgr, err := NewYAMLProfileManager()
	require.NoError(t, err)

	match := mgr.Match("https://store.myshopify.com/products/cool-hat")
	require.NotNil(t, match)
	assert.Equal(t, "*.myshopify.com", match.ProfileDomain)
	assert.Equal(t, "product", match.PageType)
}

func TestMatch_NoMatch(t *testing.T) {
	mgr, err := NewYAMLProfileManager()
	require.NoError(t, err)

	match := mgr.Match("https://www.example.com/page")
	assert.Nil(t, match)
}

func TestAvailableActions_Amazon(t *testing.T) {
	mgr, err := NewYAMLProfileManager()
	require.NoError(t, err)

	match := mgr.Match("https://www.amazon.com/dp/B0D1XD1ZV3")
	require.NotNil(t, match)

	actions := mgr.AvailableActions(match)
	assert.Equal(t, []string{"add_to_cart", "buy_now"}, actions)
}

func TestAvailableActions_NilMatch(t *testing.T) {
	mgr, err := NewYAMLProfileManager()
	require.NoError(t, err)

	actions := mgr.AvailableActions(nil)
	assert.Nil(t, actions)
}

func TestNoopProfileManager(t *testing.T) {
	noop := NewNoopProfileManager()
	assert.Nil(t, noop.Match("https://example.com"))
	assert.Nil(t, noop.AvailableActions(nil))
	assert.Nil(t, noop.Profiles())
}

func TestConvertValue(t *testing.T) {
	tests := []struct {
		raw  string
		typ  string
		want interface{}
	}{
		{"$29.99", "currency", 29.99},
		{"€1,299.00", "currency", 1299.0},
		{"4.5 out of 5 stars", "rating", 4.5},
		{"1,234", "number", int64(1234)},
		{"hello world", "string", "hello world"},
		{"true", "boolean", true},
		{"no", "boolean", false},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got := convertValue(tt.raw, tt.typ)
			assert.Equal(t, tt.want, got)
		})
	}
}
