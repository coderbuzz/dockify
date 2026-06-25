package cloudflare

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type DNSRecord struct {
	ID      string `json:"id"`
	ZoneID  string `json:"zone_id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Content string `json:"content"`
	Proxied bool   `json:"proxied"`
	TTL     int    `json:"ttl"`
}

type apiResponse struct {
	Success  bool        `json:"success"`
	Errors   []apiError  `json:"errors"`
	Messages []string    `json:"messages"`
	Result   json.RawMessage `json:"result"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type createRecordRequest struct {
	Type    string `json:"type"`
	Name    string `json:"name"`
	Content string `json:"content"`
	TTL     int    `json:"ttl"`
	Proxied bool   `json:"proxied"`
}

type Client struct {
	token  string
	zoneID string
	http   *http.Client
}

func New(token, zoneID string) *Client {
	return &Client{
		token:  token,
		zoneID: zoneID,
		http:   &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) Enabled() bool {
	return c.token != "" && c.zoneID != ""
}

func (c *Client) CreateRecord(name, ip string, proxied bool) (*DNSRecord, error) {
	return c.createOrUpdate(name, ip, proxied, "")
}

func (c *Client) UpdateRecord(id, name, ip string, proxied bool) (*DNSRecord, error) {
	return c.createOrUpdate(name, ip, proxied, id)
}

func (c *Client) UpsertRecord(name, ip string, proxied bool) (*DNSRecord, error) {
	records, err := c.ListRecords(name)
	if err != nil {
		return c.CreateRecord(name, ip, proxied)
	}
	for _, r := range records {
		if r.Name == name && r.Type == "A" {
			if r.Content == ip && r.Proxied == proxied {
				return &r, nil
			}
			log.Printf("Cloudflare DNS: updating A record %s (%s) -> %s", name, r.ID, ip)
			return c.UpdateRecord(r.ID, name, ip, proxied)
		}
	}
	return c.CreateRecord(name, ip, proxied)
}

func (c *Client) createOrUpdate(name, ip string, proxied bool, updateID string) (*DNSRecord, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", c.zoneID)
	method := "POST"
	if updateID != "" {
		url += "/" + updateID
		method = "PUT"
	}

	reqBody := createRecordRequest{
		Type:    "A",
		Name:    name,
		Content: ip,
		TTL:     120,
		Proxied: proxied,
	}

	body, _ := json.Marshal(reqBody)
	httpReq, _ := http.NewRequest(method, url, bytes.NewReader(body))
	httpReq.Header.Set("Authorization", "Bearer "+c.token)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("cloudflare api: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var ar apiResponse
	if err := json.Unmarshal(respBody, &ar); err != nil {
		return nil, fmt.Errorf("cloudflare response: %w (status %d)", err, resp.StatusCode)
	}

	if !ar.Success {
		msg := "unknown error"
		if len(ar.Errors) > 0 {
			msg = ar.Errors[0].Message
		}
		return nil, fmt.Errorf("cloudflare: %s", msg)
	}

	var record DNSRecord
	if err := json.Unmarshal(ar.Result, &record); err != nil {
		return nil, fmt.Errorf("cloudflare result: %w", err)
	}

	if updateID != "" {
		log.Printf("Cloudflare DNS: updated A record %s -> %s (proxied=%v)", name, ip, proxied)
	} else {
		log.Printf("Cloudflare DNS: created A record %s -> %s (proxied=%v)", name, ip, proxied)
	}
	return &record, nil
}

func (c *Client) DeleteRecord(id string) error {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records/%s", c.zoneID, id)

	httpReq, _ := http.NewRequest("DELETE", url, nil)
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("cloudflare api: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var ar apiResponse
	if err := json.Unmarshal(respBody, &ar); err != nil {
		return fmt.Errorf("cloudflare response: %w (status %d)", err, resp.StatusCode)
	}

	if !ar.Success {
		msg := "unknown error"
		if len(ar.Errors) > 0 {
			msg = ar.Errors[0].Message
		}
		return fmt.Errorf("cloudflare: %s", msg)
	}

	log.Printf("Cloudflare DNS: deleted record %s", id)
	return nil
}

func (c *Client) ListRecords(name string) ([]DNSRecord, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/zones/%s/dns_records", c.zoneID)
	if name != "" {
		url += "?name=" + name
	}

	httpReq, _ := http.NewRequest("GET", url, nil)
	httpReq.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("cloudflare api: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var ar apiResponse
	if err := json.Unmarshal(respBody, &ar); err != nil {
		return nil, fmt.Errorf("cloudflare response: %w", err)
	}

	if !ar.Success {
		msg := "unknown error"
		if len(ar.Errors) > 0 {
			msg = ar.Errors[0].Message
		}
		return nil, fmt.Errorf("cloudflare: %s", msg)
	}

	var records []DNSRecord
	if err := json.Unmarshal(ar.Result, &records); err != nil {
		return nil, fmt.Errorf("cloudflare result: %w", err)
	}

	return records, nil
}
