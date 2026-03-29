package gemini

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/JakobAIOdev/GoPilot/internal/chat"
)

const (
	defaultAPIBaseURL    = "https://cloudcode-pa.googleapis.com/v1internal"
	defaultOAuthTokenURL = "https://oauth2.googleapis.com/token"
	oauthClientID        = "681255809395-oo8ft2oprdrnp9e3aqf6av3hmdib135j.apps.googleusercontent.com"
	oauthClientSecret    = "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"
	freeTierID           = "free-tier"
	streamRetryCount     = 3
	streamRetryDelay     = 1200 * time.Millisecond
)

type Backend struct {
	httpClient *http.Client
	apiBaseURL string
	tokenURL   string
	credsPath  string
	timeout    time.Duration

	projectMu sync.Mutex
	projectID string
}

type generateContentRequest struct {
	Model        string                       `json:"model"`
	Project      string                       `json:"project,omitempty"`
	UserPromptID string                       `json:"user_prompt_id"`
	Request      vertexGenerateContentRequest `json:"request"`
}

type vertexGenerateContentRequest struct {
	Contents  []content `json:"contents"`
	SessionID string    `json:"session_id,omitempty"`
}

type content struct {
	Role  string `json:"role,omitempty"`
	Parts []part `json:"parts"`
}

type part struct {
	Text string `json:"text,omitempty"`
}

type generateContentResponse struct {
	Response *generateContentResult `json:"response,omitempty"`
	TraceID  string                 `json:"traceId,omitempty"`
	Error    *apiError              `json:"error,omitempty"`
}

type generateContentResult struct {
	Candidates []candidate `json:"candidates"`
}

type candidate struct {
	Content content `json:"content"`
}

type apiError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

type loadCodeAssistRequest struct {
	CloudAICompanionProject string             `json:"cloudaicompanionProject,omitempty"`
	Metadata                codeAssistMetadata `json:"metadata"`
}

type codeAssistMetadata struct {
	IDEType     string `json:"ideType"`
	Platform    string `json:"platform"`
	PluginType  string `json:"pluginType"`
	DuetProject string `json:"duetProject,omitempty"`
}

type loadCodeAssistResponse struct {
	CurrentTier             *userTier        `json:"currentTier,omitempty"`
	AllowedTiers            []userTier       `json:"allowedTiers,omitempty"`
	IneligibleTiers         []ineligibleTier `json:"ineligibleTiers,omitempty"`
	CloudAICompanionProject string           `json:"cloudaicompanionProject,omitempty"`
}

type userTier struct {
	ID        string `json:"id,omitempty"`
	IsDefault bool   `json:"isDefault,omitempty"`
}

type ineligibleTier struct {
	ReasonMessage string `json:"reasonMessage,omitempty"`
}

type onboardUserRequest struct {
	TierID                  string             `json:"tierId"`
	CloudAICompanionProject string             `json:"cloudaicompanionProject,omitempty"`
	Metadata                codeAssistMetadata `json:"metadata"`
}

type operationResponse struct {
	Name     string                   `json:"name,omitempty"`
	Done     bool                     `json:"done,omitempty"`
	Response *onboardOperationPayload `json:"response,omitempty"`
	Error    *apiError                `json:"error,omitempty"`
}

type onboardOperationPayload struct {
	CloudAICompanionProject *projectRef `json:"cloudaicompanionProject,omitempty"`
}

type projectRef struct {
	ID string `json:"id,omitempty"`
}

func NewBackend() chat.Backend {
	baseURL := strings.TrimSpace(os.Getenv("GEMINI_API_BASE_URL"))
	if baseURL == "" {
		baseURL = defaultAPIBaseURL
	}

	homeDir, err := os.UserHomeDir()
	credsPath := ""
	if err == nil {
		credsPath = filepath.Join(homeDir, ".gemini", "oauth_creds.json")
	}

	return &Backend{
		httpClient: &http.Client{},
		apiBaseURL: strings.TrimRight(baseURL, "/"),
		tokenURL:   defaultOAuthTokenURL,
		credsPath:  credsPath,
		timeout:    2 * time.Minute,
	}
}

