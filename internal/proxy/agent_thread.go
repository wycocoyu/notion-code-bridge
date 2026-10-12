package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// agentTranscriptPage reflects the current browser protocol. Each poll requests
// a full page; state is rebuilt from its patches to avoid duplicate output.
type agentTranscriptPage struct {
	Type            string `json:"type"`
	HasMoreBackward bool   `json:"has_more_backward"`
	Session         struct {
		Status string `json:"status"`
	} `json:"session"`
	Patches []struct {
		Op     string                `json:"op"`
		ID     string                `json:"id"`
		Entity agentTranscriptEntity `json:"entity"`
	} `json:"patches"`
}
type agentTranscriptEntity struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Sequence   int    `json:"sequence"`
	StopReason string `json:"stop_reason"`
	Content    []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

func readAgentTranscript(page agentTranscriptPage) (string, bool, error) {
	if page.Type != "agent_service_transcript_page" {
		return "", false, fmt.Errorf("unexpected Notion Agent transcript type")
	}
	if page.HasMoreBackward {
		return "", false, fmt.Errorf("Notion Agent transcript exceeds one page; refusing incomplete output")
	}
	switch page.Session.Status {
	case "idle", "running", "":
	default:
		return "", false, fmt.Errorf("Notion Agent session status %q", page.Session.Status)
	}
	entities := make(map[string]agentTranscriptEntity)
	for _, patch := range page.Patches {
		switch patch.Op {
		case "put":
			if patch.Entity.ID == "" {
				return "", false, fmt.Errorf("Notion Agent entity missing ID")
			}
			entities[patch.Entity.ID] = patch.Entity
		case "remove":
			delete(entities, patch.ID)
		}
	}
	done := false
	var messages []agentTranscriptEntity
	for _, entity := range entities {
		switch entity.Kind {
		case "assistant_message":
			messages = append(messages, entity)
		case "turn_completed":
			if entity.StopReason != "end_turn" {
				return "", false, fmt.Errorf("Notion Agent turn stopped: %q", entity.StopReason)
			}
			done = true
		case "error", "turn_failed":
			return "", false, fmt.Errorf("Notion Agent transcript contains failure")
		}
	}
	sort.Slice(messages, func(i, j int) bool {
		if messages[i].Sequence == messages[j].Sequence {
			return messages[i].ID < messages[j].ID
		}
		return messages[i].Sequence < messages[j].Sequence
	})
	var texts []string
	for _, entity := range messages {
		for _, block := range entity.Content {
			if block.Type == "text" {
				texts = append(texts, block.Text)
			}
		}
	}
	return strings.Join(texts, "\n"), done && page.Session.Status == "idle", nil
}

// agentThreadPrompt uses plain text so the local action protocol is visible
// directly, while preserving exact tool arguments and returned result content.
func agentThreadPrompt(messages []ChatMessage) string {
	var prompt strings.Builder
	prompt.WriteString("You are assisting the user with work on their own computer. Continue the conversation below. A local client executes requested actions on the user's computer. Request local actions using the supplied JSON protocol; do not try Notion's hosted filesystem or shell for local paths. Wait for the returned tool message before claiming a result. Never invent tool results.\n\n")
	for _, message := range messages {
		switch message.Role {
		case "system":
			prompt.WriteString("PROJECT CONTEXT AND LOCAL CLIENT INSTRUCTIONS\n")
		case "user":
			prompt.WriteString("USER\n")
		case "assistant":
			prompt.WriteString("ASSISTANT\n")
		case "tool":
			fmt.Fprintf(&prompt, "LOCAL TOOL RESULT (call_id=%s, name=%s)\n", message.ToolCallID, message.Name)
		default:
			fmt.Fprintf(&prompt, "MESSAGE (%s)\n", message.Role)
		}
		prompt.WriteString(message.Content)
		if len(message.ToolCalls) > 0 {
			data, _ := json.Marshal(message.ToolCalls)
			prompt.WriteString("\nLOCAL TOOL REQUESTS\n")
			prompt.Write(data)
		}
		prompt.WriteString("\n\n")
	}
	prompt.WriteString("NEXT ASSISTANT RESPONSE\nIf a local action is needed, output only one JSON object {\"name\":\"action_name\",\"arguments\":{...}} using an available action and its schema. The local client will execute it and return the result. Otherwise reply normally.")
	return prompt.String()
}

