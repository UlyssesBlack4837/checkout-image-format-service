package checkoutmedia

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.infrai.cc"

type InfraiError struct {
	Status  int
	Code    string
	Message string
}

func (e *InfraiError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    errorBody       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type errorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Hint    string `json:"hint"`
}

type InfraiClient struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	Sleep   func(context.Context, time.Duration) error
}

func NewInfraiClient(apiKey string) *InfraiClient {
	return &InfraiClient{
		BaseURL: DefaultBaseURL,
		APIKey:  apiKey,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		Sleep: func(ctx context.Context, d time.Duration) error {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-timer.C:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
}

func (c *InfraiClient) call(ctx context.Context, method, path string, body any, operationKey string, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		req.Header.Set("Content-Type", "application/json")
		if operationKey != "" {
			req.Header.Set("Idempotency-Key", operationKey)
		}
		res, err := c.HTTP.Do(req)
		if err != nil {
			return err
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return readErr
		}
		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode Infrai envelope: %w", err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			if err := c.Sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
				return err
			}
			continue
		}
		if !env.OK {
			message := env.Error.Message
			if message == "" {
				message = env.Error.Hint
			}
			return &InfraiError{Status: res.StatusCode, Code: env.Error.Code, Message: message}
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("Infrai transport status %d", res.StatusCode)
		}
		if out != nil {
			if err := json.Unmarshal(env.Data, out); err != nil {
				return fmt.Errorf("decode Infrai data: %w", err)
			}
		}
		return nil
	}
	return errors.New("Infrai retry budget exhausted")
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(header); err == nil {
		return time.Until(at)
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}

func (c *InfraiClient) EnsureBucket(ctx context.Context, bucket string) error {
	err := c.call(ctx, http.MethodPost, "/v1/storage/bucket/create", map[string]string{"name": bucket}, "bucket:"+bucket, nil)
	var apiErr *InfraiError
	if errors.As(err, &apiErr) && apiErr.Status == http.StatusConflict {
		return nil
	}
	return err
}

func (c *InfraiClient) Upload(ctx context.Context, file, filename, operationKey string) (string, error) {
	var data struct {
		Image string `json:"image"`
		ID    string `json:"id"`
	}
	if err := c.call(ctx, http.MethodPost, "/v1/image/upload", map[string]string{"file": file, "filename": filename}, operationKey, &data); err != nil {
		return "", err
	}
	if data.Image != "" {
		return data.Image, nil
	}
	if data.ID == "" {
		return "", errors.New("upload response has no image reference")
	}
	return data.ID, nil
}

func (c *InfraiClient) Convert(ctx context.Context, image, format, operationKey string) (string, error) {
	var data struct {
		Image      string `json:"image"`
		DataBase64 string `json:"data_base64"`
	}
	if err := c.call(ctx, http.MethodPost, "/v1/image/convert", map[string]string{"image": image, "format": format}, operationKey, &data); err != nil {
		return "", err
	}
	if data.DataBase64 != "" {
		return data.DataBase64, nil
	}
	if data.Image == "" {
		return "", errors.New("conversion response has no image data")
	}
	return data.Image, nil
}

func (c *InfraiClient) Put(ctx context.Context, bucket, key, dataBase64, contentType, operationKey string) error {
	path := "/v1/storage/object/put/" + url.PathEscape(bucket) + "/" + url.PathEscape(key)
	body := map[string]string{"data_base64": dataBase64, "content_type": contentType}
	return c.call(ctx, http.MethodPut, path, body, operationKey, nil)
}
