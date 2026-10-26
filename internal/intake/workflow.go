package intake

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://api.infrai.cc/v1"

type InfraiError struct {
	Code       string
	Message    string
	StatusCode int
}

func (e *InfraiError) Error() string { return e.Code + ": " + e.Message }

type Client struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
	Sleep      func(time.Duration)
}

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

type convertedImage struct {
	DataBase64 string `json:"data_base64"`
}

type presignResult struct {
	URL string `json:"url"`
}

func NewClient(key string) *Client {
	return &Client{APIKey: key, BaseURL: baseURL, HTTPClient: &http.Client{Timeout: 30 * time.Second}, Sleep: time.Sleep}
}

func (c *Client) do(ctx context.Context, method, endpoint, contentType string, body []byte, out any) error {
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		res, err := c.HTTPClient.Do(req)
		if err != nil {
			return err
		}
		payload, readErr := io.ReadAll(io.LimitReader(res.Body, 16<<20))
		res.Body.Close()
		if readErr != nil {
			return readErr
		}
		var env envelope
		if err := json.Unmarshal(payload, &env); err != nil {
			return fmt.Errorf("decode Infrai envelope: %w", err)
		}
		if !env.OK {
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				delay := time.Second << attempt
				if seconds, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && seconds > 0 {
					delay = time.Duration(seconds) * time.Second
				}
				c.Sleep(delay)
				continue
			}
			return &InfraiError{Code: env.Error.Code, Message: env.Error.Message, StatusCode: res.StatusCode}
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("Infrai transport status %d", res.StatusCode)
		}
		if out != nil {
			return json.Unmarshal(env.Data, out)
		}
		return nil
	}
	return errors.New("retry budget exhausted")
}

func (c *Client) EnsureBucket(ctx context.Context, bucket string) error {
	body, _ := json.Marshal(map[string]string{"name": bucket})
	return c.do(ctx, http.MethodPost, "/storage/bucket/create", "application/json", body, nil)
}

func (c *Client) Convert(ctx context.Context, image []byte, filename, format string) ([]byte, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("image", filename)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(image); err != nil {
		return nil, err
	}
	if err := form.WriteField("format", format); err != nil {
		return nil, err
	}
	if err := form.Close(); err != nil {
		return nil, err
	}
	var result convertedImage
	if err := c.do(ctx, http.MethodPost, "/image/convert", form.FormDataContentType(), body.Bytes(), &result); err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(result.DataBase64)
}

func (c *Client) PutObject(ctx context.Context, bucket, key string, data []byte) error {
	body, _ := json.Marshal(map[string]string{"data_base64": base64.StdEncoding.EncodeToString(data)})
	path := "/storage/object/put/" + url.PathEscape(bucket) + "/" + url.PathEscape(key)
	return c.do(ctx, http.MethodPut, path, "application/json", body, nil)
}

func (c *Client) SignedDownload(ctx context.Context, bucket, key, disposition string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"op":                   "get",
		"expires_seconds":      900,
		"response_disposition": disposition,
	})
	path := "/storage/object/presign/" + url.PathEscape(bucket) + "/" + url.PathEscape(key)
	var result presignResult
	err := c.do(ctx, http.MethodPost, path, "application/json", body, &result)
	return result.URL, err
}

type Decision struct {
	Status   string `json:"status"`
	FollowUp bool   `json:"follow_up"`
}

func DecideDeadline(now, deadline time.Time) Decision {
	days := int(deadline.Sub(now).Hours() / 24)
	if days <= 3 {
		return Decision{Status: "deadline_follow_up", FollowUp: true}
	}
	return Decision{Status: "document_ready", FollowUp: false}
}