// agentThreadModelID resolves only verified Agent protocol model selections.
// Avoid the legacy family fallback: an Opus 5.5 request must never become 4.6.
func agentThreadModelID(model string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "auto":
		return "", nil
	case "opus-5.5", "claude-opus-5-5", "claude-opus-5.5", "albuquerque-quinn":
		return "albuquerque-quinn", nil
	default:
		return "", fmt.Errorf("unsupported Notion Agent model %q; use auto or opus-5.5", model)
	}
}

func agentThreadBody(acc *Account, messages []ChatMessage, notionModel string) map[string]interface{} {
	// All turns are fresh threads. Claude Code carries the full conversation,
	// including locally executed tool results, in these messages.
	prompt := agentThreadPrompt(messages)
	body := map[string]interface{}{
		"type": "personal_agent", "spaceId": acc.SpaceID, "threadId": generateUUIDv4(), "clientMessageId": generateUUIDv4(),
		"createdSource": "full_page_chat", "content": []interface{}{map[string]interface{}{"type": "text", "text": [][]string{{prompt}}}},
		"agentMemorySettings": map[string]bool{"useMemories": false, "excludeChatFromMemories": true},
		"browserEnabled":      false, "enableSuggestedEditsTools": false, "policies": map[string]string{"approval_mode": "ask"},
	}
	if notionModel != "" {
		body["model"] = notionModel
		body["reasoningEffort"] = "medium"
	}
	return body
}

func callAgentThreadInference(acc *Account, messages []ChatMessage, model string, cb StreamCallback, opt CallOptions) error {
	notionModel, err := agentThreadModelID(model)
	if err != nil {
		return err
	}
	for _, attachment := range opt.Attachments {
		if attachment.AgentFileID == "" {
			return fmt.Errorf("Notion Agent attachment is missing a service file ID")
		}
	}
	return callAgentThreadWithClient(acc, messages, notionModel, cb, getChromeHTTPClient(AppConfig.InferenceTimeoutDuration()), opt.Attachments...)
}

func callAgentThreadWithClient(acc *Account, messages []ChatMessage, notionModel string, cb StreamCallback, client *http.Client, attachments ...UploadedAttachment) error {
	timeout := AppConfig.InferenceTimeoutDuration()
	if timeout <= 0 {
		return fmt.Errorf("Notion Agent inference timeout must be positive")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	post := func(endpoint string, body interface{}, result interface{}) error {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(NotionAPIBase, "/")+"/"+endpoint, bytes.NewReader(data))
		if err != nil {
			return err
		}
		setNotionHeadersJSON(req, acc)
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("Notion Agent %s: %w", endpoint, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("Notion Agent %s HTTP %d", endpoint, resp.StatusCode)
		}
		reader, cleanup, err := decompressBody(resp)
		if err != nil {
			return err
		}
		if cleanup != nil {
			defer cleanup()
		}
		if result == nil {
			_, err = io.Copy(io.Discard, io.LimitReader(reader, 8<<20))
			return err
		}
		if err = json.NewDecoder(io.LimitReader(reader, 8<<20)).Decode(result); err != nil {
			return fmt.Errorf("Notion Agent invalid response for %s", endpoint)
		}
		return nil
	}
	body := agentThreadBody(acc, messages, notionModel)
	content := body["content"].([]interface{})
	for _, attachment := range attachments {
		content = append(content, map[string]string{"type": "file", "file_id": attachment.AgentFileID})
	}
	body["content"] = content
	if err := post("createAgentThread", body, nil); err != nil {
		return err
	}
	query := map[string]interface{}{"spaceId": acc.SpaceID, "threadId": body["threadId"], "direction": "backward", "limit": 100}
	for {
		var page agentTranscriptPage
		if err := post("getThreadTranscript", query, &page); err != nil {
			return err
		}
		text, done, err := readAgentTranscript(page)
		if err != nil {
			return err
		}
		if done {
			if strings.TrimSpace(text) == "" {
				return ErrEmptyResponse
			}
			cb(text, false, nil)
			cb("", true, nil)
			return nil
		}
		timer := time.NewTimer(700 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("Notion Agent inference deadline exceeded: %w", ctx.Err())
		case <-timer.C:
		}
	}
}
