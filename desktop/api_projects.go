package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ==================== Projects & Goals ====================

type ProjectMilestone struct {
	Text string `json:"text"`
	Done bool   `json:"done"`
}

type ProjectGoal struct {
	Title      string             `json:"title"`
	Done       bool               `json:"done"`
	Milestones []ProjectMilestone `json:"milestones"`
}

type Project struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Folder      string        `json:"folder"`
	Tags        []string      `json:"tags"`
	Status      string        `json:"status"`
	Color       string        `json:"color"`
	CreatedAt   string        `json:"createdAt"`
	Notes       []string      `json:"notes"`
	TaskFilter  string        `json:"taskFilter"`
	Category    string        `json:"category"`
	Goals       []ProjectGoal `json:"goals"`
	NextAction  string        `json:"nextAction"`
	Priority    int           `json:"priority"`
	DueDate     string        `json:"dueDate"`
	TimeSpent   int           `json:"timeSpent"`
}

func (a *GranitApp) projectsFile() string {
	return filepath.Join(a.vaultRoot, ".granit", "projects.json")
}

func (a *GranitApp) loadProjects() ([]Project, error) {
	data, err := os.ReadFile(a.projectsFile())
	if err != nil {
		if os.IsNotExist(err) {
			return []Project{}, nil
		}
		return nil, err
	}
	var projects []Project
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func (a *GranitApp) saveProjects(projects []Project) error {
	dir := filepath.Join(a.vaultRoot, ".granit")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(a.projectsFile(), data, 0o644)
}

// GetProjects returns all projects.
func (a *GranitApp) GetProjects() ([]Project, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.vaultRoot == "" {
		return nil, fmt.Errorf("no vault open")
	}
	return a.loadProjects()
}

// SaveProjectsJSON saves all projects from JSON string.
func (a *GranitApp) SaveProjectsJSON(data string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.vaultRoot == "" {
		return fmt.Errorf("no vault open")
	}
	var projects []Project
	if err := json.Unmarshal([]byte(data), &projects); err != nil {
		return err
	}
	return a.saveProjects(projects)
}

// CreateProject adds a new project.
func (a *GranitApp) CreateProject(data string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.vaultRoot == "" {
		return fmt.Errorf("no vault open")
	}
	var p Project
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		return err
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("project name cannot be empty")
	}
	if p.CreatedAt == "" {
		p.CreatedAt = time.Now().Format("2006-01-02")
	}
	if p.Status == "" {
		p.Status = "active"
	}
	if p.Color == "" {
		p.Color = "blue"
	}
	projects, err := a.loadProjects()
	if err != nil {
		return err
	}
	projects = append(projects, p)
	return a.saveProjects(projects)
}

// UpdateProject updates a project at the given index.
func (a *GranitApp) UpdateProject(idx int, data string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.vaultRoot == "" {
		return fmt.Errorf("no vault open")
	}
	projects, err := a.loadProjects()
	if err != nil {
		return err
	}
	if idx < 0 || idx >= len(projects) {
		return fmt.Errorf("project index out of range")
	}
	var p Project
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		return err
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("project name cannot be empty")
	}
	projects[idx] = p
	return a.saveProjects(projects)
}

// DeleteProject removes a project at the given index.
func (a *GranitApp) DeleteProject(idx int) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.vaultRoot == "" {
		return fmt.Errorf("no vault open")
	}
	projects, err := a.loadProjects()
	if err != nil {
		return err
	}
	if idx < 0 || idx >= len(projects) {
		return fmt.Errorf("project index out of range")
	}
	projects = append(projects[:idx], projects[idx+1:]...)
	return a.saveProjects(projects)
}

// GetProjectTasks returns tasks filtered by a project's TaskFilter tag.
func (a *GranitApp) GetProjectTasks(filter string) ([]TaskItem, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	allTasks, err := a.getAllTasksInternal()
	if err != nil {
		return nil, err
	}
	if filter == "" {
		return allTasks, nil
	}
	tag := strings.ToLower(strings.TrimPrefix(filter, "#"))
	var filtered []TaskItem
	for _, t := range allTasks {
		if strings.Contains(strings.ToLower(t.Text), "#"+tag) {
			filtered = append(filtered, t)
		}
	}
	return filtered, nil
}
