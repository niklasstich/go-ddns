package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io/ioutil"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	updateInterval time.Duration
	apiKey         string
	apiSecret      string
	domains        []string
	lastIPAddr     string
)

const dateTimeFormat = "2006-01-02 15:04"

//command line flags
func init() {
	verbose := flag.Bool("v", false, "Turns on verbose output")
	flag.Parse()
	if *verbose {
		log.SetLevel(log.TraceLevel)
		log.Trace("Set log level to trace")
	}
}

//get environtment vars
func init() {
	interval := os.Getenv("GD_INTERVAL")
	var err error
	updateInterval, err = time.ParseDuration(interval)
	if err != nil {
		log.Warn("No update interval given, defaulting to 600 seconds.")
		updateInterval = time.Second * 600
	}

	apiKey = os.Getenv("GD_API_KEY")
	if apiKey == "" {
		log.Fatalf("No API Key provided in environment (GD_API_KEY).")
	}
	apiSecret = os.Getenv("GD_API_SECRET")
	if apiSecret == "" {
		log.Fatalf("No API Secret provided in environment (GD_API_SECRET).")
	}
	domains = strings.Split(os.Getenv("GD_DOMAINS"), ",")
	if len(domains) < 1 {
		log.Fatalf("No domains provided in environment (GD_DOMAINS).")
	}
}

func main() {
	log.Info("Starting go-ddns updater...")
	//run once before going into loop
	err := updateRecords(context.Background())
	if err != nil {
		log.Errorf("Failed to update DNS records: %v", err)
	} else {
		log.Infof("Update successful at %v, next update at %v", time.Now().Format(dateTimeFormat),
			time.Now().Add(updateInterval).Format(dateTimeFormat))
	}
	//establish cancelable context and waitgroup to wait for cancellation
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	go runUpdateLoop(ctx, &wg)

	//get signal channel and wait for signal
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, os.Kill, os.Interrupt)
	<-sigs
	log.Info("Received shutdown signal...")

	//cancel context and wait for full propagation
	cancel()
	wg.Wait()
	log.Info("Goodbye :(")
}

func runUpdateLoop(ctx context.Context, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()
	for {
		select {
		case <-time.After(updateInterval):
			err := updateRecords(ctx)
			if err != nil {
				log.Errorf("Failed to update DNS records: %v", err)
			} else {
				log.Infof("Update successful at %v, next update at %v", time.Now().Format(dateTimeFormat),
					time.Now().Add(updateInterval).Format(dateTimeFormat))
			}
		case <-ctx.Done():
			log.Trace("Stopping update loop")
			return
		}
	}
}

func updateRecords(ctx context.Context) error {
	//get current address from godaddy
	if lastIPAddr == "" {
		var err error
		lastIPAddr, err = getDNSEntriesIP(ctx)
		if err != nil {
			return fmt.Errorf("failed to get current DNS IP address: %v", err)
		}
	}

	//TODO: actually update GoDaddy records
	currIPAddr, err := getPublicIPAddress(ctx)
	if err != nil {
		return fmt.Errorf("failed to get public Data address: %v", err)
	}
	//if ip address is still the same no update is necessary
	if currIPAddr == lastIPAddr {
		log.Debug("Address still the same - no update necessary")
		return nil
	}
	log.Debugf("oldIP: %s; newIP: %s", lastIPAddr, currIPAddr)

	err = updateDNSEntriesIP(currIPAddr, ctx)
	if err != nil {
		return err
	}

	lastIPAddr = currIPAddr

	return nil
}

func updateDNSEntriesIP(ipaddr string, ctx context.Context) error {
	for _, domain := range domains {
		//create byte buffer for request body
		var buf bytes.Buffer
		err := json.NewEncoder(&buf).Encode([]struct {
			Name string `json:"name"`
			Data string `json:"data"`
			TTL  int64  `json:"ttl"`
		}{
			{
				"@",
				ipaddr,
				int64(updateInterval.Seconds()),
			},
		})
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, "PUT",
			fmt.Sprintf("https://api.godaddy.com/v1/domains/%s/records/A", domain), &buf)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", getGDAuthHeader())
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		if res.StatusCode != 200 {
			body, _ := ioutil.ReadAll(res.Body)
			return errors.New(fmt.Sprintf("updating dns record for domain %s failed with status code %d: %s",
				domain, res.StatusCode, string(body)))
		}
	}
	return nil
}

//getDNSEntriesIP returns the current Data address in the DNS entries, or "0.0.0.0" if there is
//more than one domain and the Data addresses aren't equal
func getDNSEntriesIP(ctx context.Context) (string, error) {
	retval := ""
	//TODO: get current ip addresses for all A entries for all domains
	for _, domain := range domains {
		req, err := http.NewRequestWithContext(ctx, "GET",
			fmt.Sprintf("https://api.godaddy.com/v1/domains/%s/records/A/@", domain), nil)
		if err != nil {
			return "", err
		}
		//set auth
		req.Header.Set("Authorization", getGDAuthHeader())
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		//anon struct to extract relevant value from json
		data := []struct {
			Data string `json:"data"`
		}{{}}
		err = json.NewDecoder(resp.Body).Decode(&data)
		if err != nil {
			return "", err
		}
		if retval != data[0].Data && retval != "" {
			return "0.0.0.0", nil
		}
		retval = data[0].Data
	}
	return retval, nil
}

func getGDAuthHeader() string {
	return fmt.Sprintf("sso-key %s:%s", apiKey, apiSecret)
}

func getPublicIPAddress(ctx context.Context) (string, error) {
	req, err := http.NewRequest("GET", "https://api.ipify.org", nil)
	if err != nil {
		return "", err
	}
	req = req.WithContext(ctx)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}
