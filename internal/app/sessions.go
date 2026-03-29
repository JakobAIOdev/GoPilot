package app

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/JakobAIOdev/GoPilot/internal/chat"
)

const sessionTitleLimit = 48

type storedSession struct {
	ID            string             `json:"id"`
	Title         string             `json:"title"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
	WorkspaceRoot string             `json:"workspace_root"`
	Model         string             `json:"model"`
	Messages      []chat.Message     `json:"messages"`
	SharedHistory []chat.Message     `json:"shared_history"`
	ContextFiles  []chat.ContextFile `json:"context_files"`
	UndoHistory   []undoBatch        `json:"undo_history,omitempty"`
}

type sessionSummary struct {
	ID        string
	Title     string
	UpdatedAt time.Time
}

func sessionsDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	return filepath.Join(configDir, "gopilot", "sessions"), nil
}

func ensureSessionsDir() (string, error) {
	dir, err := sessionsDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create sessions dir: %w", err)
	}
	return dir, nil
}

func sessionFilePath(id string) (string, error) {
	dir, err := ensureSessionsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, id+".json"), nil
}

func newSessionID() string {
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("session-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("%s-%x", time.Now().Format("20060102-150405"), raw[:])
}

func deriveSessionTitle(messages []chat.Message) string {
	for _, msg := range messages {
		if msg.From != "User" {
			continue
		}
		text := strings.Join(strings.Fields(strings.TrimSpace(msg.Content)), " ")
		if text == "" {
			continue
		}
		if len(text) > sessionTitleLimit {
			return text[:sessionTitleLimit-1] + "…"
		}
		return text
	}
	return "New Chat"
}

func saveStoredSession(session storedSession) error {
	if strings.TrimSpace(session.ID) == "" {
		return fmt.Errorf("session id is empty")
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now()
	}
	session.UpdatedAt = time.Now()
	if strings.TrimSpace(session.Title) == "" {
		session.Title = deriveSessionTitle(session.Messages)
	}

	path, err := sessionFilePath(session.ID)
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return fmt.Errorf("write temp session: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace session: %w", err)
	}
	return nil
}

func loadStoredSession(id string) (storedSession, error) {
	path, err := sessionFilePath(id)
	if err != nil {
		return storedSession{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return storedSession{}, fmt.Errorf("read session: %w", err)
	}
	var session storedSession
	if err := json.Unmarshal(data, &session); err != nil {
		return storedSession{}, fmt.Errorf("decode session: %w", err)
	}
	return session, nil
}

func listStoredSessions() ([]sessionSummary, error) {
	dir, err := ensureSessionsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read sessions dir: %w", err)
	}

	summaries := make([]sessionSummary, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var summary struct {
			ID        string    `json:"id"`
			Title     string    `json:"title"`
			UpdatedAt time.Time `json:"updated_at"`
		}
		if err := json.Unmarshal(data, &summary); err != nil {
			continue
		}
		summaries = append(summaries, sessionSummary{
			ID:        summary.ID,
			Title:     summary.Title,
			UpdatedAt: summary.UpdatedAt,
		})
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
	})
	return summaries, nil
}

func loadLatestStoredSession() (storedSession, error) {
	summaries, err := listStoredSessions()
	if err != nil {
		return storedSession{}, err
	}
	if len(summaries) == 0 {
		return storedSession{}, os.ErrNotExist
	}
	return loadStoredSession(summaries[0].ID)
}

func formatSessionList(summaries []sessionSummary) string {
	if len(summaries) == 0 {
		return "No saved sessions."
	}

	lines := []string{fmt.Sprintf("Saved sessions (%d total):", len(summaries))}
	limit := len(summaries)
	if limit > 20 {
		limit = 20
	}
	for i := 0; i < limit; i++ {
		lines = append(lines, fmt.Sprintf("- %s  •  %s  •  %s", summaries[i].ID, summaries[i].UpdatedAt.Format("2006-01-02 15:04"), summaries[i].Title))
	}
	if len(summaries) > limit {
		lines = append(lines, fmt.Sprintf("- ... and %d more  •  use /load to search and pick", len(summaries)-limit))
	}
	return strings.Join(lines, "\n")
}
