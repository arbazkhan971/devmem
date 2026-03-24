package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PluginManifest describes a memorX plugin.
type PluginManifest struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author"`
	Entry       string `json:"entry"`
}

// PluginInfo describes an installed plugin.
type PluginInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Status  string `json:"status"`
	Path    string `json:"path"`
}

// HookEntry represents a registered hook.
type HookEntry struct {
	Event   string `json:"event"`
	Command string `json:"command"`
	AddedAt string `json:"added_at"`
}

// WebhookEntry represents a registered webhook.
type WebhookEntry struct {
	URL     string `json:"url"`
	Event   string `json:"event"`
	AddedAt string `json:"added_at"`
}

func pluginsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".memorx", "plugins"), nil
}

func memorxConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home dir: %w", err)
	}
	return filepath.Join(home, ".memorx"), nil
}

// InstallPlugin creates the plugin directory structure and manifest.
func InstallPlugin(url string) (*PluginInfo, error) {
	dir, err := pluginsDir()
	if err != nil {
		return nil, err
	}
	name := extractPluginName(url)
	if name == "" {
		return nil, fmt.Errorf("cannot extract plugin name from URL: %s", url)
	}
	pluginDir := filepath.Join(dir, name)
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return nil, fmt.Errorf("create plugin dir: %w", err)
	}
	manifest := PluginManifest{
		Name: name, Version: "0.1.0",
		Description: fmt.Sprintf("Plugin installed from %s", url),
		Author: "unknown", Entry: "index.js",
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0644); err != nil {
		return nil, fmt.Errorf("write manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, ".source"), []byte(url), 0644); err != nil {
		return nil, fmt.Errorf("write source: %w", err)
	}
	return &PluginInfo{Name: name, Version: manifest.Version, Status: "installed", Path: pluginDir}, nil
}

// ListPlugins lists installed plugins from ~/.memorx/plugins/.
func ListPlugins() ([]PluginInfo, error) {
	dir, err := pluginsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read plugins dir: %w", err)
	}
	var plugins []PluginInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(dir, entry.Name(), "plugin.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			plugins = append(plugins, PluginInfo{Name: entry.Name(), Status: "invalid (no plugin.json)", Path: filepath.Join(dir, entry.Name())})
			continue
		}
		var manifest PluginManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			plugins = append(plugins, PluginInfo{Name: entry.Name(), Status: "invalid (bad plugin.json)", Path: filepath.Join(dir, entry.Name())})
			continue
		}
		plugins = append(plugins, PluginInfo{Name: manifest.Name, Version: manifest.Version, Status: "installed", Path: filepath.Join(dir, entry.Name())})
	}
	return plugins, nil
}

// FormatPluginList formats a list of plugins into a human-readable string.
func FormatPluginList(plugins []PluginInfo) string {
	if len(plugins) == 0 {
		return "No plugins installed. Use memorx_plugin_install to add plugins."
	}
	var b strings.Builder
	b.WriteString("# Installed Plugins\n\n")
	b.WriteString("| Name | Version | Status |\n|------|---------|--------|\n")
	for _, p := range plugins {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", p.Name, p.Version, p.Status)
	}
	return b.String()
}

// RegisterHook stores a hook in ~/.memorx/hooks.json.
func RegisterHook(event, command string) error {
	dir, err := memorxConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create memorx dir: %w", err)
	}
	validEvents := map[string]bool{
		"on_commit": true, "on_session_start": true,
		"on_session_end": true, "on_remember": true,
	}
	if !validEvents[event] {
		return fmt.Errorf("invalid event %q (valid: on_commit, on_session_start, on_session_end, on_remember)", event)
	}
	hooksPath := filepath.Join(dir, "hooks.json")
	hooks, err := loadHooks(hooksPath)
	if err != nil {
		return err
	}
	hooks = append(hooks, HookEntry{Event: event, Command: command, AddedAt: time.Now().UTC().Format(time.DateTime)})
	return saveHooks(hooksPath, hooks)
}

// ListHooks returns all registered hooks.
func ListHooks() ([]HookEntry, error) {
	dir, err := memorxConfigDir()
	if err != nil {
		return nil, err
	}
	return loadHooks(filepath.Join(dir, "hooks.json"))
}

// FormatHooks formats hooks into a human-readable string.
func FormatHooks(hooks []HookEntry) string {
	if len(hooks) == 0 {
		return "No hooks registered."
	}
	var b strings.Builder
	b.WriteString("# Registered Hooks\n\n")
	for _, h := range hooks {
		fmt.Fprintf(&b, "- **%s**: `%s` (added %s)\n", h.Event, h.Command, h.AddedAt)
	}
	return b.String()
}

func loadHooks(path string) ([]HookEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read hooks: %w", err)
	}
	var hooks []HookEntry
	if err := json.Unmarshal(data, &hooks); err != nil {
		return nil, fmt.Errorf("parse hooks: %w", err)
	}
	return hooks, nil
}

func saveHooks(path string, hooks []HookEntry) error {
	data, err := json.MarshalIndent(hooks, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal hooks: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// ManageWebhook registers, lists, or removes webhooks.
func ManageWebhook(action, url, event string) (string, error) {
	dir, err := memorxConfigDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create memorx dir: %w", err)
	}
	whPath := filepath.Join(dir, "webhooks.json")
	webhooks, err := loadWebhooks(whPath)
	if err != nil {
		return "", err
	}
	switch action {
	case "register":
		if url == "" || event == "" {
			return "", fmt.Errorf("url and event are required for register")
		}
		webhooks = append(webhooks, WebhookEntry{URL: url, Event: event, AddedAt: time.Now().UTC().Format(time.DateTime)})
		if err := saveWebhooks(whPath, webhooks); err != nil {
			return "", err
		}
		return fmt.Sprintf("Webhook registered: %s -> %s", event, url), nil
	case "list":
		return FormatWebhooks(webhooks), nil
	case "remove":
		if url == "" {
			return "", fmt.Errorf("url is required for remove")
		}
		var kept []WebhookEntry
		removed := 0
		for _, wh := range webhooks {
			if wh.URL == url {
				removed++
			} else {
				kept = append(kept, wh)
			}
		}
		if removed == 0 {
			return fmt.Sprintf("No webhook found with URL: %s", url), nil
		}
		if err := saveWebhooks(whPath, kept); err != nil {
			return "", err
		}
		return fmt.Sprintf("Removed %d webhook(s) matching: %s", removed, url), nil
	default:
		return "", fmt.Errorf("invalid action %q (valid: register, list, remove)", action)
	}
}

// FormatWebhooks formats webhooks into a human-readable string.
func FormatWebhooks(webhooks []WebhookEntry) string {
	if len(webhooks) == 0 {
		return "No webhooks registered."
	}
	var b strings.Builder
	b.WriteString("# Registered Webhooks\n\n")
	for _, wh := range webhooks {
		fmt.Fprintf(&b, "- **%s** -> %s (added %s)\n", wh.Event, wh.URL, wh.AddedAt)
	}
	return b.String()
}

func loadWebhooks(path string) ([]WebhookEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read webhooks: %w", err)
	}
	var webhooks []WebhookEntry
	if err := json.Unmarshal(data, &webhooks); err != nil {
		return nil, fmt.Errorf("parse webhooks: %w", err)
	}
	return webhooks, nil
}

func saveWebhooks(path string, webhooks []WebhookEntry) error {
	data, err := json.MarshalIndent(webhooks, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal webhooks: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

func extractPluginName(url string) string {
	url = strings.TrimSuffix(url, "/")
	url = strings.TrimSuffix(url, ".git")
	parts := strings.Split(url, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}
