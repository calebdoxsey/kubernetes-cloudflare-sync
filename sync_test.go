package main

import (
	"testing"

	cloudflare "github.com/cloudflare/cloudflare-go"
	"github.com/stretchr/testify/assert"
)

type mockAPI struct {
	listZones func(z ...string) ([]cloudflare.Zone, error)
}

func (m mockAPI) ListZones(z ...string) ([]cloudflare.Zone, error) {
	return m.listZones(z...)
}

func TestFindZoneID(t *testing.T) {
	t.Run("subdomain", func(t *testing.T) {
		zoneID, err := findZoneID(mockAPI{
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
		zoneID, err := findZoneID(mockAPI{
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
		zoneID, err := findZoneID(mockAPI{
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
		zoneID, err := findZoneID(mockAPI{
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
