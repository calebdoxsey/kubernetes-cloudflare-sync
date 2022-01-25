package main

import (
	"context"
	"testing"

	cloudflare "github.com/cloudflare/cloudflare-go"
	"github.com/stretchr/testify/assert"
)

type mockAPI struct {
	listZones func(z ...string) ([]cloudflare.Zone, error)
}

func (m mockAPI) ListZones(ctx context.Context, z ...string) ([]cloudflare.Zone, error) {
	return m.listZones(z...)
}

func TestFindZoneID(t *testing.T) {
	ctx := context.Background()
	t.Run("subdomain", func(t *testing.T) {
		zoneID, err := findZoneID(ctx, mockAPI{
			listZones: func(z ...string) ([]cloudflare.Zone, error) {
				return []cloudflare.Zone{
					{ID: "1", Name: "example.com"},
				}, nil
			},
		}, "kubernetes.example.com")
		assert.Nil(t, err)
		assert.Equal(t, "1", zoneID)
	})
	t.Run("domain", func(t *testing.T) {
		zoneID, err := findZoneID(ctx, mockAPI{
			listZones: func(z ...string) ([]cloudflare.Zone, error) {
				return []cloudflare.Zone{
					{ID: "1", Name: "example.com"},
				}, nil
			},
		}, "example.com")
		assert.Nil(t, err)
		assert.Equal(t, "1", zoneID)
	})
	t.Run("partial domain", func(t *testing.T) {
		zoneID, err := findZoneID(ctx, mockAPI{
			listZones: func(z ...string) ([]cloudflare.Zone, error) {
				return []cloudflare.Zone{
					{ID: "1", Name: "example.com"}, // a bare suffix match would inadvertently match this domain
					{ID: "2", Name: "anotherexample.com"},
				}, nil
			},
		}, "anotherexample.com")
		assert.Nil(t, err)
		assert.Equal(t, "2", zoneID)
	})
	t.Run(".co.uk", func(t *testing.T) {
		zoneID, err := findZoneID(ctx, mockAPI{
			listZones: func(z ...string) ([]cloudflare.Zone, error) {
				return []cloudflare.Zone{
					{ID: "1", Name: "example.co.uk"},
				}, nil
			},
		}, "subdomain.example.co.uk")
		assert.Nil(t, err)
		assert.Equal(t, "1", zoneID)
	})
}

func TestNewCloudflareClient(t *testing.T) {
	t.Run("token", func(t *testing.T) {
		api, err := newCloudflareClient("TEST", "", "")
		assert.NoError(t, err)
		assert.Equal(t, "TEST", api.APIToken)
	})
	t.Run("email", func(t *testing.T) {
		api, err := newCloudflareClient("", "EMAIL", "KEY")
		assert.NoError(t, err)
		assert.Equal(t, "EMAIL", api.APIEmail)
		assert.Equal(t, "KEY", api.APIKey)
	})
	t.Run("missing", func(t *testing.T) {
		_, err := newCloudflareClient("", "", "")
		assert.Error(t, err)
	})
}
