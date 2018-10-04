package main

import (
	"flag"
	"log"
	"os"
	"sort"
	"strings"
	"time"
	"strconv"

	cloudflare "github.com/cloudflare/cloudflare-go"
	"github.com/pkg/errors"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

var options = struct {
	CloudflareAPIEmail string
	CloudflareAPIKey   string
	CloudflareProxy    string
	CloudflareTTL	   string
	DNSName            string
}{
	CloudflareAPIEmail: os.Getenv("CF_API_EMAIL"),
	CloudflareAPIKey:   os.Getenv("CF_API_KEY"),
	CloudflareProxy:    os.Getenv("CF_PROXY"),
	CloudflareTTL:      os.Getenv("CF_TTL"),
	DNSName:            os.Getenv("DNS_NAME"),
}

func main() {
	flag.StringVar(&options.DNSName, "dns-name", options.DNSName, "the dns name for the nodes, comma-separated for multiple (same root)")
	flag.StringVar(&options.CloudflareAPIEmail, "cloudflare-api-email", options.CloudflareAPIEmail, "the email address to use for cloudflare")
	flag.StringVar(&options.CloudflareAPIKey, "cloudflare-api-key", options.CloudflareAPIKey, "the key to use for cloudflare")
	flag.StringVar(&options.CloudflareProxy, "cloudflare-proxy", options.CloudflareProxy, "enable cloudflare proxy on dns (default false)")
	flag.StringVar(&options.CloudflareTTL, "cloudflare-ttl", options.CloudflareTTL, "ttl for dns (default 120)")
	flag.Parse()

	if options.CloudflareAPIEmail == "" {
		flag.Usage()
		log.Fatalln("cloudflare api email is required")
	}
	if options.CloudflareAPIKey == "" {
		flag.Usage()
		log.Fatalln("cloudflare api key is required")
	}

	dnsNames := strings.Split(options.DNSName, ",")
	if len(dnsNames) == 1 && dnsNames[0] == "" {
		flag.Usage()
		log.Fatalln("dns name is required")
	}

	cloudflareProxy, err := strconv.ParseBool(options.CloudflareProxy)
	if err != nil {
		log.Println("CloudflareProxy config not found or incorrect, defaulting to false")
		cloudflareProxy = false
	}

	cloudflareTTL, err := strconv.Atoi(options.CloudflareTTL)
	if err != nil {
		log.Println("CloudflareTTL config not found or incorrect, defaulting to 120")
		cloudflareTTL = 120
	}

	cfg, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalln(err)
	}

	client, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		log.Fatalln(err)
	}

	stop := make(chan struct{})
	defer close(stop)

	factory := informers.NewSharedInformerFactory(client, time.Minute)
	lister := factory.Core().V1().Nodes().Lister()
	var lastIPs []string
	resync := func() {
		log.Println("resyncing")
		nodes, err := lister.List(labels.NewSelector())
		if err != nil {
			log.Fatalln("failed to list nodes", err)
		}

		var ips []string
		for _, node := range nodes {
			for _, addr := range node.Status.Addresses {
				if addr.Type == core_v1.NodeExternalIP {
					ips = append(ips, addr.Address)
				}
			}
		}
		sort.Strings(ips)
		log.Println("ips:", ips)
		if strings.Join(ips, ",") == strings.Join(lastIPs, ",") {
			log.Println("no change detected")
			return
		}
		lastIPs = ips

		err = sync(ips, dnsNames, cloudflareTTL, cloudflareProxy)
		if err != nil {
			log.Fatalln("failed to sync", err)
		}
	}

	informer := factory.Core().V1().Nodes().Informer()
	informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			resync()
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			resync()
		},
		DeleteFunc: func(obj interface{}) {
			resync()
		},
	})
	informer.Run(stop)

	select {}
}

func sync(ips []string, dnsNames []string, cloudflareTTL int, cloudflareProxy bool) error {
	api, err := cloudflare.New(options.CloudflareAPIKey, options.CloudflareAPIEmail)
	if err != nil {
		return errors.Wrap(err, "failed to access cloudflare api")
	}

	root := dnsNames[0]
	for strings.Count(root, ".") > 1 {
		root = root[strings.Index(root, ".")+1:]
	}

	zoneID, err := api.ZoneIDByName(root)
	if err != nil {
		return errors.Wrapf(err, "failed to find zone id for zone-name:=%s",
			root)
	}

	known := map[string]bool{}
	for _, ip := range ips {
		known[ip] = true
	}

	for _, dnsName := range dnsNames {
		records, err := api.DNSRecords(zoneID, cloudflare.DNSRecord{Type: "A", Name: dnsName})
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

				if record.Proxied != cloudflareProxy || record.TTL != cloudflareTTL {
					log.Printf("updating dns record name=%s ip=%s\n",
						record.Name, record.Content)
					err := api.UpdateDNSRecord(zoneID, record.ID, cloudflare.DNSRecord{
						Type:    record.Type,
						Name:    record.Name,
						Content: record.Content,
						TTL:     cloudflareTTL,
						Proxied: cloudflareProxy,
					})
					if err != nil {
						return errors.Wrapf(err, "failed to update dns record zone-id=%s record-id=%s name=%s ip=%s",
							zoneID, record.ID, record.Name, record.Content)
					}
				}
			} else {
				log.Printf("removing dns record name=%s ip=%s\n",
					record.Name, record.Content)
				err := api.DeleteDNSRecord(zoneID, record.ID)
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
			_, err := api.CreateDNSRecord(zoneID, cloudflare.DNSRecord{
				Type:    "A",
				Name:    dnsName,
				Content: ip,
				TTL:     cloudflareTTL,
				Proxied: cloudflareProxy,
			})
			if err != nil {
				return errors.Wrapf(err, "failed to create dns record zone-id=%s name=%s ip=%s",
					zoneID, dnsName, ip)
			}
		}
	}

	return nil
}
