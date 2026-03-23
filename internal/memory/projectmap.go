package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ProjectFile describes a key file in the project.
type ProjectFile struct {
	Path     string `json:"path"`
	Language string `json:"language"`
	Role     string `json:"role"` // "entry", "config", "model", "handler", "test", "util", "infra"
}

// ProjectMap holds a cached map of the project's structure.
type ProjectMap struct {
	Root        string         `json:"root"`
	FileCount   int            `json:"file_count"`
	Languages   map[string]int `json:"languages"`
	KeyFiles    []ProjectFile  `json:"key_files"`
	Directories []string       `json:"directories"`
	ScannedAt   string         `json:"scanned_at"`
}

// ScanProject scans a git root directory and builds a ProjectMap.
func (s *Store) ScanProject(gitRoot string) (*ProjectMap, error) {
	pm := &ProjectMap{
		Root:      gitRoot,
		Languages: make(map[string]int),
		ScannedAt: time.Now().UTC().Format(time.DateTime),
	}

	files, err := findSourceFiles(gitRoot)
	if err != nil {
		return nil, fmt.Errorf("scan project files: %w", err)
	}

	pm.FileCount = len(files)

	for _, f := range files {
		lang := languageFromExt(filepath.Ext(f))
		if lang != "" {
			pm.Languages[lang]++
		}
	}

	pm.KeyFiles = identifyKeyFiles(files, gitRoot)
	pm.Directories = listTopLevelDirs(gitRoot)

	if err := s.SaveProjectMap(pm); err != nil {
		return nil, err
	}
	return pm, nil
}

// GetProjectMap retrieves the cached project map from the database.
func (s *Store) GetProjectMap() (*ProjectMap, error) {
	var data string
	err := s.db.Reader().QueryRow(`SELECT data FROM project_map WHERE id = 1`).Scan(&data)
	if err != nil {
		return nil, fmt.Errorf("no project map found: %w", err)
	}
	var pm ProjectMap
	if err := json.Unmarshal([]byte(data), &pm); err != nil {
		return nil, fmt.Errorf("unmarshal project map: %w", err)
	}
	return &pm, nil
}

// SaveProjectMap persists a ProjectMap to the database.
func (s *Store) SaveProjectMap(pm *ProjectMap) error {
	data, err := json.Marshal(pm)
	if err != nil {
		return fmt.Errorf("marshal project map: %w", err)
	}
	_, err = s.db.Writer().Exec(
		`INSERT INTO project_map (id, data, scanned_at) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET data = excluded.data, scanned_at = excluded.scanned_at`,
		string(data), pm.ScannedAt,
	)
	if err != nil {
		return fmt.Errorf("save project map: %w", err)
	}
	return nil
}

// findSourceFiles runs find to discover source files, respecting .gitignore via git ls-files.
func findSourceFiles(gitRoot string) ([]string, error) {
	// Try git ls-files first (respects .gitignore).
	cmd := exec.Command("git", "ls-files", "--cached", "--others", "--exclude-standard",
		"*.go", "*.ts", "*.tsx", "*.js", "*.jsx", "*.py", "*.rs", "*.java",
		"*.rb", "*.c", "*.cpp", "*.h", "*.hpp", "*.cs", "*.swift", "*.kt",
		"*.scala", "*.sh", "*.yml", "*.yaml", "*.toml", "*.json", "*.sql",
		"*.proto", "*.graphql",
	)
	cmd.Dir = gitRoot
	out, err := cmd.Output()
	if err != nil {
		// Fallback: use find for common source files.
		return findSourceFilesFallback(gitRoot)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			files = append(files, l)
		}
	}

	// Cap at 500 files for performance.
	if len(files) > 500 {
		files = files[:500]
	}
	return files, nil
}

