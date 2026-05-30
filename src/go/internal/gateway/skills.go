package gateway

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ── Constants ──────────────────────────────────────────────────

var slugRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,99}$`)

// ── Types ──────────────────────────────────────────────────────

type skillSummary struct {
	Slug        string  `json:"slug"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Version     *string `json:"version,omitempty"`
	SkillFile   string  `json:"skillFile"`
	SkillDir    string  `json:"skillDir"`
	Scope       string  `json:"scope"`
	Mtime       *int64  `json:"mtime,omitempty"`
	IsActive    bool    `json:"is_active"`
}

type skillMeta struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// ── Path resolution ────────────────────────────────────────────

func userSkillsRoot() string {
	pilotHome := resolvePilotHome()
	return filepath.Join(pilotHome, "skills")
}

func projectSkillsRoot(projectPath string) string {
	return filepath.Join(projectPath, ".pilotdeck", "skills")
}

func skillDir(scope, slug, projectKey string) (string, error) {
	if !isValidSlug(slug) {
		return "", fmt.Errorf("invalid slug: %q", slug)
	}
	var root string
	switch scope {
	case "user":
		root = userSkillsRoot()
	case "project":
		if projectKey == "" {
			return "", fmt.Errorf("project scope requires a projectKey")
		}
		root = projectSkillsRoot(projectKey)
	default:
		return "", fmt.Errorf("unknown scope: %s", scope)
	}
	return filepath.Join(root, slug), nil
}

func isValidSlug(slug string) bool {
	return slugRe.MatchString(slug) && !strings.Contains(slug, "..")
}

// ── SKILL.md parsing ───────────────────────────────────────────

func parseSkillMeta(content string) skillMeta {
	meta := skillMeta{}
	if !strings.HasPrefix(content, "---") {
		return meta
	}
	endRel := strings.Index(content[3:], "\n---")
	if endRel == -1 {
		return meta
	}
	fmRaw := strings.TrimPrefix(content[3:3+endRel], "\n")
	fmRaw = strings.TrimPrefix(fmRaw, "\r\n")
	for _, line := range strings.Split(fmRaw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			meta.Name = strings.TrimSpace(line[5:])
		} else if strings.HasPrefix(line, "description:") {
			meta.Description = strings.TrimSpace(line[12:])
		}
	}
	return meta
}

func readSkillMeta(skillDir, slug, scope string) skillSummary {
	mdFile := filepath.Join(skillDir, "SKILL.md")
	meta := skillMeta{}
	mtime := int64(0)
	data, err := os.ReadFile(mdFile)
	if err == nil {
		meta = parseSkillMeta(string(data))
		if fi, fiErr := os.Stat(mdFile); fiErr == nil {
			mtime = fi.ModTime().UnixMilli()
		}
	}
	var vers *string
	if meta.Name != "" {
		v := meta.Name
		vers = &v
	}
	var mt *int64
	if mtime > 0 {
		mt = &mtime
	}
	return skillSummary{
		Slug:        slug,
		Name:        meta.Name,
		Description: meta.Description,
		Version:     vers,
		SkillFile:   mdFile,
		SkillDir:    skillDir,
		Scope:       scope,
		Mtime:       mt,
		IsActive:    true,
	}
}

// ── Handlers ───────────────────────────────────────────────────

func handleSkillList(gs *GatewaySession, req WsRequest) {
	var params struct {
		ProjectKey string `json:"projectKey"`
	}
	json.Unmarshal(req.Params, &params)
	projectKey := params.ProjectKey

	// Collect user skills from multiple sources
	seen := make(map[string]bool)
	userSkills := make([]skillSummary, 0)

	// 1. PilotDeck user skills (PILOT_HOME/skills)
	for _, s := range listSkillsIn(userSkillsRoot(), "user") {
		if !seen[s.Name] {
			seen[s.Name] = true
			userSkills = append(userSkills, s)
		}
	}

	// Project skills
	projectSkills := make([]skillSummary, 0)
	if projectKey != "" {
		projectSkills = listSkillsIn(projectSkillsRoot(projectKey), "project")
	}

	var nilProjectPath interface{}
	if projectKey != "" {
		nilProjectPath = projectKey
	}

	sendResponse(gs, req.ID, true, map[string]interface{}{
		"user":        userSkills,
		"project":     projectSkills,
		"projectPath": nilProjectPath,
	})
}

func listSkillsIn(root, scope string) []skillSummary {
	entries, err := os.ReadDir(root)
	if err != nil {
		return []skillSummary{}
	}
	result := make([]skillSummary, 0)
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		result = append(result, readSkillMeta(filepath.Join(root, e.Name()), e.Name(), scope))
	}
	return result
}

