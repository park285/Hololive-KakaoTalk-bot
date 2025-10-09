package iris

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/pkg/errors"
	"go.uber.org/zap"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
	logger     *zap.Logger
}

func NewClient(baseURL string, logger *zap.Logger) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}
}

func (c *Client) GetConfig(ctx context.Context) (*Config, error) {
	var config Config
	if err := c.doRequest(ctx, "GET", "/config", nil, &config); err != nil {
		c.logger.Error("Failed to get Iris config", zap.Error(err))
		return nil, err
	}
	return &config, nil
}

func (c *Client) Decrypt(ctx context.Context, data string) (string, error) {
	req := DecryptRequest{Data: data}
	var resp DecryptResponse

	if err := c.doRequest(ctx, "POST", "/decrypt", req, &resp); err != nil {
		c.logger.Error("Failed to decrypt message", zap.Error(err))
		return "", err
	}

	return resp.Decrypted, nil
}

func (c *Client) SendMessage(ctx context.Context, room, message string) error {
	req := ReplyRequest{
		Type: "text",
		Room: room,
		Data: message,
	}

	if err := c.doRequest(ctx, "POST", "/reply", req, nil); err != nil {
		c.logger.Error("Failed to send message",
			zap.Error(err),
			zap.String("room", room),
		)
		return err
	}

	return nil
}

func (c *Client) SendImage(ctx context.Context, room, imageBase64 string) error {
	req := ImageReplyRequest{
		Type: "image",
		Room: room,
		Data: imageBase64,
	}

	if err := c.doRequest(ctx, "POST", "/reply", req, nil); err != nil {
		c.logger.Error("Failed to send image",
			zap.Error(err),
			zap.String("room", room),
		)
		return err
	}

	return nil
}

func (c *Client) Ping(ctx context.Context) bool {
	_, err := c.GetConfig(ctx)
	return err == nil
}

func (c *Client) doRequest(ctx context.Context, method, path string, reqBody, respBody any) error {
	url := c.baseURL + path

	var bodyReader io.Reader
	if reqBody != nil {
		jsonData, err := json.Marshal(reqBody)
		if err != nil {
			return errors.NewAPIError("failed to marshal request", 400, map[string]any{
				"url": url,
			}).WithCause(err)
		}
		bodyReader = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return errors.NewAPIError("failed to create request", 500, map[string]any{
			"url": url,
		}).WithCause(err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return errors.NewAPIError("request failed", 500, map[string]any{
			"url": url,
		}).WithCause(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return errors.NewAPIError(
			fmt.Sprintf("Iris API error: %s", resp.Status),
			resp.StatusCode,
			map[string]any{
				"url":  url,
				"body": string(bodyBytes),
			},
		)
	}

	if respBody != nil {
		if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
			return errors.NewAPIError("failed to decode response", 500, map[string]any{
				"url": url,
			}).WithCause(err)
		}
	}

	return nil
}
