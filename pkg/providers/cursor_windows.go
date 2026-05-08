//go:build windows

package providers

import (
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type cursorComposerEntry struct {
	ComposerID        string  `json:"composerId"`
	Name              string  `json:"name"`
	CreatedAt         int64   `json:"createdAt"`
	UnifiedMode       string  `json:"unifiedMode"`
	IsArchived        bool    `json:"isArchived"`
	FilesChangedCount int     `json:"filesChangedCount"`
}

type cursorComposerDataWin struct {
	AllComposers []cursorComposerEntry `json:"allComposers"`
}

// ParseCursorWindowsWorkspaceSessions reads Cursor composer sessions from
// %APPDATA%\Cursor\User\workspaceStorage\*\state.vscdb on Windows.
// Cursor on Windows does not produce agent-transcript JSONL files, so this
// provides session activity data (names, dates) even without token counts.
func ParseCursorWindowsWorkspaceSessions() []Session {
	appdata := os.Getenv("APPDATA")
	if appdata == "" {
		return nil
	}
	wsRoot := filepath.Join(appdata, "Cursor", "User", "workspaceStorage")
	entries, err := os.ReadDir(wsRoot)
	if err != nil {
		return nil
	}

	var sessions []Session
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dbPath := filepath.Join(wsRoot, entry.Name(), "state.vscdb")
		if _, err := os.Stat(dbPath); err != nil {
			continue
		}
		project := cursorWorkspaceProject(filepath.Join(wsRoot, entry.Name(), "workspace.json"))
		if project == "" {
			project = entry.Name()
		}
		sessions = append(sessions, cursorWorkspaceDBSessions(dbPath, project)...)
	}
	return sessions
}

func cursorWorkspaceProject(wjPath string) string {
	data, err := os.ReadFile(wjPath)
	if err != nil {
		return ""
	}
	var wj struct {
		Folder string `json:"folder"`
	}
	if err := json.Unmarshal(data, &wj); err != nil || wj.Folder == "" {
		return ""
	}
	decoded, err := url.PathUnescape(wj.Folder)
	if err != nil {
		decoded = wj.Folder
	}
	if i := strings.Index(decoded, "///"); i >= 0 {
		decoded = decoded[i+3:]
	}
	return filepath.Base(filepath.FromSlash(decoded))
}

func cursorWorkspaceDBSessions(dbPath, project string) []Session {
	data, err := os.ReadFile(dbPath)
	if err != nil {
		return nil
	}
	tmp, err := os.CreateTemp("", "tokalytics-cursor-ws-*.db")
	if err != nil {
		return nil
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		_ = os.Remove(tmpPath)
		return nil
	}
	defer os.Remove(tmpPath)

	db, err := sql.Open("sqlite3", tmpPath+"?immutable=1")
	if err != nil {
		return nil
	}
	defer db.Close()

	var raw string
	if err := db.QueryRow("SELECT value FROM ItemTable WHERE key = 'composer.composerData'").Scan(&raw); err != nil {
		return nil
	}

	var cd cursorComposerDataWin
	if err := json.Unmarshal([]byte(raw), &cd); err != nil {
		return nil
	}

	var sessions []Session
	for _, c := range cd.AllComposers {
		if c.IsArchived || c.ComposerID == "" {
			continue
		}
		ts := time.UnixMilli(c.CreatedAt).UTC()
		name := c.Name
		if name == "" {
			name = "(sem título)"
		}
		model := "cursor"
		if c.UnifiedMode != "" {
			model = "cursor-" + c.UnifiedMode
		}
		qc := c.FilesChangedCount
		if qc < 1 {
			qc = 1
		}
		sessions = append(sessions, Session{
			SessionID:   "cursor-ws-" + c.ComposerID,
			Project:     project,
			Date:        ts.Format("2006-01-02"),
			Timestamp:   ts.Format(time.RFC3339),
			FirstPrompt: name,
			Model:       model,
			QueryCount:  qc,
			Queries:     []Query{{Model: model}},
		})
	}
	return sessions
}