func (b *Backend) Stream(ctx context.Context, req chat.Request) (<-chan chat.StreamEvent, error) {
	if strings.TrimSpace(req.Model) == "" {
		return nil, fmt.Errorf("model is empty")
	}
	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("messages are empty")
	}

	if b.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, b.timeout)
		go func() {
			<-ctx.Done()
			cancel()
		}()
	}

	events := make(chan chat.StreamEvent)
	go func() {
		defer close(events)

		sendStatus := func(status string) bool {
			select {
			case <-ctx.Done():
				return false
			case events <- chat.StreamEvent{Status: status}:
				return true
			}
		}

		if !sendStatus("Authenticating...") {
			return
		}

		accessToken, err := b.accessToken(ctx)
		if err != nil {
			events <- chat.StreamEvent{Err: err, Done: true}
			return
		}

		if !sendStatus("Loading project...") {
			return
		}

		projectID, err := b.resolveCodeAssistProject(ctx, accessToken)
		if err != nil {
			events <- chat.StreamEvent{Err: err, Done: true}
			return
		}

		if !sendStatus("Preparing context...") {
			return
		}

		payload := generateContentRequest{
			Model:        req.Model,
			Project:      projectID,
			UserPromptID: newPromptID(),
			Request: vertexGenerateContentRequest{
				Contents: toContents(req),
			},
		}

		if !sendStatus("Sending request...") {
			return
		}

		stream, err := b.openStream(ctx, accessToken, b.methodURL("streamGenerateContent"), payload)
		if err != nil {
			events <- chat.StreamEvent{Err: err, Done: true}
			return
		}

		if !sendStatus("Thinking...") {
			stream.Close()
			return
		}

		b.forwardStream(ctx, stream, events)
	}()
	return events, nil
}

func toContents(req chat.Request) []content {
	contents := make([]content, 0, len(req.Messages)+1)
	for _, msg := range req.Messages {
		text := strings.TrimSpace(msg.Content)
		if text == "" {
			continue
		}

		role := "user"
		if msg.From != "User" {
			role = "model"
		}

		contents = append(contents, content{
			Role: role,
			Parts: []part{
				{Text: text},
			},
		})
	}

	if contextText := contextPrompt(req); contextText != "" {
		contents = append(contents, content{
			Role: "user",
			Parts: []part{
				{Text: contextText},
			},
		})
	}

	return contents
}

func contextPrompt(req chat.Request) string {
	if len(req.ContextFiles) == 0 && !req.AllowFileEdits {
		return ""
	}

	var b strings.Builder
	b.WriteString("Workspace context follows.\n")
	if root := strings.TrimSpace(req.WorkspaceRoot); root != "" {
		b.WriteString("Workspace root: ")
		b.WriteString(root)
		b.WriteString("\n")
	}

	if len(req.ContextFiles) > 0 {
		b.WriteString("\nAttached files:\n")
		for _, file := range req.ContextFiles {
			if strings.TrimSpace(file.Path) == "" {
				continue
			}

			b.WriteString("\nFILE ")
			b.WriteString(file.Path)
			b.WriteString("\n```")
			if lang := strings.TrimSpace(file.Language); lang != "" {
				b.WriteString(lang)
			}
			b.WriteString("\n")
			b.WriteString(file.Content)
			if !strings.HasSuffix(file.Content, "\n") {
				b.WriteString("\n")
			}
			b.WriteString("```\n")
		}
	}

	if req.AllowFileEdits {
		b.WriteString("\nIf the user asks you to create or edit files, return the proposed files using one or more fenced code blocks in exactly this format:\n")
		b.WriteString("```gopilot-file path=relative/path/from/workspace\n")
		b.WriteString("full file contents here\n")
		b.WriteString("```\n")
		b.WriteString("Only include blocks for files that should be written. Keep explanations outside the fenced blocks.\n")
	}

	return strings.TrimSpace(b.String())
}