func handleSkillRead(gs *GatewaySession, req WsRequest) {
	var params struct {
		Scope      string `json:"scope"`
		Slug       string `json:"slug"`
		ProjectKey string `json:"projectKey"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_params", Message: "cannot parse params"})
		return
	}

	dir, err := skillDir(params.Scope, params.Slug, params.ProjectKey)
	if err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_input", Message: err.Error()})
		return
	}

	mdFile := filepath.Join(dir, "SKILL.md")
	content, err := os.ReadFile(mdFile)
	if err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "not_found", Message: "SKILL.md not found"})
		return
	}

	skill := readSkillMeta(dir, params.Slug, params.Scope)
	sendResponse(gs, req.ID, true, map[string]interface{}{
		"content": string(content),
		"scope":   params.Scope,
		"slug":    params.Slug,
		"skill":   skill,
	})
}

func handleSkillWrite(gs *GatewaySession, req WsRequest) {
	var params struct {
		Scope      string `json:"scope"`
		Slug       string `json:"slug"`
		ProjectKey string `json:"projectKey"`
		Content    string `json:"content"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_params", Message: "cannot parse params"})
		return
	}

	if params.Content == "" {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_input", Message: "content is required"})
		return
	}

	dir, err := skillDir(params.Scope, params.Slug, params.ProjectKey)
	if err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_input", Message: err.Error()})
		return
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "write_error", Message: err.Error()})
		return
	}

	mdFile := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(mdFile, []byte(params.Content), 0644); err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "write_error", Message: err.Error()})
		return
	}

	skill := readSkillMeta(dir, params.Slug, params.Scope)
	sendResponse(gs, req.ID, true, map[string]interface{}{
		"ok":    true,
		"scope": params.Scope,
		"slug":  params.Slug,
		"skill": skill,
	})
}

func handleSkillCreate(gs *GatewaySession, req WsRequest) {
	var params struct {
		Scope       string `json:"scope"`
		Slug        string `json:"slug"`
		ProjectKey  string `json:"projectKey"`
		Content     string `json:"content"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_params", Message: "cannot parse params"})
		return
	}

	dir, err := skillDir(params.Scope, params.Slug, params.ProjectKey)
	if err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_input", Message: err.Error()})
		return
	}

	// Check if already exists
	if _, err := os.Stat(dir); err == nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "conflict", Message: "Skill already exists"})
		return
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "write_error", Message: err.Error()})
		return
	}

	// Build initial content
	finalContent := params.Content
	if finalContent == "" {
		name := params.Name
		if name == "" {
			name = params.Slug
		}
		desc := params.Description
		builder := "---\n"
		builder += "name: " + strings.ReplaceAll(name, "\n", " ") + "\n"
		if desc != "" {
			builder += "description: " + strings.ReplaceAll(desc, "\n", " ") + "\n"
		}
		builder += "---\n\n"
		builder += "# " + name + "\n\n"
		builder += "Describe what this skill does, when to invoke it, and any prerequisites.\n"
		finalContent = builder
	}

	mdFile := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(mdFile, []byte(finalContent), 0644); err != nil {
		os.RemoveAll(dir)
		sendResponse(gs, req.ID, false, &WsError{Code: "write_error", Message: err.Error()})
		return
	}

	skill := readSkillMeta(dir, params.Slug, params.Scope)
	sendResponse(gs, req.ID, true, map[string]interface{}{
		"ok":        true,
		"scope":     params.Scope,
		"slug":      params.Slug,
		"skillPath": dir,
		"skill":     skill,
	})
}

func handleSkillDelete(gs *GatewaySession, req WsRequest) {
	var params struct {
		Scope      string `json:"scope"`
		Slug       string `json:"slug"`
		ProjectKey string `json:"projectKey"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_params", Message: "cannot parse params"})
		return
	}

	dir, err := skillDir(params.Scope, params.Slug, params.ProjectKey)
	if err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_input", Message: err.Error()})
		return
	}

	os.RemoveAll(dir)
	sendResponse(gs, req.ID, true, map[string]interface{}{
		"ok":    true,
		"scope": params.Scope,
		"slug":  params.Slug,
	})
}

