package main

import (
	"context"
	"log"
	"strings"

	cloudflare "github.com/cloudflare/cloudflare-go"
	"github.com/pkg/errors"
)

func sync(ctx context.Context, ips []string, dnsNames []string, cloudflareTTL int, cloudflareProxy bool) error {
	api, err := newCloudflareClient(options.CloudflareAPIToken, options.CloudflareAPIEmail, options.CloudflareAPIKey)
	if err != nil {
		return errors.Wrap(err, "failed to access cloudflare api")
	}

	root := dnsNames[0]
	zoneID, err := findZoneID(ctx, api, root)
	if err != nil {
		return errors.Wrapf(err, "failed to find zone id for dns-name:=%s",
			root)
	}

	known := map[string]bool{}
	for _, ip := range ips {
		known[ip] = true
	}

	for _, dnsName := range dnsNames {
		records, err := api.DNSRecords(ctx, zoneID, cloudflare.DNSRecord{Type: "A", Name: dnsName})
		if err != nil {
			return errors.Wrapf(err, "failed to list dns records for zone-id=%s name=%s",
				zoneID, dnsName)
		}

		seen := map[string]bool{}

		for _, record := range records {
			log.Printf("found existing record name=%s ip=%s\n",
				record.Name, record.Content)
			if _, ok := known[record.Content]; ok {
				seen[record.Content] = true

				isProxied := false
				if record.Proxied != nil && *record.Proxied {
					isProxied = true
				}

				if isProxied != cloudflareProxy || record.TTL != cloudflareTTL {
					log.Printf("updating dns record name=%s ip=%s\n",
						record.Name, record.Content)
					err := api.UpdateDNSRecord(ctx, zoneID, record.ID, cloudflare.DNSRecord{
						Type:    record.Type,
						Name:    record.Name,
						Content: record.Content,
						TTL:     cloudflareTTL,
						Proxied: &cloudflareProxy,
					})
					if err != nil {
						return errors.Wrapf(err, "failed to update dns record zone-id=%s record-id=%s name=%s ip=%s",
							zoneID, record.ID, record.Name, record.Content)
					}
				}
			} else {
				log.Printf("removing dns record name=%s ip=%s\n",
					record.Name, record.Content)
				err := api.DeleteDNSRecord(ctx, zoneID, record.ID)
				if err != nil {
					return errors.Wrapf(err, "failed to delete dns record zone-id=%s record-id=%s name=%s ip=%s",
						zoneID, record.ID, record.Name, record.Content)
				}
			}
		}

		for ip := range known {
			if _, ok := seen[ip]; ok {
				continue
			}
			log.Printf("adding dns record name=%s ip=%s\n",
				dnsName, ip)
			_, err := api.CreateDNSRecord(ctx, zoneID, cloudflare.DNSRecord{
				Type:    "A",
				Name:    dnsName,
				Content: ip,
				TTL:     cloudflareTTL,
				Proxied: &cloudflareProxy,
			})
			if err != nil {
				return errors.Wrapf(err, "failed to create dns record zone-id=%s name=%s ip=%s",
					zoneID, dnsName, ip)
			}
		}
	}

	return nil
}

// findZoneID finds a zone id for the given dns record
func findZoneID(ctx context.Context, api interface {
	ListZones(ctx context.Context, z ...string) ([]cloudflare.Zone, error)
}, dnsName string) (string, error) {
	zones, err := api.ListZones(ctx)
	if err != nil {
		return "", err
	}

	for _, zone := range zones {
		if zone.Name == dnsName || strings.HasSuffix(dnsName, "."+zone.Name) {
			return zone.ID, nil
		}
	}

	return "", errors.New("zone id not found")
}

func newCloudflareClient(token, email, key string) (api *cloudflare.API, err error) {
	if token != "" {
		api, err = cloudflare.NewWithAPIToken(token)
	} else {
		api, err = cloudflare.New(key, email)
	}
	return api, err
}
