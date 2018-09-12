package main

import (
	"flag"
	"log"
	"os"
	"sort"
	"strings"
	"time"

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
	DNSName            string
}{
	CloudflareAPIEmail: os.Getenv("CF_API_EMAIL"),
	CloudflareAPIKey:   os.Getenv("CF_API_KEY"),
	DNSName:            os.Getenv("DNS_NAME"),
}

func main() {
	flag.StringVar(&options.DNSName, "dns-name", options.DNSName, "the dns name to use for the nodes")
	flag.StringVar(&options.CloudflareAPIEmail, "cloudflare-api-email", options.CloudflareAPIEmail, "the email address to use for cloudflare")
	flag.StringVar(&options.CloudflareAPIKey, "cloudflare-api-key", options.CloudflareAPIKey, "the key to use for cloudflare")
	flag.Parse()

	if options.CloudflareAPIEmail == "" {
		flag.Usage()
		log.Fatalln("cloudflare api email is required")
	}
	if options.CloudflareAPIKey == "" {
		flag.Usage()
		log.Fatalln("cloudflare api key is required")
	}
	if options.DNSName == "" {
		flag.Usage()
		log.Fatalln("dns name is required", options.DNSName)
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

		err = sync(ips)
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

func sync(ips []string) error {
	api, err := cloudflare.New(options.CloudflareAPIKey, options.CloudflareAPIEmail)
	if err != nil {
		return errors.Wrap(err, "failed to access cloudflare api")
	}

	root := options.DNSName
	for strings.Count(root, ".") > 1 {
		root = root[strings.Index(root, ".")+1:]
	}

	zoneID, err := api.ZoneIDByName(root)
	if err != nil {
		return errors.Wrapf(err, "failed to find zone id for zone-name:=%s",
			root)
	}

	records, err := api.DNSRecords(zoneID, cloudflare.DNSRecord{})
	if err != nil {
		return errors.Wrapf(err, "failed to list dns records for zone-id=%s",
			zoneID)
	}

	known := map[string]bool{}
	for _, ip := range ips {
		known[ip] = true
	}
	seen := map[string]bool{}
	var remove []string

	for _, record := range records {
		if record.Type == "A" &&
			record.Name == options.DNSName {
			log.Printf("found existing record name=%s type=%s content=%s\n",
				record.Name, record.Type, record.Content)
			if _, ok := known[record.Content]; ok {
				seen[record.Content] = true
			} else {
				remove = append(remove, record.ID)
			}
		}
	}

	for ip := range known {
		if _, ok := seen[ip]; ok {
			continue
		}
		log.Printf("adding dns record zone-id=%s ip=%s\n",
			zoneID, ip)

		_, err := api.CreateDNSRecord(zoneID, cloudflare.DNSRecord{
			Type:    "A",
			Name:    options.DNSName,
			Content: ip,
			TTL:     120,
			Proxied: true,
		})
		if err != nil {
			return errors.Wrapf(err, "failed to create dns record zone-id=%s name=%s ip=%s",
				zoneID, options.DNSName, ip)
		}
	}

	for _, recordID := range remove {
		log.Printf("removing dns record zone-id=%s record-id=%s\n",
			zoneID, recordID)
		err := api.DeleteDNSRecord(zoneID, recordID)
		if err != nil {
			return errors.Wrapf(err, "failed to delete dns record zone-id=%s record-id=%s",
				zoneID, recordID)
		}
	}

	return nil
}