func handleSkillValidate(gs *GatewaySession, req WsRequest) {
	var params struct {
		SourcePath string `json:"sourcePath"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_params", Message: "cannot parse params"})
		return
	}

	sourcePath := params.SourcePath
	if sourcePath == "" {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_input", Message: "sourcePath is required"})
		return
	}

	info, err := os.Stat(sourcePath)
	if err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "not_found", Message: "Source path not found"})
		return
	}
	if !info.IsDir() {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_input", Message: "Source is not a directory"})
		return
	}

	// Check SKILL.md exists
	mdFile := filepath.Join(sourcePath, "SKILL.md")
	if _, err := os.Stat(mdFile); err != nil {
		sendResponse(gs, req.ID, true, map[string]interface{}{
			"ok":     false,
			"issues": []interface{}{},
			"sourcePath": sourcePath,
		})
		return
	}

	sendResponse(gs, req.ID, true, map[string]interface{}{
		"ok":     true,
		"issues": []interface{}{},
		"sourcePath": sourcePath,
	})
}

func handleSkillImport(gs *GatewaySession, req WsRequest) {
	var params struct {
		Scope      string `json:"scope"`
		Slug       string `json:"slug"`
		ProjectKey string `json:"projectKey"`
		SourcePath string `json:"sourcePath"`
		Mode       string `json:"mode"` // "copy" or "symlink"
		Force      bool   `json:"force"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_params", Message: "cannot parse params"})
		return
	}

	if params.SourcePath == "" {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_input", Message: "sourcePath is required"})
		return
	}

	// Check source exists
	srcInfo, err := os.Stat(params.SourcePath)
	if err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "source_missing", Message: "Source path does not exist"})
		return
	}
	if !srcInfo.IsDir() {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_input", Message: "Source is not a directory"})
		return
	}

	// Check source has SKILL.md
	if _, err := os.Stat(filepath.Join(params.SourcePath, "SKILL.md")); err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "no_skill_md", Message: "Source does not contain SKILL.md"})
		return
	}

	// Determine slug
	inferredSlug := params.Slug
	if inferredSlug == "" {
		inferredSlug = filepath.Base(params.SourcePath)
	}

	dir, err := skillDir(params.Scope, inferredSlug, params.ProjectKey)
	if err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_input", Message: err.Error()})
		return
	}

	// Check target
	if _, err := os.Stat(dir); err == nil && !params.Force {
		sendResponse(gs, req.ID, false, &WsError{Code: "conflict", Message: "Target already exists"})
		return
	}

	if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "write_error", Message: err.Error()})
		return
	}

	if _, err := os.Stat(dir); err == nil {
		os.RemoveAll(dir)
	}

	importMode := params.Mode
	if importMode == "" {
		importMode = "copy"
	}

	if importMode == "symlink" {
		if err := os.Symlink(params.SourcePath, dir); err != nil {
			sendResponse(gs, req.ID, false, &WsError{Code: "write_error", Message: err.Error()})
			return
		}
	} else {
		if err := copyDir(params.SourcePath, dir); err != nil {
			sendResponse(gs, req.ID, false, &WsError{Code: "write_error", Message: err.Error()})
			return
		}
	}

	skill := readSkillMeta(dir, inferredSlug, params.Scope)
	sendResponse(gs, req.ID, true, map[string]interface{}{
		"ok":         true,
		"mode":       importMode,
		"scope":      params.Scope,
		"slug":       inferredSlug,
		"sourcePath": params.SourcePath,
		"skillPath":  dir,
		"skill":      skill,
	})
}

func handleSkillScan(gs *GatewaySession, req WsRequest) {
	var params struct {
		ParentPath string `json:"parentPath"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_params", Message: "cannot parse params"})
		return
	}

	parentPath := params.ParentPath
	if parentPath == "" {
		sendResponse(gs, req.ID, false, &WsError{Code: "invalid_input", Message: "parentPath is required"})
		return
	}

	entries, err := os.ReadDir(parentPath)
	if err != nil {
		sendResponse(gs, req.ID, false, &WsError{Code: "not_found", Message: "Directory not found"})
		return
	}

	type scanFolder struct {
		FolderName  string       `json:"folderName"`
		HasSkillMd  bool         `json:"hasSkillMd"`
		Name        interface{} `json:"name"`
		Description interface{} `json:"description"`
		SourcePath  string       `json:"sourcePath"`
		FileCount   int          `json:"fileCount"`
		TotalSize   int64        `json:"totalSize"`
	}

	var folders []scanFolder
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		subDir := filepath.Join(parentPath, e.Name())
		mdFile := filepath.Join(subDir, "SKILL.md")
		hasSkillMd := false
		var name, desc interface{}
		if data, err := os.ReadFile(mdFile); err == nil {
			hasSkillMd = true
			meta := parseSkillMeta(string(data))
			name = meta.Name
			desc = meta.Description
		}

		folders = append(folders, scanFolder{
			FolderName:  e.Name(),
			HasSkillMd:  hasSkillMd,
			Name:        name,
			Description: desc,
			SourcePath:  subDir,
			FileCount:   0,
			TotalSize:   0,
		})
	}

	sendResponse(gs, req.ID, true, map[string]interface{}{
		"parentPath": parentPath,
		"folders":    folders,
	})
}

// ── Helpers ────────────────────────────────────────────────────

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}