func (b *Backend) forwardStream(ctx context.Context, stream io.ReadCloser, events chan<- chat.StreamEvent) {
	defer stream.Close()

	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var dataLines []string
	var emitted string

	flush := func() bool {
		if len(dataLines) == 0 {
			return true
		}

		chunk := strings.Join(dataLines, "\n")
		dataLines = nil

		var response generateContentResponse
		if err := json.Unmarshal([]byte(chunk), &response); err != nil {
			events <- chat.StreamEvent{Err: fmt.Errorf("decode stream chunk: %w", err), Done: true}
			return false
		}
		if response.Error != nil {
			events <- chat.StreamEvent{Err: fmt.Errorf("gemini subscription error (%d %s): %s", response.Error.Code, response.Error.Status, response.Error.Message), Done: true}
			return false
		}
		if response.Response == nil || len(response.Response.Candidates) == 0 {
			return true
		}

		text := extractText(response.Response.Candidates[0].Content)
		if text == "" {
			return true
		}

		delta := text
		if strings.HasPrefix(text, emitted) {
			delta = strings.TrimPrefix(text, emitted)
		}
		emitted = text

		if delta != "" {
			events <- chat.StreamEvent{Text: delta}
		}
		return true
	}

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			events <- chat.StreamEvent{Done: true}
			return
		default:
		}

		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data: ")))
			continue
		}
		if line == "" {
			if !flush() {
				return
			}
		}
	}

	if err := scanner.Err(); err != nil {
		events <- chat.StreamEvent{Err: fmt.Errorf("read stream: %w", err), Done: true}
		return
	}
	if !flush() {
		return
	}

	events <- chat.StreamEvent{Done: true}
}

func (b *Backend) resolveCodeAssistProject(ctx context.Context, accessToken string) (string, error) {
	b.projectMu.Lock()
	if b.projectID != "" {
		projectID := b.projectID
		b.projectMu.Unlock()
		return projectID, nil
	}
	b.projectMu.Unlock()

	projectID := strings.TrimSpace(os.Getenv("GOOGLE_CLOUD_PROJECT"))
	if projectID == "" {
		projectID = strings.TrimSpace(os.Getenv("GOOGLE_CLOUD_PROJECT_ID"))
	}

	loadReq := loadCodeAssistRequest{
		CloudAICompanionProject: projectID,
		Metadata: codeAssistMetadata{
			IDEType:     "IDE_UNSPECIFIED",
			Platform:    "PLATFORM_UNSPECIFIED",
			PluginType:  "GEMINI",
			DuetProject: projectID,
		},
	}

	var loadResp loadCodeAssistResponse
	if err := b.postJSONValue(ctx, accessToken, b.methodURL("loadCodeAssist"), loadReq, &loadResp); err != nil {
		return "", fmt.Errorf("load code assist: %w", err)
	}

	if loadResp.CurrentTier != nil {
		if strings.TrimSpace(loadResp.CloudAICompanionProject) != "" {
			return b.cacheProjectID(strings.TrimSpace(loadResp.CloudAICompanionProject)), nil
		}
		if projectID != "" {
			return b.cacheProjectID(projectID), nil
		}
		return "", b.ineligibleOrProjectError(loadResp)
	}

	tier, ok := defaultAllowedTier(loadResp.AllowedTiers)
	if !ok {
		return "", b.ineligibleOrProjectError(loadResp)
	}

	onboardReq := onboardUserRequest{
		TierID: tier.ID,
		Metadata: codeAssistMetadata{
			IDEType:     "IDE_UNSPECIFIED",
			Platform:    "PLATFORM_UNSPECIFIED",
			PluginType:  "GEMINI",
			DuetProject: projectID,
		},
	}
	if tier.ID != freeTierID {
		onboardReq.CloudAICompanionProject = projectID
	}

	var op operationResponse
	if err := b.postJSONValue(ctx, accessToken, b.methodURL("onboardUser"), onboardReq, &op); err != nil {
		return "", fmt.Errorf("onboard code assist user: %w", err)
	}

	for !op.Done && strings.TrimSpace(op.Name) != "" {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(5 * time.Second):
		}

		var next operationResponse
		if err := b.getJSON(ctx, accessToken, b.operationURL(op.Name), &next); err != nil {
			return "", fmt.Errorf("poll code assist onboarding: %w", err)
		}
		op = next
	}

	if op.Error != nil {
		return "", fmt.Errorf("code assist onboarding failed (%d %s): %s", op.Error.Code, op.Error.Status, op.Error.Message)
	}
	if op.Response != nil && op.Response.CloudAICompanionProject != nil && strings.TrimSpace(op.Response.CloudAICompanionProject.ID) != "" {
		return b.cacheProjectID(strings.TrimSpace(op.Response.CloudAICompanionProject.ID)), nil
	}
	if projectID != "" {
		return b.cacheProjectID(projectID), nil
	}

	return "", fmt.Errorf("code assist onboarding returned no project")
}

