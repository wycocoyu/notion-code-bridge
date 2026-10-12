package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAgentImageUploadCurrentProtocolAndCredentialIsolation(t *testing.T) {
	oldCfg, oldBase := AppConfig, NotionAPIBase
	AppConfig = DefaultConfig()
	defer func() { AppConfig, NotionAPIBase = oldCfg, oldBase }()
	stored := false
	store := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("signed upload leaked credentials or used wrong method")
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "image-bytes" || r.Header.Get("x-amz-tagging") != "test-tag" {
			t.Error("file bytes or signed headers changed")
		}
		stored = true
		w.WriteHeader(200)
	}))
	defer store.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		if body["spaceId"] != "space" || !strings.Contains(r.Header.Get("Cookie"), "test-cookie") {
			t.Error("Notion account scope missing")
		}
		switch r.URL.Path {
		case "/createAgentServiceFileUploadURL":
			if _, ok := body["pointer"]; ok {
				t.Error("legacy thread pointer sent")
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"type": "file_upload_url", "file": map[string]string{"id": "file-id"}, "upload": map[string]interface{}{"type": "single_part", "method": "PUT", "url": store.URL, "headers": map[string]string{"x-amz-tagging": "test-tag"}}})
		case "/completeAgentServiceFileUpload":
			if !stored || body["fileId"] != "file-id" {
				t.Error("completion before successful upload")
			}
			io.WriteString(w, `{"type":"file","id":"file-id"}`)
		default:
			t.Error("unexpected API endpoint")
		}
	}))
	defer api.Close()
	NotionAPIBase = api.URL
	result, err := uploadAgentImageWithClients(&Account{SpaceID: "space", TokenV2: "test-cookie"}, &FileAttachment{FileName: "test.png", ContentType: "image/png", Data: []byte("image-bytes")}, api.Client(), store.Client())
	if err != nil || result.AgentFileID != "file-id" {
		t.Fatalf("%v %v", result, err)
	}
}

func TestAgentImageUploadRejectsNonImageBeforeNetwork(t *testing.T) {
	_, err := uploadAgentImageWithClients(&Account{}, &FileAttachment{ContentType: "application/pdf", Data: []byte("pdf")}, nil, nil)
	if err == nil {
		t.Fatal("unsupported document accepted")
	}
}

func TestAgentThreadImageReferencesReachCreateRequest(t *testing.T) {
	oldCfg, oldBase := AppConfig, NotionAPIBase
	AppConfig = DefaultConfig()
	defer func() { AppConfig, NotionAPIBase = oldCfg, oldBase }()
	created := false
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/createAgentThread" {
			var body struct {
				Content []struct {
					Type   string `json:"type"`
					FileID string `json:"file_id"`
				} `json:"content"`
			}
			json.NewDecoder(r.Body).Decode(&body)
			if len(body.Content) != 2 || body.Content[0].Type != "text" || body.Content[1].FileID != "image-file" {
				t.Errorf("attachment reference lost: %+v", body)
			}
			created = true
			io.WriteString(w, `{}`)
		} else {
			io.WriteString(w, capturedShape)
		}
	}))
	defer api.Close()
	NotionAPIBase = api.URL
	err := callAgentThreadWithClient(&Account{}, nil, "", func(string, bool, *UsageInfo) {}, api.Client(), UploadedAttachment{AgentFileID: "image-file"})
	if err != nil || !created {
		t.Fatalf("%v created=%v", err, created)
	}
}
