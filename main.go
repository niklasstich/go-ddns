package main

import (
	"context"
	"flag"
	"fmt"
	log "github.com/sirupsen/logrus"
	"io"
	"net"
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

	zeroDialer net.Dialer
	httpClient = &http.Client{
		Timeout: 10 * time.Second,
	}
)

const dateTimeFormat = "2006-01-02 15:04"

// command line flags
func init() {
	verbose := flag.Bool("v", false, "Turns on verbose output")
	flag.Parse()
	if *verbose {
		log.SetLevel(log.TraceLevel)
		log.Trace("Set log level to trace")
	}
}

// set force ipv4
func init() {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		return zeroDialer.DialContext(ctx, "tcp4", addr)
	}
	httpClient.Transport = transport
}

// get environment vars
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

	loopFunc := func() {
		currIPAddr, err := getPublicIPAddress()
		if err != nil {
			log.Errorf("failed to get public IP address: %v", err)
			return
		}
		for _, domain := range domains {
			err := checkAndUpdate(ctx, domain, currIPAddr)
			if err != nil {
				log.Errorf("Failed to update DNS records: %v", err)
			} else {
				log.Infof("Update successful at %v", time.Now().Format(dateTimeFormat))
			}
		}
		log.Infof("Next update at %v", time.Now().Add(updateInterval).Format(dateTimeFormat))
	}

	//run once before the loop
	loopFunc()
	for {
		select {
		case <-time.After(updateInterval):
			loopFunc()
		case <-ctx.Done():
			log.Trace("Stopping update loop")
			return
		}
	}
}

// checkAndUpdate determines if it is necessary to update the DNS records and does so accordingly
func checkAndUpdate(ctx context.Context, domain string, currentIpAddr string) error {
	//get current address from godaddy
	godaddyIPAddr, err := getDomainAtRecordIP(ctx, domain)
	if err != nil {
		return fmt.Errorf("failed to get DNS record IP address for domain %s: %v", domain, err)
	}
	if currentIpAddr == godaddyIPAddr {
		log.Infof("No update necessary for %s", domain)
		return nil
	}
	log.Debugf("oldIP: %s; newIP: %s", godaddyIPAddr, currentIpAddr)

	err = setDomainAtRecord(ctx, domain, currentIpAddr)
	if err != nil {
		return err
	}

	return nil
}

// getPublicIPAddress gets the current public IP address of this device
func getPublicIPAddress() (string, error) {
	resp, err := httpClient.Get("http://ifconfig.co")
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	ip := string(body)
	ip = strings.TrimRight(ip, "\n")
	return ip, nil
}