func (b *Backend) cacheProjectID(projectID string) string {
	b.projectMu.Lock()
	defer b.projectMu.Unlock()
	b.projectID = projectID
	return projectID
}

func (b *Backend) postJSONValue(ctx context.Context, accessToken string, endpoint string, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	return b.postJSON(ctx, accessToken, endpoint, body, target)
}

func (b *Backend) postJSON(ctx context.Context, accessToken string, endpoint string, body []byte, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("User-Agent", "GoPilot/0.1")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeAPIError(resp.Status, raw)
	}
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}

func (b *Backend) getJSON(ctx context.Context, accessToken string, endpoint string, target any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "GoPilot/0.1")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeAPIError(resp.Status, raw)
	}
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	return nil
}

func (b *Backend) openStream(ctx context.Context, accessToken string, endpoint string, payload any) (io.ReadCloser, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal stream request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < streamRetryCount; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * streamRetryDelay):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"?alt=sse", bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("build stream request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("User-Agent", "GoPilot/0.1")

		resp, err := b.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("send stream request: %w", err)
			continue
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return resp.Body, nil
		}

		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var wrapper struct {
			Error *apiError `json:"error"`
		}
		if json.Unmarshal(raw, &wrapper) == nil && wrapper.Error != nil {
			lastErr = fmt.Errorf("%s (%d %s)", wrapper.Error.Message, wrapper.Error.Code, wrapper.Error.Status)
			return nil, lastErr
		}

		lastErr = decodeAPIError(resp.Status, raw)
		return nil, lastErr
	}

	return nil, lastErr
}

func (b *Backend) methodURL(method string) string {
	return fmt.Sprintf("%s:%s", b.apiBaseURL, method)
}

func (b *Backend) operationURL(name string) string {
	return fmt.Sprintf("%s/%s", b.apiBaseURL, strings.TrimLeft(name, "/"))
}

func extractText(content content) string {
	var parts []string
	for _, part := range content.Parts {
		text := strings.TrimSpace(part.Text)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func (b *Backend) ineligibleOrProjectError(resp loadCodeAssistResponse) error {
	if len(resp.IneligibleTiers) > 0 {
		reasons := make([]string, 0, len(resp.IneligibleTiers))
		for _, tier := range resp.IneligibleTiers {
			if strings.TrimSpace(tier.ReasonMessage) != "" {
				reasons = append(reasons, strings.TrimSpace(tier.ReasonMessage))
			}
		}
		if len(reasons) > 0 {
			return fmt.Errorf("%s", strings.Join(reasons, ", "))
		}
	}
	return fmt.Errorf("this account requires onboarding but no usable Code Assist project was returned")
}

func defaultAllowedTier(tiers []userTier) (userTier, bool) {
	for _, tier := range tiers {
		if tier.IsDefault {
			return tier, true
		}
	}
	if len(tiers) == 0 {
		return userTier{}, false
	}
	return tiers[0], true
}

func newPromptID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("prompt-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", raw[:])
}

func decodeAPIError(status string, raw []byte) error {
	var wrapper struct {
		Error *apiError `json:"error"`
	}
	if json.Unmarshal(raw, &wrapper) == nil && wrapper.Error != nil {
		return fmt.Errorf("%s (%d %s)", wrapper.Error.Message, wrapper.Error.Code, wrapper.Error.Status)
	}

	body := strings.TrimSpace(string(raw))
	if body == "" {
		return fmt.Errorf("request failed: %s", status)
	}
	if len(body) > 240 {
		body = body[:240] + "..."
	}
	return fmt.Errorf("request failed: %s: %s", status, body)
}
