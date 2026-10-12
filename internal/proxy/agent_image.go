package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Current Agent uploads are user-scoped files, independent of a chat thread.
// Credentials are sent only to Notion; the signed PUT uses only supplied headers.
func uploadAgentImage(acc *Account, file *FileAttachment) (*UploadedAttachment, error) {
	return uploadAgentImageWithClients(acc, file, getChromeHTTPClient(30*time.Second), &http.Client{
		Timeout:       30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	})
}

func uploadAgentImageWithClients(acc *Account, file *FileAttachment, api, storage *http.Client) (*UploadedAttachment, error) {
	if file == nil || len(file.Data) == 0 {
		return nil, fmt.Errorf("image bytes are required")
	}
	switch file.ContentType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
	default:
		return nil, fmt.Errorf("Agent uploads currently support PNG, JPEG, GIF and WebP images only")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	post := func(endpoint string, body, output interface{}) error {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(NotionAPIBase, "/")+endpoint, bytes.NewReader(data))
		if err != nil {
			return err
		}
		setNotionHeadersJSON(req, acc)
		resp, err := api.Do(req)
		if err != nil {
			return fmt.Errorf("Agent file API request failed")
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("Agent file %s HTTP %d", endpoint, resp.StatusCode)
		}
		reader, cleanup, err := decompressBody(resp)
		if err != nil {
			return err
		}
		if cleanup != nil {
			defer cleanup()
		}
		if err := json.NewDecoder(io.LimitReader(reader, 1<<20)).Decode(output); err != nil {
			return fmt.Errorf("invalid Agent file response")
		}
		return nil
	}
	target := map[string]string{"type": "user"}
	var created struct {
		Type string `json:"type"`
		File struct {
			ID string `json:"id"`
		} `json:"file"`
		Upload struct {
			Type    string            `json:"type"`
			Method  string            `json:"method"`
			URL     string            `json:"url"`
			Headers map[string]string `json:"headers"`
		} `json:"upload"`
	}
	if err := post("/createAgentServiceFileUploadURL", map[string]interface{}{
		"spaceId": acc.SpaceID, "target": target, "filename": file.FileName,
		"mediaType": file.ContentType, "sizeBytes": len(file.Data),
	}, &created); err != nil {
		return nil, err
	}
	u, err := url.Parse(created.Upload.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil ||
		created.Type != "file_upload_url" || created.File.ID == "" || created.Upload.Type != "single_part" || created.Upload.Method != "PUT" {
		return nil, fmt.Errorf("unsupported Agent signed upload response")
	}
	req, err := http.NewRequestWithContext(ctx, "PUT", u.String(), bytes.NewReader(file.Data))
	if err != nil {
		return nil, fmt.Errorf("invalid Agent signed upload request")
	}
	for name, value := range created.Upload.Headers {
		if strings.EqualFold(name, "Cookie") || strings.EqualFold(name, "Authorization") {
			return nil, fmt.Errorf("unexpected authentication header in signed upload")
		}
		req.Header.Set(name, value)
	}
	resp, err := storage.Do(req)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			// Exclude the credential-bearing signed URL from diagnostics.
			cause := urlErr.Err
			for errors.As(cause, &urlErr) {
				cause = urlErr.Err
			}
			return nil, fmt.Errorf("Agent signed upload failed: %v", cause)
		}
		return nil, fmt.Errorf("Agent signed upload failed (%T)", err)
	}
	resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Agent signed upload HTTP %d", resp.StatusCode)
	}
	var completed struct {
		Type string `json:"type"`
		ID   string `json:"id"`
	}
	if err := post("/completeAgentServiceFileUpload", map[string]interface{}{
		"spaceId": acc.SpaceID, "target": target, "fileId": created.File.ID,
	}, &completed); err != nil {
		return nil, err
	}
	if completed.Type != "file" || completed.ID != created.File.ID {
		return nil, fmt.Errorf("Agent file completion identity mismatch")
	}
	return &UploadedAttachment{AgentFileID: completed.ID, FileName: file.FileName, ContentType: file.ContentType, FileSizeBytes: int64(len(file.Data))}, nil
}