func findSourceFilesFallback(gitRoot string) ([]string, error) {
	cmd := exec.Command("find", gitRoot,
		"-maxdepth", "6",
		"(", "-name", "*.go", "-o", "-name", "*.ts", "-o", "-name", "*.js",
		"-o", "-name", "*.py", "-o", "-name", "*.rs", "-o", "-name", "*.java", ")",
		"-not", "-path", "*/node_modules/*",
		"-not", "-path", "*/.git/*",
		"-not", "-path", "*/vendor/*",
	)
	cmd.Dir = gitRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("find source files: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var files []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l != "" {
			rel, _ := filepath.Rel(gitRoot, l)
			if rel != "" {
				files = append(files, rel)
			}
		}
	}
	if len(files) > 500 {
		files = files[:500]
	}
	return files, nil
}

func languageFromExt(ext string) string {
	m := map[string]string{
		".go": "go", ".ts": "typescript", ".tsx": "typescript", ".js": "javascript",
		".jsx": "javascript", ".py": "python", ".rs": "rust", ".java": "java",
		".rb": "ruby", ".c": "c", ".cpp": "cpp", ".h": "c", ".hpp": "cpp",
		".cs": "csharp", ".swift": "swift", ".kt": "kotlin", ".scala": "scala",
		".sh": "shell", ".yml": "yaml", ".yaml": "yaml", ".toml": "toml",
		".json": "json", ".sql": "sql", ".proto": "protobuf", ".graphql": "graphql",
	}
	return m[strings.ToLower(ext)]
}

// identifyKeyFiles classifies files by role based on name patterns.
func identifyKeyFiles(files []string, gitRoot string) []ProjectFile {
	type rule struct {
		pattern  string
		suffix   bool
		exact    bool
		role     string
		priority int // lower = more important
	}
	rules := []rule{
		// Entry points
		{pattern: "main.go", exact: true, role: "entry", priority: 1},
		{pattern: "/main.go", suffix: true, role: "entry", priority: 1},
		{pattern: "index.ts", exact: true, role: "entry", priority: 1},
		{pattern: "index.js", exact: true, role: "entry", priority: 1},
		{pattern: "app.py", exact: true, role: "entry", priority: 1},
		{pattern: "/cmd/", suffix: false, role: "entry", priority: 2},
		// Config
		{pattern: "go.mod", exact: true, role: "config", priority: 1},
		{pattern: "go.sum", exact: true, role: "config", priority: 3},
		{pattern: "package.json", exact: true, role: "config", priority: 1},
		{pattern: "Cargo.toml", exact: true, role: "config", priority: 1},
		{pattern: "pyproject.toml", exact: true, role: "config", priority: 1},
		{pattern: "tsconfig.json", exact: true, role: "config", priority: 1},
		{pattern: "Makefile", exact: true, role: "config", priority: 2},
		// Infra
		{pattern: "Dockerfile", exact: true, role: "infra", priority: 2},
		{pattern: "docker-compose.yml", exact: true, role: "infra", priority: 2},
		{pattern: "docker-compose.yaml", exact: true, role: "infra", priority: 2},
		{pattern: ".github/workflows/", suffix: false, role: "infra", priority: 3},
		// Tests
		{pattern: "_test.go", suffix: true, role: "test", priority: 4},
		{pattern: ".test.ts", suffix: true, role: "test", priority: 4},
		{pattern: ".test.js", suffix: true, role: "test", priority: 4},
		{pattern: ".spec.ts", suffix: true, role: "test", priority: 4},
		{pattern: "_test.py", suffix: true, role: "test", priority: 4},
		{pattern: "test_", suffix: false, role: "test", priority: 4},
	}

	// Also check for key files that might exist but weren't in git ls-files output
	// (like Makefile, Dockerfile, etc.)
	extraNames := []string{"Makefile", "Dockerfile", "docker-compose.yml", "docker-compose.yaml"}
	for _, name := range extraNames {
		fullPath := filepath.Join(gitRoot, name)
		if _, err := os.Stat(fullPath); err == nil {
			found := false
			for _, f := range files {
				if f == name {
					found = true
					break
				}
			}
			if !found {
				files = append(files, name)
			}
		}
	}

	type scored struct {
		file     ProjectFile
		priority int
	}
	var matched []scored

	for _, f := range files {
		base := filepath.Base(f)
		for _, r := range rules {
			hit := false
			if r.exact {
				hit = base == r.pattern
			} else if r.suffix {
				hit = strings.HasSuffix(f, r.pattern)
			} else {
				hit = strings.Contains(f, r.pattern)
			}
			if hit {
				lang := languageFromExt(filepath.Ext(f))
				matched = append(matched, scored{
					file:     ProjectFile{Path: f, Language: lang, Role: r.role},
					priority: r.priority,
				})
				break // first matching rule wins
			}
		}
	}

	// Sort by priority, then path.
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].priority != matched[j].priority {
			return matched[i].priority < matched[j].priority
		}
		return matched[i].file.Path < matched[j].file.Path
	})

	// Limit test files to avoid flooding output.
	var result []ProjectFile
	testCount := 0
	for _, m := range matched {
		if m.file.Role == "test" {
			testCount++
			if testCount > 5 {
				continue
			}
		}
		result = append(result, m.file)
	}

	// Cap total key files.
	if len(result) > 30 {
		result = result[:30]
	}
	return result
}

