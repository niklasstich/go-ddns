package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const GodaddyBaseUrl = "https://api.godaddy.com"
const GodaddyApiBase = GodaddyBaseUrl + "/v1/domains"

type GodaddyGetDNSRecordResponse struct {
	Data string `json:"data"`
	Name string `json:"name"`
}

type GodaddySetDNSRecordRequest struct {
	Data string `json:"data"`
	TTL  uint64 `json:"ttl"`
}

func getDomainAtRecordIP(ctx context.Context, domain string) (string, error) {
	//prepare request
	url := fmt.Sprintf("%s/%s/records/A/@", GodaddyApiBase, domain)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("Authorization", getGDAuthHeader())
	httpRes, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	//read body
	httpResBytes, err := io.ReadAll(httpRes.Body)
	if err != nil {
		return "", err
	}

	if httpRes.StatusCode != http.StatusOK {
		return "", fmt.Errorf("godaddy sent non-ok status code %d, body: %s", httpRes.StatusCode, string(httpResBytes))
	}
	//parse body
	var res []GodaddyGetDNSRecordResponse
	err = json.Unmarshal(httpResBytes, &res)
	if err != nil {
		return "", err
	}

	return res[0].Data, nil
}

func setDomainAtRecord(ctx context.Context, domain, ip string) error {
	//prepare body
	var body bytes.Buffer
	err := json.NewEncoder(&body).Encode([]GodaddySetDNSRecordRequest{
		{
			Data: ip,
			TTL:  600,
		},
	})
	if err != nil {
		return err
	}
	//prepare request
	url := fmt.Sprintf("%s/%s/records/A/@", GodaddyApiBase, domain)
	req, err := http.NewRequestWithContext(ctx, "PUT", url, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", getGDAuthHeader())
	res, err := httpClient.Do(req)
	if err != nil {
		return nil
	}
	if res.StatusCode != http.StatusOK {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("received non-ok status code %d from godaddy, but failed to read body: %v", res.StatusCode, err)
		}
		return fmt.Errorf("received non-ok status code %d from godaddy, body: %s", res.StatusCode, string(body))
	}
	return nil
}

func getGDAuthHeader() string {
	return fmt.Sprintf("sso-key %s:%s", apiKey, apiSecret)
}
