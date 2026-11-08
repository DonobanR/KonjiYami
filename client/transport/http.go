package transport

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"
)

// HTTPClient connects to the KonjiYami server via the REST API.
type HTTPClient struct {
	BaseURL    string
	httpClient *http.Client
}

// HTTPConnect authenticates to the server and returns a ready client.
func HTTPConnect(host string, port int, username, password string) (*HTTPClient, error) {
	jar, _ := cookiejar.New(nil)
	c := &HTTPClient{
		BaseURL: fmt.Sprintf("http://%s:%d", host, port),
		httpClient: &http.Client{
			Timeout: 10 * time.Minute,
			Jar:     jar,
		},
	}
	if err := c.login(username, password); err != nil {
		return nil, fmt.Errorf("login failed: %w", err)
	}
	return c, nil
}

func (c *HTTPClient) login(username, password string) error {
	data := fmt.Sprintf("username=%s&password=%s", username, password)
	req, _ := http.NewRequest("POST", c.BaseURL+"/login", bytes.NewBufferString(data))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c.httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := c.httpClient.Do(req)
	c.httpClient.CheckRedirect = nil
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != 303 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

// Get performs a GET request and decodes JSON into dst.
func (c *HTTPClient) Get(path string, dst interface{}) error {
	resp, err := c.httpClient.Get(c.BaseURL + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	if dst != nil {
		return json.NewDecoder(resp.Body).Decode(dst)
	}
	return nil
}

// PostJSON sends a JSON body and decodes the response.
func (c *HTTPClient) PostJSON(path string, body, dst interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Post(c.BaseURL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, respBody)
	}
	if dst != nil {
		return json.NewDecoder(resp.Body).Decode(dst)
	}
	return nil
}

// Delete sends a DELETE request.
func (c *HTTPClient) Delete(path string) error {
	req, _ := http.NewRequest("DELETE", c.BaseURL+path, nil)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	return nil
}