// listTopLevelDirs returns the immediate subdirectories of the git root,
// excluding hidden dirs and common non-essential dirs.
func listTopLevelDirs(gitRoot string) []string {
	entries, err := os.ReadDir(gitRoot)
	if err != nil {
		return nil
	}
	skip := map[string]bool{
		"node_modules": true, ".git": true, "vendor": true,
		".idea": true, ".vscode": true, "__pycache__": true,
		".memory": true, "dist": true, "build": true, ".next": true,
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") || skip[name] {
			continue
		}
		dirs = append(dirs, name)
	}
	sort.Strings(dirs)
	return dirs
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// FormatProjectMap returns a human-readable markdown representation of the project map.
func FormatProjectMap(pm *ProjectMap) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Project Map: %s\n\n", filepath.Base(pm.Root))
	fmt.Fprintf(&b, "**Files:** %d | **Scanned:** %s\n\n", pm.FileCount, pm.ScannedAt)

	// Languages
	if len(pm.Languages) > 0 {
		b.WriteString("## Languages\n\n")
		type langCount struct {
			lang  string
			count int
		}
		var langs []langCount
		for l, c := range pm.Languages {
			langs = append(langs, langCount{l, c})
		}
		sort.Slice(langs, func(i, j int) bool { return langs[i].count > langs[j].count })
		for _, lc := range langs {
			fmt.Fprintf(&b, "- %s: %d files\n", lc.lang, lc.count)
		}
		b.WriteString("\n")
	}

	// Directories
	if len(pm.Directories) > 0 {
		b.WriteString("## Directories\n\n")
		for _, d := range pm.Directories {
			fmt.Fprintf(&b, "- %s/\n", d)
		}
		b.WriteString("\n")
	}

	// Key files by role
	if len(pm.KeyFiles) > 0 {
		b.WriteString("## Key Files\n\n")
		roleOrder := []string{"entry", "config", "infra", "handler", "model", "util", "test"}
		grouped := map[string][]ProjectFile{}
		for _, f := range pm.KeyFiles {
			grouped[f.Role] = append(grouped[f.Role], f)
		}
		for _, role := range roleOrder {
			files := grouped[role]
			if len(files) == 0 {
				continue
			}
			fmt.Fprintf(&b, "### %s\n", titleCase(role))
			for _, f := range files {
				lang := f.Language
				if lang == "" {
					lang = "-"
				}
				fmt.Fprintf(&b, "- `%s` (%s)\n", f.Path, lang)
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}
